import Foundation
import Observation
import MetamonDSL
import MetamonServices

/// 会話で作った・直したアプリの DSL と、その版の「元に戻す / やり直す」(#967・#968)、作ったアプリの一覧の整理(#969)・複製(#1285)。
/// - 作ったアプリは端末に残す(`LocalAppLibrary`: iOS のキャッシュと同じ置き方)。サーバー(#930)が入るまでは端末だけ
/// - 同梱の例を会話で直した分も端末に残す(#1283)。右クリックの「元の例に戻す」で消す。版の履歴はアプリを終えると消える
/// - 版の nil は「同梱の例のまま(会話で直す前)」。同梱の例を直して戻すと、元の例を表示し直す
/// - サーバーにあるアプリの「元に戻す / やり直す」は、iOS と同じくサーバーの版(`GET /versions`・`POST /versions/{n}/restore`)で戻す。
///   届かない時は端末の履歴(`LocalVersionHistory`)で戻す(#1279)
/// - 本物の AI で作った・直したアプリはサーバー(appconfig)にも保存される。その印(`isOnServer`)を端末に残し、
///   記録をサーバーに読み書きするかを決める(#1284 の続き)。端末で複製したアプリはサーバーに無いので端末だけ
/// - サインインしたらサーバーの自分のアプリ(iOS の変身一覧と同じ `GET /v1/apps`・同じ並び)も一覧に混ぜる(#1057)。
///   起動直後は前回の一覧(`AppListCache`)をすぐ出し、取り直したら差し替える。中身(DSL)は開いた時に取る
/// ウインドウをまたいで 1 つだけ持つ(`MetamonMacApp` が作って渡す)。
@MainActor
@Observable
final class MacAppLibrary {
    struct Version {
        var document: AppDocument
        /// 端末に残す形(封筒)。無い版(古い結果など)は残さない
        var envelope: Data?
    }

    private(set) var generated: [String: AppDocument] = [:]
    /// 作ったアプリの id(一覧の並び・上から)
    private(set) var createdIds: [String] = []
    private var envelopes: [String: Data] = [:]
    /// 一覧に残す名前・版。端末の DSL と揃える(封筒の無い版を当てても進めない)
    private var savedRows: [String: LocalAppLibrary.Row] = [:]
    private var histories: [String: LocalVersionHistory<Version?>] = [:]
    /// サーバーにあるアプリの id(本物の AI の結果を当てた)
    private(set) var serverIds: Set<String>
    /// サーバーの版で戻した後の「やり直す」先(戻す前の版の番号・iOS の `RedoPoint.viaServer`)。新しい版を当てたら消す
    private var serverRedo: [String: Int] = [:]
    /// サーバーで戻している途中(二重に押さない)
    private(set) var restoring: Set<String> = []
    @ObservationIgnored private let store: LocalAppLibrary?
    @ObservationIgnored private let defaults: UserDefaults?
    /// サーバーの版(#1279)。nil(モック)なら端末の履歴だけ
    @ObservationIgnored private let server: (any AppConfigClient)?
    /// サーバーから受けた版の封筒(端末に残す形)。既定は `HTTPAppConfigClient` が書く `DocumentCache`
    @ObservationIgnored private let serverEnvelope: (String) -> Data?
    /// 設定の「使用量」(作ったアプリの数)。作った・消した時に古くする(iOS は `AppSwitcherSheet` が同じことをする)
    @ObservationIgnored private let usage: PlanUsageStore
    /// サーバーの自分のアプリ(#1057)。iOS の変身一覧と同じ並び(`AppSummary.visible`)
    private(set) var serverApps: [AppSummary] = []
    /// サーバーの一覧の読み込み(#1057)
    private(set) var serverListState: ServerListState = .idle
    /// 中身(DSL)を取っている途中・取れなかったサーバーのアプリ
    private(set) var downloading: Set<String> = []
    private(set) var downloadFailed: Set<String> = []
    /// 前回のサーバーの一覧(起動直後にすぐ出す)。nil なら使わない(テスト)
    @ObservationIgnored private let serverCache: AppListCache?
    /// 取り直しの途中(前面に戻る・ウインドウごとに頼まれても 1 回にまとめる)
    @ObservationIgnored private var refreshTask: Task<Void, Never>?
    static let serverIdsKey = "metamon.mac.serverAppIds"

