import SwiftUI
import Observation
import MetamonDSL
import MetamonRenderer
import MetamonServices

/// アプリ全体の単一ランタイム。DSL ドキュメント・レコードキャッシュ・ナビゲーション・アクション実行を担う。
/// Renderer には `DSLRuntime` protocol 越しに見せる(`.environment(\.dslRuntime, runtime)`)。
@MainActor
@Observable
final class AppRuntime: DSLRuntime {
    private(set) var document: AppDocument
    var dsl: AppDSL { document.dsl }

    /// entity id → レコード(サーバーが正。ここはキャッシュ)
    private(set) var records: [String: [Record]] = [:]
    private(set) var loading: Set<String> = []

    var paths: [String: [Route]] = [:]
    /// タブ表示時に選択中のタブ(= tab.screen)。
    var selectedTab: String = ""
    var sheet: Route?
    var fullScreen: Route?
    var showVoiceSheet = false

    struct Confirm: Identifiable {
        let id = UUID()
        var message: String
        var perform: @MainActor () async -> Void
    }
    var confirm: Confirm?
    /// エラー等の短い通知(下部カプセル)。生成の summary はこちらではなく `summaryBanner`。
    var toast: String? {
        didSet {
            #if DEBUG
            if let toast { print("[Metamon] toast: \(toast)") } // ヘッドレス E2E でエラー文言を拾うため
            #endif
        }
    }

    /// 生成中の状態。nil なら生成していない。`create` は初期状態からの生成(全画面演出)、`edit` は 2 回目以降(薄いインジケータ)。
    struct MorphJob: Equatable {
        enum Kind { case create, edit }
        var kind: Kind
        var transcript: String
        var startedAt: Date
    }
    private(set) var morphJob: MorphJob?
    var isMorphing: Bool { morphJob != nil }

    /// 変身直後に画面上部へ数秒出す要約(読み上げ文)。位置ヒントがあれば該当領域をハイライトする。
    struct SummaryBanner: Identifiable, Equatable {
        let id = UUID()
        var text: String
        var highlight: UnitPoint?
    }
    var summaryBanner: SummaryBanner?

    /// サーバー(appconfig)に届かず、ローカルキャッシュの DSL を表示している。
    private(set) var isOffline = false

    let services: Services
    /// レコードのローカル DB + 未送信キュー(Issue #48)。`records` はここから読み、書きはここに即反映してから裏で送る。
    let recordSync: RecordSyncEngine
    @ObservationIgnored private var syncTask: Task<Void, Never>?
    @ObservationIgnored private var morphTask: Task<MorphResult, Error>?
    @ObservationIgnored private var isForeground = true
    /// 「元に戻して」のフォールバック(appconfig が versions を持たない mock / 未デプロイ時)。直近 10 件。
    @ObservationIgnored private var localHistory: [AppDocument] = []

    init(document: AppDocument, services: Services) {
        self.document = document
        self.services = services
        self.recordSync = RecordSyncFactory.make(client: services.appConfig)
        self.selectedTab = document.dsl.initialTab
        recordSync.onEvent = { [weak self] event in self?.handle(event) }
    }

    // MARK: - Lifecycle

    /// 起動時: live ならサーバーの最新 DSL を取得しホットリロード開始。文字起こしモデルも温めておく。
    /// サーバーに届かなければ最後に取得した DSL(`DocumentCache`)を出し、トーストで「オフライン」を知らせる。
    func bootstrap() async {
        Task { try? await services.transcriber.prepare() }
        await services.notifications.sync(dsl.app.notifications)
        recordSync.start()
        if services.isLive {
            await restoreFromServer()
            startSync()
        }
        #if DEBUG
        // ヘッドレス E2E 用: `METAMON_AUTO_TRANSCRIPT="家計簿アプリが欲しい"` で起動直後に 1 回だけ生成を投げる
        if let text = ProcessInfo.processInfo.environment["METAMON_AUTO_TRANSCRIPT"], !text.isEmpty {
            await morph(transcript: text)
        }
        // ヘッドレス E2E 用: `METAMON_AUTO_RECORD='{"entity":"expense","data":{"amount":980,...}}'` で起動直後に 1 件保存する
        if let raw = ProcessInfo.processInfo.environment["METAMON_AUTO_RECORD"], let data = raw.data(using: .utf8),
           let spec = try? JSONDecoder().decode(AutoRecord.self, from: data) {
            records[spec.entity] = await recordSync.save(appId: document.appId, entity: spec.entity, recordId: nil, data: spec.data)
            print("[Metamon] auto record saved locally: entity=\(spec.entity) count=\(records[spec.entity]?.count ?? 0) sync=\(recordSync.status.label)")
        }
        #endif
    }

