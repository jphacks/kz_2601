import Foundation
import MetamonDSL

/// appconfig へ送る 1 操作。`id` が create の `clientId`(冪等キー)。ファイルに永続化し再起動後も送る。
public struct SyncOperation: Codable, Sendable, Identifiable, Equatable {
    public var id: String
    public var appId: String
    public var entity: String
    /// create は仮 id(`local-<clientId>`)。送信成功後は `SyncQueue` が仮 id → サーバー id の別名を覚える。
    public var recordId: String
    public var kind: PendingOp
    public var data: [String: JSONValue]
    public var attempts: Int
    public var enqueuedAt: Date

    public init(id: String = UUID().uuidString.lowercased(), appId: String, entity: String, recordId: String,
                kind: PendingOp, data: [String: JSONValue] = [:], attempts: Int = 0, enqueuedAt: Date = .now) {
        self.id = id
        self.appId = appId
        self.entity = entity
        self.recordId = recordId
        self.kind = kind
        self.data = data
        self.attempts = attempts
        self.enqueuedAt = enqueuedAt
    }
}

/// キューから UI(AppRuntime)への通知。
public enum SyncEvent: Sendable, Equatable {
    /// 1 操作がサーバーに反映された。ローカル DB は更新済みなので entity を読み直せばよい
    case sent(appId: String, entity: String)
    /// 競合(404/409 等)。サーバー優先でローカルの未送信印を落とした。entity をサーバーから引き直すこと
    case conflict(appId: String, entity: String, message: String)
    /// サーバーが内容を拒否(400/422)。操作は破棄し、create はローカル行も消した
    case rejected(appId: String, entity: String, message: String)
    /// 連続失敗が上限に達し、次のトリガー(オンライン復帰 / 前面復帰 / 今すぐ同期)まで停止
    case paused(pending: Int)
    /// キューが空になった
    case drained
}

/// 指数バックオフ: base × multiplier^(attempt-1)。attempt は 1 始まり。
public struct BackoffPolicy: Sendable, Equatable {
    public var base: Duration
    public var multiplier: Double
    public var maxAttempts: Int

    public init(base: Duration = .milliseconds(500), multiplier: Double = 2, maxAttempts: Int = 5) {
        self.base = base
        self.multiplier = multiplier
        self.maxAttempts = maxAttempts
    }

    public func delay(attempt: Int) -> Duration {
        let seconds = Double(base.components.seconds) + Double(base.components.attoseconds) / 1e18
        return .seconds(seconds * pow(multiplier, Double(max(0, attempt - 1))))
    }

    public static let immediate = BackoffPolicy(base: .zero, multiplier: 1, maxAttempts: 5)
}

