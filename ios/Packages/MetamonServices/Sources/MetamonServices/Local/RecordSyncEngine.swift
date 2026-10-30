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
        // 動画(#206)は既定なら Application Support/metamon-videos、テストで directory を渡したらその下
        let videos = directory.map { LocalVideoStore(directory: $0.appendingPathComponent("videos", isDirectory: true)) } ?? .shared
        let audios = directory.map { LocalAudioStore(directory: $0.appendingPathComponent("audio", isDirectory: true)) } ?? .shared
        let store = RecordStore(directory: dir, videos: videos, audios: audios)
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

    /// 表示名を送らなかった投稿に backend が付ける名前(appconfig `defaultAuthorName`)。送信前のローカル行にも同じものを出す。
    nonisolated public static let anonymousName = "匿名"

    /// 作成(`recordId == nil`)または更新。ローカルに即反映し、更新後の一覧を返す。送信はキューが行う。
    /// - Parameter author: shared / public の entity に作る時の投稿者(#250)。uid はローカル行の作者判定と「自分の投稿」用、
    ///   name は POST で送る表示名(nil なら送らない)。更新では使わない。
    public func save(appId: String, entity: String, recordId: String?, data: [String: JSONValue],
                     author: (uid: String?, name: String?)? = nil) async -> [Record] {
        let now = ISO8601DateFormatter().string(from: .now)
        if let recordId, var row = await store.row(appId: appId, entity: entity, recordId: recordId) {
            let oldVideos = VideoValue.localIDs(in: row.data)
            let oldAudios = AudioValue.localIDs(in: row.data)
            row.data.merge(data) { _, new in new } // PATCH と同じくキー単位でマージ(DSL から消えた項目も保持)
            store.videos.remove(oldVideos.subtracting(VideoValue.localIDs(in: row.data))) // 撮り直し・削除した動画(#206)
            store.audios.remove(oldAudios.subtracting(AudioValue.localIDs(in: row.data))) // 録り直し・削除した録音(#420)
            row.updatedAt = now
            if row.pendingOp != .create { row.pendingOp = .update }
            await store.upsert(row)
            await queue.enqueue(SyncOperation(appId: appId, entity: entity, recordId: recordId, kind: .update, data: data))
        } else {
            let clientId = UUID().uuidString.lowercased()
            let row = LocalRecord(appId: appId, entity: entity, recordId: LocalRecord.temporaryId(clientId: clientId), data: data,
                                  createdAt: now, updatedAt: now, pendingOp: .create, clientId: clientId,
                                  author: author?.uid, authorName: author.map { $0.name ?? Self.anonymousName })
            await store.upsert(row)
            await queue.enqueue(SyncOperation(id: clientId, appId: appId, entity: entity, recordId: row.recordId, kind: .create, data: data,
                                              authorName: author?.name))
        }
        kick()
        return await store.records(appId: appId, entity: entity)
    }

    /// 印を付ける / 外す(#255)。押した瞬間にローカルの件数と状態を変え、サーバーの `{count, mine}` で確定する。
    /// 失敗したら元に戻して投げる。印は送信待ちの列に積まない(オフライン・未送信の自分の投稿では `MarkError`)。
    @discardableResult
    public func toggleMark(appId: String, entity: String, recordId: String, mark: String) async throws -> MarkState {
        guard let row = await store.row(appId: appId, entity: entity, recordId: recordId), row.pendingOp != .delete else {
            throw MarkError.notFound
        }
        guard !row.isTemporary else { throw MarkError.notSynced }
        guard isOnline else { throw MarkError.offline }
        let before = row.record.markState(mark)
        let next = before.toggled()
        await store.setMark(appId: appId, entity: entity, recordId: recordId, mark: mark, state: next)
        do {
            let server = try await client.setMark(appId: appId, entity: entity, id: recordId, mark: mark, on: next.mine)
            await store.setMark(appId: appId, entity: entity, recordId: recordId, mark: mark, state: server)
            return server
        } catch {
            await store.setMark(appId: appId, entity: entity, recordId: recordId, mark: mark, state: before)
            throw error
        }
    }

    /// アプリ削除(#180): その appId の未送信キューとローカル DB を消す。サーバー削除が成功してから呼ぶ。
    public func purge(appId: String) async {
        await queue.discard(appId: appId)
        await store.removeAll(appId: appId)
        await refreshStatus()
    }

    /// 削除。ローカルでは即座に一覧から消え(行は delete 待ちで残る)、送信成功で行も消える。
    public func delete(appId: String, entity: String, recordId: String) async -> [Record] {
        if let row = await store.row(appId: appId, entity: entity, recordId: recordId) {
            store.videos.remove(VideoValue.localIDs(in: row.data)) // 端末内の動画ファイル(#206)
            store.audios.remove(AudioValue.localIDs(in: row.data)) // 端末内の録音(#420)
        }
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

/// 印を付けられない理由(#255)。
public enum MarkError: Error, Equatable, Sendable {
    /// オフライン(印は送信待ちに積まない)
    case offline
    /// 自分の投稿がまだ送信されていない(サーバーにレコードが無い)
    case notSynced
    /// ローカルにレコードが無い(削除済み)
    case notFound
}