    #if DEBUG
    private struct AutoRecord: Decodable {
        var entity: String
        var data: [String: JSONValue]
    }
    #endif

    /// `GET /dsl` で復元。サーバーの version が今より進んでいれば差し替え、遅れていれば(ai の store だけ進んだ等)今の表示を保つ。
    private func restoreFromServer() async {
        do {
            let remote = try await services.appConfig.fetchDocument(appId: document.appId)
            isOffline = false
            if remote.version > document.version || (remote.version == document.version && document.version == 0) {
                apply(remote, animated: false)
            }
            print("[Metamon] restore: server version=\(remote.version) shown=\(document.version) screens=\(dsl.screens.map(\.id))")
        } catch {
            print("[Metamon] restore failed: \(error)")
            if let cached = DocumentCache.shared.load(appId: document.appId), cached.version >= document.version {
                apply(cached, animated: false)
                isOffline = true
                toast = "オフライン: 最後に取得したアプリ(v\(cached.version))を表示しています"
            } else {
                isOffline = true
                toast = "サーバーに接続できません: \(error.localizedDescription)"
            }
        }
    }

    func startSync() {
        syncTask?.cancel()
        guard services.isLive, isForeground else { return }
        let appId = document.appId
        let version = document.version
        syncTask = Task { [weak self] in
            guard let self else { return }
            for await doc in services.sync.updates(appId: appId, after: version) {
                guard doc.version > document.version, morphJob == nil else { continue }
                print("[Metamon] hot reload: version \(document.version) → \(doc.version)")
                apply(doc)
                summaryBanner = SummaryBanner(text: "アプリが更新されました(version \(doc.version))", highlight: nil)
            }
        }
    }

    /// 前面の時だけポーリングする(G13 Day1 版)。背面では止め、戻ったら今の version から張り直す。
    func setForeground(_ active: Bool) {
        guard isForeground != active else { return }
        isForeground = active
        recordSync.setForeground(active)
        if active { startSync() } else { syncTask?.cancel() }
    }

    /// 新しい DSL に差し替える。ナビ・キャッシュは初期化(画面構成が変わるため)。通知は DSL に同期。
    func apply(_ doc: AppDocument, animated: Bool = true) {
        let swap = {
            self.document = doc
            self.records = [:]
            self.paths = [:]
            self.sheet = nil
            self.fullScreen = nil
            self.showVoiceSheet = false
            self.selectedTab = doc.dsl.initialTab
            self.morphJob = nil
        }
        if animated {
            withAnimation(.spring(duration: 0.65, bounce: 0.12), swap)
        } else {
            swap()
        }
        Task { await services.notifications.sync(doc.dsl.app.notifications) }
    }

    // MARK: - Morph / Undo / New app

    /// 発話 → ai サービス → 最新 DSL 適用。
    /// 送る前に iOS 側で意図を判定する: 「元に戻して」系は appconfig の restore(ai を呼ばない)、
    /// 「別のアプリ」系は appId を採番し直してから ai へ(前のアプリはサーバーに残る)。
    /// 生成中は `morphJob` が立ち、RootView が create(全画面「変身中」)/ edit(薄いインジケータ)を出す。
    /// 完了時は新ツリーの挿入と生成中オーバーレイの消去を**同じアニメーション**で行い、クロスフェードにする。
    func morph(transcript: String) async {
        let text = transcript.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !text.isEmpty, morphJob == nil else { return }
        switch VoiceIntent.detect(text) {
        case .undo:
            await undo()
            return
        case .newApp:
            startNewApp()
        case .morph:
            break
        }
        withAnimation(.easeInOut(duration: 0.35)) {
            morphJob = MorphJob(kind: dsl.isVoiceOnly ? .create : .edit, transcript: text, startedAt: .now)
        }
        let previous = document
        let task = Task { [services, appId = document.appId, version = document.version] in
            try await services.ai.morph(transcript: text, appId: appId, currentVersion: version)
        }
        morphTask = task
        defer { morphTask = nil }
        do {
            let result = try await task.value
            remember(previous)
            apply(result.document) // withAnimation の中で morphJob = nil も行う
            startSync()
            if let summary = result.summary?.trimmingCharacters(in: .whitespacesAndNewlines), !summary.isEmpty {
                summaryBanner = SummaryBanner(text: summary, highlight: PositionHint.anchor(in: summary))
            }
            print("[Metamon] morph ok: version=\(result.document.version) mode=\(result.mode ?? "-") screens=\(result.document.dsl.screens.map(\.id)) summary=\(result.summary ?? "")")
        } catch is CancellationError {
            withAnimation(.easeInOut(duration: 0.3)) { morphJob = nil }
            toast = "生成をキャンセルしました"
        } catch {
            print("[Metamon] morph failed: \(error)")
            withAnimation(.easeInOut(duration: 0.3)) { morphJob = nil }
            toast = "生成に失敗しました: \(error.localizedDescription)"
        }
    }