    /// `store` が nil なら端末に残さない(プレビュー・テスト用)。`defaults` はサーバーにあるアプリの印の置き場(nil なら残さない)。
    /// `server` はサーバーの版で戻す時の相手(nil なら端末の履歴だけ)
    init(store: LocalAppLibrary? = .standard, defaults: UserDefaults? = nil, server: (any AppConfigClient)? = nil,
         serverEnvelope: @escaping (String) -> Data? = { DocumentCache.shared.loadData(appId: $0) },
         usage: PlanUsageStore = .shared, serverCache: AppListCache? = nil) {
        self.store = store
        self.serverCache = serverCache
        self.usage = usage
        self.defaults = defaults
        self.server = server
        self.serverEnvelope = serverEnvelope
        serverIds = Set(defaults?.stringArray(forKey: Self.serverIdsKey) ?? [])
        for entry in store?.load() ?? [] {
            generated[entry.summary.id] = entry.document
            envelopes[entry.summary.id] = entry.envelope
            savedRows[entry.summary.id] = .init(name: entry.document.dsl.app.name, version: entry.document.version)
            createdIds.append(entry.summary.id)
        }
        for (id, edited) in store?.loadEditedExamples() ?? [:] where generated[id] == nil {
            generated[id] = edited.document
            envelopes[id] = edited.envelope
        }
        if server != nil, let cached = serverCache?.load(), !cached.isEmpty { adoptServerList(cached) }
    }

    /// 会話で直した同梱の例(右クリックで「元の例に戻す」を出す)
    func isEditedExample(_ appId: String) -> Bool { !isCreated(appId) && generated[appId] != nil }

    /// 同梱の例を元に戻す(#1283)。端末に残した DSL と版の履歴を消す(確認はサイドバーで済んでいる)
    func resetExample(_ appId: String) {
        guard isEditedExample(appId) else { return }
        generated[appId] = nil
        envelopes[appId] = nil
        histories[appId] = nil
        store?.removeExample(appId: appId)
        markOnServer(appId, false)
    }

    func isCreated(_ appId: String) -> Bool { createdIds.contains(appId) }

    /// 端末に残した形(封筒)。フィードバックに添える DSL の全文を見せる(#1330)
    func envelope(for appId: String) -> Data? { envelopes[appId] }

    /// サーバー(appconfig)に保存されているアプリか。記録・版・フィードバックの添付をサーバーにつなぐかの判定
    func isOnServer(_ appId: String) -> Bool { serverIds.contains(appId) }

    /// 最後に開いていたアプリ(起動時に開き直す)
    var lastAppId: String? {
        get { store?.currentAppId }
        set { store?.currentAppId = newValue }
    }

    /// 会話の結果を当てる。作った直後の版には戻り先が無い(iOS も作った直後は「元に戻す」を出さない)。作ったアプリは一覧の先頭に足す。
    /// `onServer`: 本物の AI の結果(サーバーにも保存された)
    func apply(_ document: AppDocument, envelope: Data? = nil, created: Bool, onServer: Bool = false) {
        let id = document.appId
        if onServer { markOnServer(id, true) }
        serverRedo[id] = nil
        if !(created && generated[id] == nil) {
            histories[id, default: .init()].record(previous: current(id))
        }
        if created, !createdIds.contains(id) { createdIds.insert(id, at: 0) }
        set(id, Version(document: document, envelope: envelope))
        if created { usage.invalidateMe() }
    }

    /// 「元に戻す」を押せるか。サーバーにあるアプリは前の版がある(版が 2 以上)時も(iOS の `canUndo` と同じ)
    func canUndo(_ appId: String) -> Bool {
        guard !restoring.contains(appId) else { return false }
        if usesServerVersions(appId), (generated[appId]?.version ?? 0) > 1 { return true }
        return histories[appId]?.canUndo ?? false
    }

    func canRedo(_ appId: String) -> Bool {
        guard !restoring.contains(appId) else { return false }
        return serverRedo[appId] != nil || (histories[appId]?.canRedo ?? false)
    }

    /// 版を戻す・やり直すの結果
    enum VersionStep: Equatable {
        case moved
        /// 戻す先が無い(「これ以上前の状態はありません」)
        case nothing
        /// サーバーに届かず、端末の履歴でも戻せなかった
        case failed
    }

