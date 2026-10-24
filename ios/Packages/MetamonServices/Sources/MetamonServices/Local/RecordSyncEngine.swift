import Foundation
import Observation
import MetamonDSL

/// ローカル DB(`RecordStore`)+ 未送信キュー(`SyncQueue`)+ 到達性(`ConnectivityMonitor`)をまとめ、
/// `AppRuntime` に「ローカル先出し → サーバー差し替え」と「楽観的な書き込み」を提供する。
///
/// 読み: `cached` を即返し、続けて `pull`(サーバー全件 → ローカル置換)。
/// 書き: `save` / `delete` はローカルに即反映して `[Record]` を返し、裏でキューが appconfig へ送る。
/// サーバーに `updatedAfter` フィルタが無い(`buildFilter` は entity の Field しか受けない)ため、差分ではなく全件(≤1000 件/entity)で置換する。
@MainActor
@Observable
public final class RecordSyncEngine {
    public enum Status: Equatable, Sendable {
        case synced
        case pending(Int)
        case syncing(Int)
        case offline(pending: Int)

        public var label: String {
            switch self {
            case .synced: "最新"
            case .pending(let n): "未送信 \(n) 件"
            case .syncing(let n): "送信中(\(n) 件)"
            case .offline(let n): n == 0 ? "オフライン" : "オフライン・未送信 \(n) 件"
            }
        }
    }

    public private(set) var status: Status = .synced
    public private(set) var pendingCount = 0
    public private(set) var isOnline = true
    /// 競合・拒否・送信完了の通知(AppRuntime が toast と再読込に使う)
    public var onEvent: (@MainActor (SyncEvent) -> Void)?

    @ObservationIgnored public let store: RecordStore
    @ObservationIgnored public let queue: SyncQueue
    @ObservationIgnored private let client: any AppConfigClient
    /// 到達性。`AppRuntime` が生成の圏外判定・復帰待ち(#60)にも使う(E2E の機内モード擬似もここに入る)
    @ObservationIgnored public let connectivity: any ConnectivityMonitor
    @ObservationIgnored private var tasks: [Task<Void, Never>] = []
    @ObservationIgnored private var isForeground = true
    @ObservationIgnored private var started = false

    public init(store: RecordStore, queue: SyncQueue, client: any AppConfigClient, connectivity: any ConnectivityMonitor) {
        self.store = store
        self.queue = queue
        self.client = client
        self.connectivity = connectivity
        self.isOnline = connectivity.isOnline
    }

    /// 既定構成: Application Support/metamon-records にファイル永続化、`NWPathMonitor` で到達性監視。
    public convenience init(client: any AppConfigClient, connectivity: any ConnectivityMonitor = NetworkPathConnectivity(), directory: URL? = nil) {
        let base = FileManager.default.urls(for: .applicationSupportDirectory, in: .userDomainMask).first
            ?? FileManager.default.temporaryDirectory
        let dir = directory ?? base.appendingPathComponent("metamon-records", isDirectory: true)
        let store = RecordStore(directory: dir)
        self.init(store: store, queue: SyncQueue(directory: dir, store: store, client: client), client: client, connectivity: connectivity)
    }

    // MARK: - Lifecycle

    /// 到達性とキューのイベントを購読し、未送信があれば流し始める。2 回呼んでも 1 回だけ。
    public func start() {
        guard !started else { return }
        started = true
        tasks.append(Task { [weak self] in
            guard let self else { return }
            for await online in connectivity.updates() {
                self.isOnline = online
                await self.refreshStatus()
                if online { self.kick() }
            }
        })
        tasks.append(Task { [weak self] in
            guard let self else { return }
            for await event in await queue.events() {
                await self.refreshStatus()
                self.onEvent?(event)
            }
        })
        kick()
    }

    public func stop() {
        tasks.forEach { $0.cancel() }
        tasks.removeAll()
        started = false
    }

    /// アカウント削除用(#119): 購読を止め、未送信キューとローカル DB を空にする。次の `start()` で再開できる。
    public func wipe() async {
        stop()
        await queue.discardAll()
        await store.removeEverything()
        await refreshStatus()
    }

    /// 前面復帰でも未送信を流す(背面でネットワークが戻っていた場合の取りこぼし防止)。
    public func setForeground(_ active: Bool) {
        isForeground = active
        if active { kick() }
    }

    /// 設定シートの「今すぐ同期」。
    public func syncNow() { kick() }

    /// オンラインならキューを(停止中でも)流し直す。書き込み・復帰・手動同期のすべてがここを通る。
    private func kick() {
        Task { [weak self] in await self?.refreshStatus() }
        guard isOnline else { return }
        Task { [weak self] in
            guard let self else { return }
            await self.queue.resume()
            await self.refreshStatus()
        }
    }

    private func refreshStatus() async {
        let n = await queue.count
        let draining = await queue.isDraining
        pendingCount = n
        if !isOnline { status = .offline(pending: n) }
        else if draining { status = .syncing(n) }
        else if n > 0 { status = .pending(n) }
        else { status = .synced }
    }

    // MARK: - Read

    /// ローカル DB の内容(即時)。削除待ちは除く。
    public func cached(appId: String, entity: String) async -> [Record] {
        await store.records(appId: appId, entity: entity)
    }

    /// サーバー全件を取り、ローカルを置換(未送信行は残す)して返す。失敗はそのまま投げる(呼び出し側がローカル表示を続ける)。
    public func pull(appId: String, entity: String) async throws -> [Record] {
        let server = try await client.list(appId: appId, entity: entity)
        return await store.replace(appId: appId, entity: entity, with: server)
    }

    // MARK: - Write(楽観的)

    /// 作成(`recordId == nil`)または更新。ローカルに即反映し、更新後の一覧を返す。送信はキューが行う。
    public func save(appId: String, entity: String, recordId: String?, data: [String: JSONValue]) async -> [Record] {
        let now = ISO8601DateFormatter().string(from: .now)
        if let recordId, var row = await store.row(appId: appId, entity: entity, recordId: recordId) {
            row.data.merge(data) { _, new in new } // PATCH と同じくキー単位でマージ(DSL から消えた項目も保持)
            row.updatedAt = now
            if row.pendingOp != .create { row.pendingOp = .update }
            await store.upsert(row)
            await queue.enqueue(SyncOperation(appId: appId, entity: entity, recordId: recordId, kind: .update, data: data))
        } else {
            let clientId = UUID().uuidString.lowercased()
            let row = LocalRecord(appId: appId, entity: entity, recordId: LocalRecord.temporaryId(clientId: clientId), data: data,
                                  createdAt: now, updatedAt: now, pendingOp: .create, clientId: clientId)
            await store.upsert(row)
            await queue.enqueue(SyncOperation(id: clientId, appId: appId, entity: entity, recordId: row.recordId, kind: .create, data: data))
        }
        kick()
        return await store.records(appId: appId, entity: entity)
    }

    /// 削除。ローカルでは即座に一覧から消え(行は delete 待ちで残る)、送信成功で行も消える。
    public func delete(appId: String, entity: String, recordId: String) async -> [Record] {
        let result = await queue.enqueue(SyncOperation(appId: appId, entity: entity, recordId: recordId, kind: .delete))
        if result == .cancelledCreate {
            await store.remove(appId: appId, entity: entity, recordId: recordId) // まだサーバーに無い行はその場で消す
        } else if var row = await store.row(appId: appId, entity: entity, recordId: recordId) {
            row.pendingOp = .delete
            await store.upsert(row)
        }
        kick()
        return await store.records(appId: appId, entity: entity)
    }
}