    /// 生成の中断(30 秒超の長引きで UI から呼ぶ)。URLSession は Task キャンセルで切れる。
    func cancelMorph() {
        morphTask?.cancel()
    }

    /// 「元に戻して」(G11)。appconfig の `GET /versions` で直前の version を取り `POST /versions/{n}/restore`。
    /// サーバーが履歴を返せない時(mock / 未デプロイ)はローカル履歴にフォールバックする。
    func undo() async {
        guard morphJob == nil else { return }
        withAnimation(.easeInOut(duration: 0.35)) {
            morphJob = MorphJob(kind: .edit, transcript: "元に戻して", startedAt: .now)
        }
        if services.isLive {
            do {
                let versions = try await services.appConfig.listVersions(appId: document.appId)
                guard let current = versions.first else { throw APIError.server(status: 404, code: "not_found", message: "no versions") }
                guard let target = versions.first(where: { $0.version < current.version }) else {
                    withAnimation(.easeInOut(duration: 0.3)) { morphJob = nil }
                    toast = "これ以上前の状態はありません"
                    return
                }
                let doc = try await services.appConfig.restore(appId: document.appId, toVersion: target.version)
                apply(doc)
                startSync()
                summaryBanner = SummaryBanner(text: "ひとつ前の状態(version \(target.version))に戻しました", highlight: nil)
                print("[Metamon] undo ok: restored from version \(target.version) → new version \(doc.version)")
                return
            } catch {
                print("[Metamon] undo via server failed, falling back to local history: \(error)")
            }
        }
        guard let previous = localHistory.popLast() else {
            withAnimation(.easeInOut(duration: 0.3)) { morphJob = nil }
            toast = "これ以上前の状態はありません"
            return
        }
        apply(previous)
        summaryBanner = SummaryBanner(text: "ひとつ前の状態(version \(previous.version))に戻しました", highlight: nil)
    }

    private func remember(_ doc: AppDocument) {
        localHistory.append(doc)
        if localHistory.count > 10 { localHistory.removeFirst() }
    }

    /// 「別のアプリ。」: appId を新規採番して初期状態にする(サーバー上の前のアプリは残る)。続けて ai に送る前提。
    private func startNewApp() {
        syncTask?.cancel()
        localHistory.removeAll()
        var fresh = (try? DSLExamples.document(DSLExamples.initial, version: 0)) ?? .fallback
        fresh.appId = DeviceAppID.rotate()
        apply(fresh, animated: false)
        print("[Metamon] new app: appId=\(fresh.appId)")
    }

    /// アプリを初期状態(音声画面のみ)に戻す。appId を作り直すので ai/appconfig 側からも「新規端末」に見える(旧データは削除しない)。
    func resetToInitial() {
        syncTask?.cancel()
        localHistory.removeAll()
        DocumentCache.shared.remove(appId: document.appId)
        var fresh = (try? DSLExamples.document(DSLExamples.initial, version: 0)) ?? .fallback
        fresh.appId = DeviceAppID.rotate()
        apply(fresh)
        summaryBanner = nil
        toast = "初期状態に戻しました"
    }

    // MARK: - Records

    func records(for entity: String) -> [Record] { records[entity] ?? [] }

    func load(entities: Set<String>, force: Bool = false) async {
        for e in entities where force || records[e] == nil {
            await refresh(e)
        }
    }

    /// ローカル先出し → サーバー全件で置換 → UI 更新。サーバーに届かなければローカル表示のまま(通信断は toast しない)。
    func refresh(_ entity: String) async {
        let appId = document.appId
        if records[entity] == nil {
            let cached = await recordSync.cached(appId: appId, entity: entity)
            if !cached.isEmpty, document.appId == appId { records[entity] = cached }
        }
        loading.insert(entity)
        defer { loading.remove(entity) }
        do {
            let fresh = try await recordSync.pull(appId: appId, entity: entity)
            guard document.appId == appId else { return }
            records[entity] = fresh
            print("[Metamon] records loaded: entity=\(entity) count=\(fresh.count)")
        } catch {
            if records[entity] == nil { records[entity] = [] }
            print("[Metamon] records pull failed (showing local): entity=\(entity) \(error)")
            if error is APIError { toast = message(for: error, entity: entity) }
        }
    }