    /// 「元に戻す」(iOS の `AppRuntime.undo` と同じ順): サーバーにあるアプリは直前の版を restore、
    /// サーバーに届かない時は端末の履歴で戻す。restore は新しい版を作るので、やり直しは戻す前の版をもう一度 restore する
    func undo(_ appId: String) async -> VersionStep {
        guard !restoring.contains(appId) else { return .failed }
        if usesServerVersions(appId), let server {
            restoring.insert(appId)
            defer { restoring.remove(appId) }
            do {
                let versions = try await server.listVersions(appId: appId)
                guard let latest = versions.first else { throw APIError.server(status: 404, code: "not_found", message: "no versions") }
                guard let target = versions.first(where: { $0.version < latest.version }) else { return .nothing }
                let document = try await server.restore(appId: appId, toVersion: target.version)
                applyFromServer(document)
                serverRedo[appId] = latest.version
                return .moved
            } catch {
                print("[Metamon] Mac: undo via server failed, falling back to local history: \(error)")
            }
        }
        guard var history = histories[appId], let previous = history.undo(from: current(appId)) else { return .nothing }
        histories[appId] = history
        serverRedo[appId] = nil
        set(appId, previous)
        return .moved
    }

    /// 「やり直す」: サーバーで戻したなら戻す前の版を restore、端末の履歴で戻したなら当て直す
    func redo(_ appId: String) async -> VersionStep {
        guard !restoring.contains(appId) else { return .failed }
        if let version = serverRedo[appId], let server {
            restoring.insert(appId)
            defer { restoring.remove(appId) }
            do {
                let document = try await server.restore(appId: appId, toVersion: version)
                applyFromServer(document)
                serverRedo[appId] = nil
                return .moved
            } catch {
                print("[Metamon] Mac: redo failed: \(error)")
                return .failed
            }
        }
        guard var history = histories[appId], let next = history.redo(from: current(appId)) else { return .nothing }
        histories[appId] = history
        set(appId, next)
        return .moved
    }

    /// 競合(409)の後に取り直したサーバーの最新を当てる(iOS の `present(.conflict)` と同じ)。履歴には積まない。
    /// 当てた版を返す(取り直せなければ nil)
    func refreshFromServer(_ appId: String) async -> Int? {
        guard usesServerVersions(appId), let server, let latest = try? await server.fetchDocument(appId: appId) else { return nil }
        if latest.version != generated[appId]?.version { applyFromServer(latest) }
        return latest.version
    }

    // MARK: - 一覧の整理(#969・iOS の AppRuntime+AppList / +Delete)

    /// 名前を変える(作ったアプリだけ)。iOS と同じく版の履歴に積む(「元に戻す」で前の名前に戻せる)。
    /// 中身は変わらないので版の番号は上げない(中央を作り直さず、開いている記録をそのまま残す)。名前が空 / 長すぎなら false
    @discardableResult
    func rename(_ appId: String, to raw: String) -> Bool {
        guard let name = AppListEdit.normalizedName(raw) else { return false }
        if isServerOnly(appId), let index = serverApps.firstIndex(where: { $0.id == appId }) { // 中身を取る前のサーバーのアプリ
            serverApps[index].name = name
            renameOnServer(appId, name)
            return true
        }
        guard isCreated(appId), var document = generated[appId], document.dsl.app.name != name else { return true }
        if usesServerVersions(appId) { renameOnServer(appId, name) }
        histories[appId, default: .init()].record(previous: current(appId))
        serverRedo[appId] = nil
        document.dsl.app.name = name
        set(appId, Version(document: document, envelope: envelopes[appId].flatMap { DocumentEnvelope.renamed($0, to: name) }))
        return true
    }

    /// 削除する(作ったアプリだけ)。端末の DSL と版の履歴も消す
    func delete(_ appId: String) {
        guard let index = createdIds.firstIndex(of: appId) else { return }
        createdIds.remove(at: index)
        generated[appId] = nil
        envelopes[appId] = nil
        savedRows[appId] = nil
        histories[appId] = nil
        serverApps.removeAll { $0.id == appId } // 取り直すまでに一覧へ戻さない(サーバーで消すのは呼ぶ側)
        downloadFailed.remove(appId)
        store?.remove(appId: appId)
        markOnServer(appId, false)
        saveList()
        usage.invalidateMe()
    }

    /// 端末のデータを消した回数(開いているウインドウが選択を初期に戻す合図・#972)
    private(set) var wipeCount = 0

    /// サインアウト・アカウント削除(#972)。メモリ上の一覧・版の履歴を空にする(端末のファイルと UserDefaults は `MacLocalWipe` が消す)
    func resetAfterWipe() {
        wipeCount += 1
        generated = [:]
        createdIds = []
        envelopes = [:]
        savedRows = [:]
        histories = [:]
        serverIds = []
        serverRedo = [:]
        restoring = []
        serverApps = []
        serverListState = .idle
        downloading = []
        downloadFailed = []
        refreshTask?.cancel()
        refreshTask = nil
    }

