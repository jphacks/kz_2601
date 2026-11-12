import Foundation
import Observation
import MetamonDSL
import MetamonServices

/// Mac の App Intents(#1034)と本体の橋渡し。Intent の型・Query・フレーズは iOS と同じファイル
/// (`Metamon/Features/Intents/`)を使い、そこから呼ばれる `IntentHost` の口だけ Mac 用に持つ。
///
/// - 一覧: `MacAppLibrary`(作ったアプリ・サーバーのアプリ)と同梱の例。`MetamonMacApp` が起動時に `register` する
/// - 開く / 話しかける: `pending` に積み、`MacRootView`(`macIntentRouting`)が受け取って一覧の選択・⇧⌘M の会話につなぐ
/// - 記録の追加: 端末に残る作ったアプリだけ。中央と同じ `MacRecordEngines` のエンジンに書く(画面を開かない)
@MainActor
@Observable
final class IntentHost {
    static var shared = IntentHost()

    enum Pending: Equatable {
        case open(appId: String)
        case talk
    }

    /// 記録を足した(`object` は appId・`userInfo["entity"]`)。そのアプリを中央に出していれば読み直す
    static let recordAdded = Notification.Name("metamon.intent.recordAdded")

    @ObservationIgnored private(set) var library: MacAppLibrary?
    @ObservationIgnored private(set) var bundled: [SampleApp] = []
    @ObservationIgnored let records: any MacIntentRecordWriting
    @ObservationIgnored private let server: @MainActor () -> (any AppConfigClient)?
    @ObservationIgnored private var refreshTask: Task<Void, Never>?
    /// ウインドウが受け取る前の「開く / 話しかける」(ウインドウがまだ無ければ出来てから受け取る)
    private(set) var pending: Pending?

    /// `records` を省くと中央と同じエンジンに書く(`MacIntentEngineWriter`)
    init(records: (any MacIntentRecordWriting)? = nil,
         server: @escaping @MainActor () -> (any AppConfigClient)? = { MacServices.shared.appConfig }) {
        self.records = records ?? MacIntentEngineWriter()
        self.server = server
    }

    func register(library: MacAppLibrary, bundled: [SampleApp]) {
        self.library = library
        self.bundled = bundled
        #if DEBUG
        IntentE2E.runIfRequested()
        #endif
        appsMayHaveChanged()
    }

    // MARK: - 開く / 話しかける

    func request(_ action: Pending) async {
        print("[Metamon] intent: requested \(action)")
        pending = action
    }

    func take() -> Pending? {
        defer { pending = nil }
        return pending
    }

    /// 一覧が変わった(作った・名前を変えた・消した)ら、Siri のパラメータ付きフレーズを引き直す(2 秒まとめる)
    func appsMayHaveChanged() {
        refreshTask?.cancel()
        refreshTask = Task {
            try? await Task.sleep(for: .seconds(2))
            guard !Task.isCancelled else { return }
            TailorItShortcuts.updateAppShortcutParameters()
        }
    }

    // MARK: - カタログ(Entity の候補)

    /// サイドバーと同じ並び: 作ったアプリ → 同梱の例(直した例は直した名前)
    func apps() async -> [AppSummary] {
        guard let library else { return [] }
        let examples = bundled.map { app in
            AppSummary(id: app.id, name: library.generated[app.id]?.dsl.app.name ?? app.name, currentVersion: 0)
        }
        return Self.created(in: library) + examples
    }

    /// 記録先は端末に残る作ったアプリだけ(同梱の例の記録はメモリ上で、ウインドウを閉じると消える)
    func recordableApps() async -> [AppSummary] {
        guard let library else { return [] }
        return Self.created(in: library)
    }

    private static func created(in library: MacAppLibrary) -> [AppSummary] {
        library.createdIds.compactMap { id in
            if let document = library.generated[id] {
                return AppSummary(id: id, name: document.dsl.app.name, currentVersion: document.version)
            }
            return library.serverSummary(id)
        }
    }

    /// アプリの DSL。中身をまだ取っていないサーバーのアプリは取ってから
    func document(appId: String) async -> AppDocument? {
        guard let library else { return nil }
        if library.isServerOnly(appId) { await library.download(appId) }
        if let document = library.generated[appId] { return document }
        guard let app = bundled.first(where: { $0.id == appId }), let dsl = try? app.loadDSL() else { return nil }
        return AppDocument(appId: appId, version: 0, dsl: dsl)
    }

    // MARK: - 記録の追加

    var canWriteRecords: Bool { library != nil }

    func cachedRecords(appId: String, entity: String) async -> [Record] {
        await records.cached(appId: appId, entity: entity, server: serverClient(for: appId))
    }

    /// 1 件足す。戻り値はサーバー(端末だけのアプリは端末の appconfig)へ送り終えたか
    func addRecord(appId: String, entity: String, data: [String: JSONValue],
                   encryptedFields: [String: Set<String>]) async throws -> Bool {
        guard let library else { throw TailorIntentError.notReady }
        guard library.isCreated(appId) else { throw TailorIntentError.targetNotFound(appId) }
        let server = serverClient(for: appId)
        // 暗号化(#233)はサーバーに送る時だけ(中央の `AppCanvas` と同じ)
        let sent = await records.add(appId: appId, entity: entity, data: data,
                                     encryptedFields: server == nil ? [:] : encryptedFields, server: server)
        print("[Metamon] intent: record added appId=\(appId) entity=\(entity)")
        NotificationCenter.default.post(name: Self.recordAdded, object: appId, userInfo: ["entity": entity])
        return sent
    }

    private func serverClient(for appId: String) -> (any AppConfigClient)? {
        guard let library, library.isCreated(appId), library.isOnServer(appId) else { return nil }
        return server()
    }
}

// MARK: - 記録の書き込み

/// Intent から記録を読み書きする口(テストでは偽物に替える)
@MainActor
protocol MacIntentRecordWriting {
    func cached(appId: String, entity: String, server: (any AppConfigClient)?) async -> [Record]
    func add(appId: String, entity: String, data: [String: JSONValue],
             encryptedFields: [String: Set<String>], server: (any AppConfigClient)?) async -> Bool
}

/// 中央と同じエンジン(`MacRecordEngines`)を借りて書く。同じアプリを開いたウインドウがあればそのエンジンをそのまま使う
@MainActor
struct MacIntentEngineWriter: MacIntentRecordWriting {
    var engines: MacRecordEngines = .shared
    /// 返す前に送り終えるのを待つ長さ(iOS の `addRecordFromIntent` と同じ)
    var wait: Duration = .seconds(4)

    func cached(appId: String, entity: String, server: (any AppConfigClient)?) async -> [Record] {
        let lease = engines.acquire(appId: appId, server: server)
        defer { lease.release() }
        return await lease.engine.cached(appId: appId, entity: entity)
    }

    func add(appId: String, entity: String, data: [String: JSONValue],
             encryptedFields: [String: Set<String>], server: (any AppConfigClient)?) async -> Bool {
        let lease = engines.acquire(appId: appId, server: server)
        defer { lease.release() } // 送り終わらなくても列はファイルに残り、`MacRecordResend` が送り直す
        let engine = lease.engine
        if !encryptedFields.isEmpty { engine.encryptedFields[appId] = encryptedFields }
        _ = await engine.save(appId: appId, entity: entity, recordId: nil, data: data)
        guard engine.connectivity.isOnline else { return false }
        let deadline = ContinuousClock.now + wait
        while ContinuousClock.now < deadline {
            if await engine.queue.count == 0 { return true }
            try? await Task.sleep(for: .milliseconds(200))
        }
        return false
    }
}