    /// 同期キューからの通知。送信完了はローカル DB を読み直し、競合はサーバーから引き直す(サーバー優先)。
    private func handle(_ event: SyncEvent) {
        Task { @MainActor in
            switch event {
            case .sent(let appId, let entity):
                guard appId == document.appId else { return }
                records[entity] = await recordSync.cached(appId: appId, entity: entity)
            case .conflict(let appId, let entity, _):
                guard appId == document.appId else { return }
                toast = "他の端末で変更されていたため、サーバーの内容に合わせました"
                await refresh(entity)
            case .rejected(let appId, let entity, let message):
                guard appId == document.appId else { return }
                toast = "保存できませんでした: \(message)"
                records[entity] = await recordSync.cached(appId: appId, entity: entity)
            case .paused(let pending):
                toast = "送信に失敗しました(未送信 \(pending) 件)。接続を確認してください"
            case .drained:
                break
            }
        }
    }

    // MARK: - Navigation

    func pathBinding(_ stack: String) -> Binding<[Route]> {
        Binding(get: { self.paths[stack] ?? [] }, set: { self.paths[stack] = $0 })
    }

    private func navigate(to screen: String, presentation: Presentation?, recordId: String?, from route: Route) {
        switch presentation ?? .push {
        case .push:
            paths[route.stack, default: []].append(Route(screen: screen, recordId: recordId, stack: route.stack))
        case .sheet:
            sheet = Route(screen: screen, recordId: recordId, stack: "sheet")
        case .fullScreen:
            fullScreen = Route(screen: screen, recordId: recordId, stack: "full")
        }
    }

    /// back: スタックに積まれていれば pop、モーダルの最下層なら dismiss。
    func close(_ route: Route) {
        if !(paths[route.stack] ?? []).isEmpty {
            paths[route.stack]?.removeLast()
            return
        }
        switch route.stack {
        case "sheet": sheet = nil
        case "full": fullScreen = nil
        default: break // main / tab の根は閉じない
        }
    }

    // MARK: - Actions

    func run(_ action: Action, _ ctx: ActionContext) async {
        if let message = action.confirmMessage {
            confirm = Confirm(message: message) { [weak self] in await self?.perform(action, ctx) }
            return
        }
        await perform(action, ctx)
    }

    private func perform(_ action: Action, _ ctx: ActionContext) async {
        switch action.type {
        case .navigate:
            guard let screen = action.screen else { return }
            navigate(to: screen, presentation: action.presentation, recordId: ctx.recordId, from: ctx.route)

        case .back:
            close(ctx.route)

        case .saveRecord:
            guard let form = ctx.form, let entity = action.entity ?? ctx.form?.entity.id else {
                toast = "保存するフォームがありません"
                return
            }
            let missing = form.missingRequired
            guard missing.isEmpty else {
                toast = "\(missing.joined(separator: "・")) を入力してください"
                return
            }
            // 楽観的 UI: ローカル DB に即反映して画面を進める。appconfig への送信は recordSync のキューが行う
            records[entity] = await recordSync.save(appId: document.appId, entity: entity, recordId: form.recordId, data: form.payload)
            if let screen = action.screen {
                navigate(to: screen, presentation: action.presentation, recordId: nil, from: ctx.route)
            } else {
                close(ctx.route)
            }

        case .deleteRecord:
            guard let entity = action.entity ?? ctx.form?.entity.id,
                  let id = ctx.recordId ?? ctx.form?.recordId else { return }
            records[entity] = await recordSync.delete(appId: document.appId, entity: entity, recordId: id)
            if ctx.recordId == nil { close(ctx.route) } // フォーム上からの削除は画面を閉じる

        case .openVoice:
            showVoiceSheet = true

        case .openUrl:
            guard let s = action.url, let url = URL(string: s) else { return }
            _ = await UIApplication.shared.open(url)

        case .resetForm:
            ctx.form?.reset()

        case .scheduleNotification:
            guard let spec = action.notification else { return }
            await attempt {
                try await services.notifications.schedule(spec)
                toast = "通知を予約しました(\(spec.time ?? ""))"
            }

        case .cancelNotification:
            guard let id = action.notificationId else { return }
            services.notifications.cancel(id: id)

        case .unknown:
            toast = "未対応のアクション: \(action.rawType)"
        }
    }

    private func attempt(entity: String? = nil, _ body: () async throws -> Void) async {
        do { try await body() } catch { toast = message(for: error, entity: entity) }
    }

    /// appconfig のエラー封筒を人向けに。422 `invalid_record` は `details.field` を DSL の Field 名に引き直して「どの項目か」を出す。
    func message(for error: Error, entity: String?) -> String {
        guard let api = error as? APIError else { return error.localizedDescription }
        if api.code == "invalid_record" || (api.status == 400 && api.field != nil), let fieldId = api.field {
            let name = entity.flatMap { dsl.entity($0)?.field(fieldId)?.name } ?? fieldId
            return "「\(name)」の入力を確認してください(\(api.errorDescription ?? ""))"
        }
        if api.status == 401 { return "サインインの有効期限が切れました。設定からログインし直してください" }
        return api.errorDescription ?? "\(api)"
    }
}