    /// 複製する(#1285)。名前の付け方は iOS の `duplicateApp` と同じ(「○○のコピー」)。画面の定義だけを写し、記録は写さない。
    /// 作ったアプリは元のすぐ下、同梱の例は作ったアプリの先頭に置く。切り替えはしない(iOS と同じ)。複製したアプリの名前を返す
    @discardableResult
    func duplicate(_ appId: String, bundled: SampleApp? = nil) -> String? {
        let source = envelopes[appId] ?? bundled?.url.flatMap { url in
            (try? Data(contentsOf: url)).flatMap { DocumentEnvelope.make(appId: appId, version: 1, dsl: $0) }
        }
        guard let source, let original = generated[appId]?.dsl.app.name ?? bundled?.name else { return nil }
        let name = AppListEdit.copyName(of: original) { L10n.tr("\($0)のコピー") }
        let id = "mac-\(UUID().uuidString.prefix(8).lowercased())" // 会話で作るアプリと同じ形(MacComposeSession)
        guard let envelope = DocumentEnvelope.duplicated(source, appId: id, name: name),
              var document = try? AppDocument.decode(envelope) else { return nil }
        document.appId = id
        createdIds.insert(id, at: createdIds.firstIndex(of: appId).map { $0 + 1 } ?? 0)
        set(id, Version(document: document, envelope: envelope))
        return name
    }

    /// 1 つ足す前の持てるアプリの数の確かめ(#630)。iOS ではサーバーが 402 `app_limit_reached` で断るが、
    /// Mac はサーバーにつながっていない(#930)ので、作ったアプリの数とプランの数(`planApps`)をここで比べる。
    /// 足すと超えるなら置き換えシートに渡す上限を返す(nil = 足せる)。`planApps` が 0 なら数えない(サーバーと同じ)
    func limitForAdding(planApps: Int) -> AppLimit? {
        guard planApps > 0, createdIds.count >= planApps else { return nil }
        return AppLimit(held: createdIds.count, limit: planApps, upgrade: planApps < MacPlanApps.pro)
    }

    /// 作ったアプリの `index` 番目に動かす(右クリックの「上へ / 下へ」)
    func move(_ appId: String, to index: Int) {
        let next = AppListEdit.moved(createdIds, id: appId, to: index)
        guard next != createdIds else { return }
        createdIds = next
        saveList()
        syncServerOrder()
    }

    /// ドラッグで並べ替え(`List` の `onMove`)
    func move(fromOffsets source: IndexSet, toOffset destination: Int) {
        var next = createdIds
        next.move(fromOffsets: source, toOffset: destination)
        guard next != createdIds else { return }
        createdIds = next
        saveList()
        syncServerOrder()
    }

    // MARK: - サーバーの一覧(#1057・iOS の `appsList`)

    enum ServerListState: Equatable {
        case idle
        /// 前回の一覧が無く、初めて読んでいる(サイドバーにスケルトンを出す)
        case loading
        case loaded
        /// 届かなかった(前回の一覧のまま)
        case offline
    }

    /// サーバーの一覧の行(中身を取る前のアプリの名前・版)
    func serverSummary(_ appId: String) -> AppSummary? { serverApps.first { $0.id == appId } }

    /// サーバーにあり、まだ中身(DSL)を取っていないアプリ
    func isServerOnly(_ appId: String) -> Bool { generated[appId] == nil && serverSummary(appId) != nil }

    /// `GET /v1/apps` を取り直して一覧に混ぜる。モック(サーバー無し)は何もしない。途中なら同じ取り直しを待つ
    func refreshServerApps() async {
        guard let server else { return }
        if let refreshTask { return await refreshTask.value }
        let generation = wipeCount
        if serverApps.isEmpty { serverListState = .loading }
        let task = Task { @MainActor [weak self] in
            do {
                let list = try await server.listApps()
                guard let self, generation == self.wipeCount else { return } // 途中でサインアウトした
                self.adoptServerList(list)
                self.serverListState = .loaded
            } catch {
                print("[Metamon] Mac: listApps failed, keeping the saved list: \(error)")
                guard let self, generation == self.wipeCount else { return }
                self.serverListState = .offline
            }
        }
        refreshTask = task
        await task.value
        if generation == wipeCount { refreshTask = nil }
    }