/// 未送信操作の直列キュー。FIFO・1 件ずつ・失敗は指数バックオフで同じ操作を再試行(最大 `maxAttempts`)。
/// 同じレコードへの操作は積む時に畳む(create+update → create / create+delete → 消す / update+update → 1 つ)。
public actor SyncQueue {
    public typealias Sleeper = @Sendable (Duration) async throws -> Void

    public enum EnqueueResult: Equatable { case appended, merged, cancelledCreate }

    private let store: RecordStore
    private let client: any AppConfigClient
    private let policy: BackoffPolicy
    private let sleep: Sleeper
    private let fileURL: URL?

    private var ops: [SyncOperation] = []
    /// 仮 id → サーバー id(create 送信後に同じレコードへ update/delete が来た時に引き直す)
    private var aliases: [String: String] = [:]
    private var inFlight: String?
    private var draining = false
    private(set) public var isPaused = false
    private var listeners: [UUID: AsyncStream<SyncEvent>.Continuation] = [:]

    /// - Parameters:
    ///   - directory: `queue.json` の置き場。nil ならメモリのみ(テスト用)
    ///   - sleep: バックオフ待ちの差し替え口(テストでは待ち時間を記録するだけにする)
    public init(directory: URL?, store: RecordStore, client: any AppConfigClient,
                policy: BackoffPolicy = BackoffPolicy(), sleep: @escaping Sleeper = { try await Task.sleep(for: $0) }) {
        self.store = store
        self.client = client
        self.policy = policy
        self.sleep = sleep
        self.fileURL = directory?.appendingPathComponent("queue.json")
        if let fileURL, let data = try? Data(contentsOf: fileURL),
           let saved = try? JSONDecoder().decode(Persisted.self, from: data) {
            ops = saved.ops
            aliases = saved.aliases
        }
    }

    private struct Persisted: Codable {
        var ops: [SyncOperation]
        var aliases: [String: String]
    }

    public var count: Int { ops.count }
    public var isDraining: Bool { draining }
    public var operations: [SyncOperation] { ops }

    public func events() -> AsyncStream<SyncEvent> {
        AsyncStream { cont in
            let id = UUID()
            listeners[id] = cont
            cont.onTermination = { [weak self] _ in Task { await self?.removeListener(id) } }
        }
    }

    private func removeListener(_ id: UUID) { listeners[id] = nil }
    private func emit(_ e: SyncEvent) { listeners.values.forEach { $0.yield(e) } }

    // MARK: - Enqueue

    @discardableResult
    public func enqueue(_ op: SyncOperation) -> EnqueueResult {
        defer { persist() }
        let same = { (o: SyncOperation) in o.appId == op.appId && o.entity == op.entity && o.recordId == op.recordId && o.id != self.inFlight }
        switch op.kind {
        case .update:
            if let i = ops.firstIndex(where: { same($0) && ($0.kind == .create || $0.kind == .update) }) {
                ops[i].data.merge(op.data) { _, new in new }
                return .merged
            }
        case .delete:
            if let i = ops.firstIndex(where: { same($0) && $0.kind == .create }) {
                ops.remove(at: i)
                ops.removeAll { same($0) && $0.kind == .update }
                return .cancelledCreate
            }
            ops.removeAll { same($0) && $0.kind == .update }
        case .create, .none:
            break
        }
        ops.append(op)
        return .appended
    }

    // MARK: - Drain

    /// 停止(`isPaused`)を解除して流す。オンライン復帰・前面復帰・「今すぐ同期」から呼ぶ。
    public func resume() async {
        isPaused = false
        if !ops.isEmpty { ops[0].attempts = 0 }
        await drain()
    }

    /// 先頭から順に送る。空になるか、連続失敗で停止するまで戻らない。同時に 2 本は走らない。
    public func drain() async {
        guard !draining, !isPaused else { return }
        draining = true
        defer { draining = false }
        while let op = ops.first {
            inFlight = op.id
            defer { inFlight = nil }
            do {
                try await perform(op)
                finish(op)
                emit(.sent(appId: op.appId, entity: op.entity))
            } catch {
                switch Self.classify(error) {
                case .conflict:
                    await store.revertPending(appId: op.appId, entity: op.entity, recordId: op.recordId)
                    finish(op)
                    emit(.conflict(appId: op.appId, entity: op.entity, message: (error as? APIError)?.errorDescription ?? "\(error)"))
                case .rejected(let message):
                    await store.revertPending(appId: op.appId, entity: op.entity, recordId: op.recordId)
                    finish(op)
                    emit(.rejected(appId: op.appId, entity: op.entity, message: message))
                case .transient:
                    ops[0].attempts += 1
                    persist()
                    let attempt = ops[0].attempts
                    print("[Metamon] sync \(op.kind.rawValue) \(op.entity)/\(op.recordId) failed (attempt \(attempt)/\(policy.maxAttempts)): \(error)")
                    if attempt >= policy.maxAttempts {
                        isPaused = true
                        emit(.paused(pending: ops.count))
                        return
                    }
                    do { try await sleep(policy.delay(attempt: attempt)) } catch { return } // キャンセルされたら次のトリガーで続き
                }
            }
        }
        emit(.drained)
    }

    private func finish(_ op: SyncOperation) {
        ops.removeAll { $0.id == op.id }
        persist()
    }

    private func perform(_ op: SyncOperation) async throws {
        let serverId = aliases[op.recordId] ?? op.recordId
        switch op.kind {
        case .create:
            let rec = try await client.create(appId: op.appId, entity: op.entity, data: op.data, clientId: op.id)
            aliases[op.recordId] = rec.id
            await store.markSynced(appId: op.appId, entity: op.entity, localId: op.recordId, server: rec)
        case .update:
            let rec = try await client.update(appId: op.appId, entity: op.entity, id: serverId, data: op.data)
            await store.markSynced(appId: op.appId, entity: op.entity, localId: op.recordId, server: rec)
        case .delete:
            do {
                try await client.delete(appId: op.appId, entity: op.entity, id: serverId)
            } catch let e as APIError where e.status == 404 {
                // もう無い = 目的は達成
            }
            await store.remove(appId: op.appId, entity: op.entity, recordId: op.recordId)
            aliases[op.recordId] = nil
        case .none:
            break
        }
    }

    enum Outcome: Equatable { case transient, conflict, rejected(String) }

    /// 通信断・5xx・401(トークン更新待ち)・429 は再試行。404/409/412 は競合(サーバー優先)。他の 4xx は内容の拒否。
    static func classify(_ error: Error) -> Outcome {
        guard let api = error as? APIError, let status = api.status else { return .transient }
        switch status {
        case 404, 409, 412: return .conflict
        case 401, 408, 425, 429: return .transient
        case 400..<500: return .rejected(api.errorDescription ?? "HTTP \(status)")
        default: return .transient
        }
    }

    private func persist() {
        guard let fileURL else { return }
        do {
            try FileManager.default.createDirectory(at: fileURL.deletingLastPathComponent(), withIntermediateDirectories: true)
            try JSONEncoder().encode(Persisted(ops: ops, aliases: aliases)).write(to: fileURL, options: .atomic)
        } catch {
            print("[Metamon] SyncQueue persist failed: \(error)")
        }
    }
}
