import AppIntents
import MetamonDSL
import MetamonServices

/// App Intents(#80)と本体の橋渡し。Intent は本体プロセス内で動く(拡張ターゲットではない)ので、
/// `MetamonApp` が作った唯一の `AppRuntime` / `RecordSyncEngine` をそのまま使う(ローカル DB を二重に開かない)。
///
/// - 記録の追加: 本体が背面起動されただけ(画面なし・bootstrap 前)でも `runtime` は居るので、そのまま書ける
/// - 開く/話しかける: 画面の準備(bootstrap 完了)前に来たら `pending` に積み、`markReady()` で実行する
///   (bootstrap の復元 `GET /dsl` と切替が競合して前のアプリに戻るのを避ける)
@MainActor
final class IntentHost {
    static let shared = IntentHost()

    enum Pending: Equatable {
        case open(appId: String)
        case talk
        #if !APP_STORE_REVIEW
        /// 通知のタップ(#269)。すでに開いているアプリなら何も言わない
        case openFromNotification(appId: String)
        /// 連係の Handoff(#1851)。そのアプリに切り替えてから画面と記録を開く
        case handoff(HandoffTarget)
        #endif
    }

    private(set) var runtime: AppRuntime?
    private(set) var isReady = false
    private var pending: Pending?
    private var refreshTask: Task<Void, Never>?

    func register(_ runtime: AppRuntime) {
        self.runtime = runtime
        #if DEBUG
        IntentE2E.runIfRequested()
        #endif
    }

    /// bootstrap 完了で呼ぶ(`MetamonApp`)。保留中の「開く/話しかける」を実行し、Siri のフレーズ候補(アプリ名)を更新する。
    func markReady() {
        isReady = true
        if let next = pending {
            pending = nil
            Task { await perform(next) }
        }
        appsMayHaveChanged()
    }

    /// 開く/話しかける: 準備済みなら今すぐ、まだなら bootstrap 後に。
    func request(_ action: Pending) async {
        guard isReady else {
            pending = action
            print("[Metamon] intent: queued \(action) until bootstrap")
            return
        }
        await perform(action)
    }

    private func perform(_ action: Pending) async {
        await SplashPresence.shared.waitUntilHidden() // 通知・Siri から開いた時もスプラッシュの後に
        guard let runtime else { return }
        switch action {
        case .open(let appId): await runtime.openFromIntent(appId: appId)
        case .talk: runtime.openVoiceFromIntent()
        #if !APP_STORE_REVIEW
        case .openFromNotification(let appId):
            if appId != runtime.document.appId { await runtime.switchApp(to: appId) }
        case .handoff(let target): await runtime.openHandoff(target)
        #endif
        }
    }

    /// DSL が差し替わった(生成・切替・アプリ名変更)ら、Siri のパラメータ付きフレーズ(「Tailor It で家計簿を開く」)を引き直す。
    /// `updateAppShortcutParameters()` は各 Entity の suggestedEntities を呼ぶ(=`GET /apps`)ので 2 秒まとめる。
    func appsMayHaveChanged() {
        refreshTask?.cancel()
        refreshTask = Task {
            try? await Task.sleep(for: .seconds(2))
            guard !Task.isCancelled else { return }
            TailorItShortcuts.updateAppShortcutParameters()
        }
    }

    // MARK: - 記録の追加(AddRecordIntent)。Mac は同じ名前の口を MacIntentHost.swift に持つ(#1034)

    /// 背面起動(bootstrap 前)でも `runtime` があれば書ける
    var canWriteRecords: Bool { runtime != nil }

    /// ref 項目の候補(参照先の記録)をローカル DB から読む
    func cachedRecords(appId: String, entity: String) async -> [Record] {
        await runtime?.recordSync.cached(appId: appId, entity: entity) ?? []
    }

    /// 1 件足す。戻り値はサーバーへ送り終えたか
    func addRecord(appId: String, entity: String, data: [String: JSONValue],
                   encryptedFields: [String: Set<String>]) async throws -> Bool {
        guard let runtime else { throw TailorIntentError.notReady }
        runtime.recordSync.encryptedFields[appId] = encryptedFields
        return await runtime.addRecordFromIntent(appId: appId, entity: entity, data: data)
    }

    // MARK: - カタログ(Entity の候補)

    /// 作ったアプリ(変身一覧と同じ並び・未生成は除く)。サーバーに届かなければ `AppListCache`。
    func apps() async -> [AppSummary] {
        let all: [AppSummary]
        if let runtime {
            do {
                all = try await runtime.services.appConfig.listApps()
            } catch {
                print("[Metamon] intent: listApps failed, using cache: \(error)")
                all = AppListCache.shared.load()
            }
        } else {
            all = AppListCache.shared.load()
        }
        return AppSummary.visible(all, current: nil)
    }

    /// 記録先の候補になるアプリ。iOS は作ったアプリ全部(Mac は端末に残る作ったアプリだけ)
    func recordableApps() async -> [AppSummary] { await apps() }

    /// アプリの DSL。表示中ならそれ、無ければキャッシュ(`DocumentCache`)、それも無ければ `GET /dsl`。
    func document(appId: String) async -> AppDocument? {
        if let runtime, runtime.document.appId == appId, !runtime.dsl.isVoiceOnly { return runtime.document }
        if let cached = DocumentCache.shared.load(appId: appId) { return cached }
        return try? await runtime?.services.appConfig.fetchDocument(appId: appId)
    }
}