    /// 中身を取る前のサーバーのアプリを開く(`GET /dsl`)。取れたら端末に残し、ほかの作ったアプリと同じに扱う
    func download(_ appId: String) async {
        guard let server, isServerOnly(appId), !downloading.contains(appId) else { return }
        let generation = wipeCount
        downloading.insert(appId)
        downloadFailed.remove(appId)
        defer { if generation == wipeCount { downloading.remove(appId) } }
        do {
            var document = try await server.fetchDocument(appId: appId)
            guard generation == wipeCount, isCreated(appId) else { return } // 途中で消した・サインアウトした
            document.appId = appId
            markOnServer(appId, true)
            applyFromServer(document)
        } catch {
            print("[Metamon] Mac: fetchDocument failed: \(error)")
            if generation == wipeCount { downloadFailed.insert(appId) }
        }
    }

    // MARK: - 内部

    private func usesServerVersions(_ appId: String) -> Bool { server != nil && isOnServer(appId) }

    /// サーバーの一覧を当てる: 並びはサーバー(iOS と同じ)、この Mac にだけあるアプリはその上に今の並びで残す
    private func adoptServerList(_ list: [AppSummary]) {
        let visible = AppSummary.visible(list, current: nil)
        serverApps = visible
        let ids = visible.map(\.id)
        if !Set(ids).isSubset(of: serverIds) {
            serverIds.formUnion(ids)
            defaults?.set(serverIds.sorted(), forKey: Self.serverIdsKey)
        }
        let merged = MacServerAppList.merged(local: createdIds, server: ids)
        guard merged != createdIds else { return }
        createdIds = merged
        saveList()
    }

    /// 並べ替えをサーバーにも送る(iOS の `reorderApps`・#277)。サーバーの一覧も先に並べ替える(取り直しで戻らない)
    private func syncServerOrder() {
        guard let server, !serverApps.isEmpty else { return }
        let ids = createdIds.filter { id in serverApps.contains { $0.id == id } }
        serverApps = AppSummary.visible(AppListEdit.applyingOrder(ids, to: serverApps), current: nil)
        Task {
            do { try await server.reorderApps(ids: ids) } catch { print("[Metamon] Mac: reorderApps failed: \(error)") }
        }
    }

    /// 名前をサーバーにも送り、一覧を取り直す(iOS の `renameApp`)
    private func renameOnServer(_ appId: String, _ name: String) {
        guard let server else { return }
        Task { [weak self] in
            do { _ = try await server.renameApp(appId: appId, name: name) } catch { print("[Metamon] Mac: renameApp failed: \(error)") }
            await self?.refreshServerApps()
        }
    }

    /// サーバーの版を当てる(端末の DSL・一覧も揃える)。封筒が無ければ同じ版の DSL から作れないので中央だけ差し替える
    private func applyFromServer(_ document: AppDocument) {
        let envelope = serverEnvelope(document.appId).flatMap { data in
            (try? AppDocument.decode(data)).map(\.version) == document.version ? data : nil
        }
        set(document.appId, Version(document: document, envelope: envelope))
    }

    private func markOnServer(_ appId: String, _ on: Bool) {
        guard serverIds.contains(appId) != on else { return }
        if on { serverIds.insert(appId) } else { serverIds.remove(appId) }
        defaults?.set(serverIds.sorted(), forKey: Self.serverIdsKey)
    }

    private func current(_ appId: String) -> Version? {
        generated[appId].map { Version(document: $0, envelope: envelopes[appId]) }
    }

    /// 今の版を差し替える。作ったアプリは端末にも残す。封筒の無い版は DSL も一覧も前のまま(古い DSL に新しい名前を付けない)
    private func set(_ appId: String, _ version: Version?) {
        generated[appId] = version?.document
        envelopes[appId] = version?.envelope
        guard isCreated(appId) else {
            // 同梱の例(#1283): 直した版は端末に残し、元の例まで戻したら消す
            if version == nil { store?.removeExample(appId: appId) }
            else if let envelope = version?.envelope { store?.storeExample(appId: appId, envelope: envelope) }
            return
        }
        guard let envelope = version?.envelope else { return }
        store?.store(appId: appId, envelope: envelope)
        savedRows[appId] = LocalAppLibrary.row(after: version?.document, envelope: envelope, previous: savedRows[appId])
        saveList() // 名前・版が変わる
    }

    private func saveList() {
        guard let store else { return }
        store.save(LocalAppLibrary.summaries(createdIds.compactMap { id in
            savedRows[id].map { (id, $0.name, $0.version) }
        }))
    }
}
