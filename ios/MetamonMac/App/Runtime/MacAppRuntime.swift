import SwiftUI
import AppKit
import Observation
import MetamonDSL
import MetamonRenderer
import MetamonServices

/// Mac の中央(生成したアプリ)のランタイム(#939)。iOS の `AppRuntime` から UIKit に依存しない部分を共有して動かす:
/// - 画面の移動: `RuntimeNavigating`(iOS と同じ実装)
/// - 確認ダイアログ: `RuntimeConfirm`(iOS と同じ文言・主ボタン)
/// - 保存・削除の前段: `RecordActionSteps`、書き込みと同期: `RecordSyncEngine`(MetamonServices)
///
/// サーバーへの接続(appconfig のクライアント・ログイン)は Mac に Firebase がまだ無い(#930)ので、
/// iOS の `METAMON_MOCK=1` と同じくメモリ上の appconfig(`InMemoryAppConfigClient`)に記録を持つ。
/// ローカル通知は `MacRuntimeNotifications`(#965)。AI の部品(aiButton・aiGenerate・aiChat)は `MacAppRuntime+AIText.swift`(#962)。
/// 話して入力(aiChat のマイク・transcribeField)は `MacAppRuntime+Dictation.swift`(#1328)。
/// 共有・課金など iOS 固有の機能は持たない(未対応のアクションは一言で知らせる)。
@MainActor
@Observable
final class MacAppRuntime: RuntimeNavigating {
    let dsl: AppDSL
    let appId: String
    var paths: [String: [Route]] = [:]
    var selectedTab: String
    var sheet: Route?
    var fullScreen: Route?
    private(set) var loading: Set<String> = []
    private(set) var failed: Set<String> = []
    private(set) var records: [String: [Record]] = [:]

    /// 確認ダイアログ(アクションの confirm)。中央の列に `.confirmationDialog` で出す
    var confirm: RuntimeConfirm?
    /// 中央の列の上に出す一言
    var toast: String?
    /// 一覧の記録を消した直後の「削除しました・元に戻す」(iOS の M11 と同じく数秒は送らない)
    var recordUndo: DeletionTicket?
    static let undoSeconds: Double = 5
    /// addToCalendar で出す確認シートの中身(`MacAppRuntime+Calendar.swift`・#978)
    var calendarDraft: CalendarEventDraft?
    /// AI の部品が同意(#246)の無いまま送ろうとした。中央の列に同意の説明を出す(`MacAppRuntime+AIText.swift`)
    var needsAIConsent = false
    /// 保存していない入力を捨てる前の確認(Esc・⌘W・アプリの切り替え・#943)。確認ダイアログと同じ場所に出す
    var discardRequest: DiscardRequest?
    /// 描いているフォームの控え(`MacAppRuntime+Discard.swift`)
    @ObservationIgnored let openForms = OpenFormRegistry()

    @ObservationIgnored let recordSync: RecordSyncEngine
    /// AI の部品のオンライン版の送り先(#962)
    @ObservationIgnored let ai: any AIClient
    @ObservationIgnored let isAIConsentGranted: @MainActor () -> Bool
    /// lookupBook の問い合わせ先。appconfig への接続(#930)が来るまでは nil
    @ObservationIgnored var books: (any BookLookupClient)?
    @ObservationIgnored var calendarWriter: any CalendarWriting = EventKitCalendarWriter()
    @ObservationIgnored private var savingForms: Set<ObjectIdentifier> = []
    /// `mockRecordSync` の一時フォルダ。ランタイムを捨てる時(アプリの切り替え)に消す
    @ObservationIgnored private let previewDirectory: URL?
    /// ローカル通知(#965・MacRuntimeNotifications)
    @ObservationIgnored let notifications = MacRuntimeNotifications()
    /// 話して入力(aiChat のマイク・transcribeField・#1328)。声で頼む(`MacVoiceInput`)と同じ認識を使う。nil なら使えない
    @ObservationIgnored let dictationSource: TranscriberDictation?

    init(appId: String, dsl: AppDSL, recordSync: RecordSyncEngine? = nil, ai: any AIClient = MacComposeSession.gatedAI(),
         isAIConsentGranted: @escaping @MainActor () -> Bool = { AIConsentStore.shared.isGranted },
         dictation: TranscriberDictation? = nil) {
        self.appId = appId
        self.dictationSource = dictation
        self.dsl = dsl
        self.ai = ai
        self.isAIConsentGranted = isAIConsentGranted
        selectedTab = dsl.initialTab // iOS と同じ: initialScreen がタブならそのタブから
        if let recordSync {
            self.recordSync = recordSync
            previewDirectory = nil
        } else {
            let dir = Self.newPreviewDirectory()
            self.recordSync = Self.mockRecordSync(directory: dir)
            previewDirectory = dir
        }
        self.recordSync.onEvent = { [weak self] event in self?.handle(event) }
        self.recordSync.start()
        notifications.start(self)
    }

    /// アプリを切り替えるとランタイムごと作り直す(`AppCanvas` の `.id`)。購読のタスクがエンジンを握り続けないよう止め、
    /// 一時フォルダも消す(切り替えのたびに増えないように)
    deinit {
        let engine = recordSync
        let dir = previewDirectory
        Task { @MainActor in
            engine.stop()
            if let dir { try? FileManager.default.removeItem(at: dir) }
        }
    }

    private static let previewPrefix = "metamon-mac-preview-"

    private static func newPreviewDirectory() -> URL {
        FileManager.default.temporaryDirectory
            .appendingPathComponent("\(previewPrefix)\(UUID().uuidString)", isDirectory: true)
    }

    /// 一時フォルダをまとめて消す。使っている途中のものも消すので、起動時(前回の残り)と終了時だけ呼ぶ
    static func removePreviewDirectories() {
        let fm = FileManager.default
        let items = (try? fm.contentsOfDirectory(at: fm.temporaryDirectory, includingPropertiesForKeys: nil)) ?? []
        for item in items where item.lastPathComponent.hasPrefix(previewPrefix) {
            try? fm.removeItem(at: item)
        }
    }

    /// メモリ上の appconfig + 一時フォルダのローカル DB(アプリを切り替える・閉じると消す)。iOS の METAMON_MOCK 相当。
    static func mockRecordSync(directory dir: URL) -> RecordSyncEngine {
        let store = RecordStore(directory: dir)
        let client = InMemoryAppConfigClient()
        return RecordSyncEngine(store: store, queue: SyncQueue(directory: nil, store: store, client: client),
                                client: client, connectivity: ManualConnectivity(online: true))
    }

    // MARK: - Records

    func records(for entity: String) -> [Record] { records[entity] ?? [] }

    func load(entities: Set<String>, force: Bool) async {
        for e in entities where force || records[e] == nil {
            await refresh(e)
        }
    }

    /// ローカル先出し → appconfig(メモリ)の全件で置換。取れずローカルにも無ければ「読み込めませんでした」。
    func refresh(_ entity: String) async {
        if records[entity] == nil {
            let cached = await recordSync.cached(appId: appId, entity: entity)
            if !cached.isEmpty { records[entity] = cached }
        }
        loading.insert(entity)
        defer { loading.remove(entity) }
        do {
            records[entity] = try await recordSync.pull(appId: appId, entity: entity)
            failed.remove(entity)
        } catch {
            if records[entity]?.isEmpty ?? true {
                records[entity] = []
                failed.insert(entity)
            }
            print("[MetamonMac] records pull failed: entity=\(entity) \(error)")
        }
    }

    func updateRecord(entity: String, recordId: String, data: [String: JSONValue]) async -> Bool {
        guard let current = record(entity, recordId) else { return false }
        guard canEdit(entity: entity, record: current) else {
            notifyReadOnly(entity: entity)
            return false
        }
        records[entity] = await recordSync.save(appId: appId, entity: entity, recordId: recordId, data: data)
        return true
    }

    func createRecord(entity: String, data: [String: JSONValue]) async -> Bool {
        guard dsl.entity(entity) != nil else { return false }
        records[entity] = await recordSync.save(appId: appId, entity: entity, recordId: nil, data: data)
        return true
    }

    func notifyReadOnly(entity: String) { toast = RecordActionSteps.readOnlyMessage }
    func notifySaveFailed(entity: String) { toast = RecordActionSteps.saveFailedMessage }

    private func record(_ entity: String, _ id: String) -> Record? {
        records[entity]?.first { $0.id == id }
    }

    private func reloadLocal(_ entity: String) async {
        records[entity] = await recordSync.cached(appId: appId, entity: entity)
    }

    /// 同期キューからの通知。送れたらローカルを読み直し、競合はサーバーから引き直す(iOS と同じくサーバー優先)。
    private func handle(_ event: SyncEvent) {
        Task { @MainActor in
            switch event {
            case .sent(let id, let entity) where id == appId:
                await reloadLocal(entity)
            case .conflict(let id, let entity, _) where id == appId:
                await refresh(entity)
            case .sent, .conflict, .paused, .drained:
                break
            default:
                toast = RecordActionSteps.saveFailedMessage
                // 断られた書き込みは破棄済み(create はローカル行も消えた)。楽観的に出した分をサーバーの内容に戻す
                if let refused = Self.refusedEntity(event), refused.appId == appId { await refresh(refused.entity) }
            }
        }
    }

    private static func refusedEntity(_ event: SyncEvent) -> (appId: String, entity: String)? {
        switch event {
        case .forbidden(let appId, let entity, _), .rejected(let appId, let entity, _, _, _),
             .notMember(let appId, let entity, _), .denied(let appId, let entity, _, _),
             .proRequired(let appId, let entity, _, _), .planLimit(let appId, let entity, _, _),
             .appStopped(let appId, let entity, _), .textNotAllowed(let appId, let entity, _, _):
            return (appId, entity)
        case .sent, .conflict, .paused, .drained:
            return nil
        }
    }

    // MARK: - Actions

    func run(_ action: Action, _ ctx: ActionContext) async {
        if let message = action.confirmMessage {
            let button = RuntimeConfirm.button(for: action.type)
            confirm = RuntimeConfirm(message: message, primaryLabel: button.label, isDestructive: button.destructive) { [weak self] in
                await self?.perform(action, ctx)
            }
            return
        }
        await perform(action, ctx)
    }

    private func perform(_ action: Action, _ ctx: ActionContext) async {
        switch action.type {
        case .navigate:
            guard let screen = action.screen else { return }
            if let ref = action.prefillRef {
                // 親から子を作る(#256): 今のレコードを子フォームの ref に積み、子は新規で開く
                guard let parentId = ctx.currentRecordId else {
                    toast = RecordActionSteps.unsavedParentMessage(ctx)
                    return
                }
                FormPrefill.set([ref: .string(parentId)])
                navigate(to: screen, presentation: action.presentation, recordId: nil, from: ctx.route)
                return
            }
            navigate(to: screen, presentation: action.presentation, recordId: ctx.recordId, from: ctx.route)

        case .back:
            close(ctx.route)

        case .saveRecord:
            await save(action, ctx)

        case .deleteRecord:
            guard let target = RecordActionSteps.deleteTarget(action, ctx, dsl: dsl) else { return }
            if let current = record(target.entity, target.recordId), !canDelete(entity: target.entity, record: current) {
                notifyReadOnly(entity: target.entity)
                return
            }
            await deleteUndoably(entity: target.entity, recordId: target.recordId)
            if ctx.recordId == nil { close(ctx.route) } // フォーム上からの削除は画面を閉じる

        case .resetForm:
            ctx.form?.reset()

        case .openUrl:
            guard let url = ExternalURLPolicy.openable(action.url) else {
                toast = L10n.tr("openUrl: url は https か line/slack/discord/tel/mailto/maps のみ")
                return
            }
            NSWorkspace.shared.open(url)

        case .addToCalendar:
            addToCalendar(action, ctx)

        case .lookupBook:
            await lookupBook(action, ctx)

        case .scheduleNotification, .cancelNotification:
            await notifications.perform(action)

        default:
            // 共有・書き出しなどは Mac ではまだ動かない(後の Issue)
            toast = L10n.tr("このボタンは Mac ではまだ使えません")
        }
    }

    private func save(_ action: Action, _ ctx: ActionContext) async {
        let target = RecordActionSteps.saveTarget(action, ctx, dsl: dsl) { self.records(for: $0) }
        guard case .ready(let entity, let form, let recordId) = target else {
            if case .invalid(let message) = target { toast = message }
            return
        }
        if let id = recordId, let current = record(entity, id), !canEdit(entity: entity, record: current) {
            notifyReadOnly(entity: entity)
            return
        }
        // 既存の記録を何も変えずに保存: 書き込まずに画面だけ進める(M10)
        guard form.needsWrite else {
            leaveAfterSave(action, ctx)
            return
        }
        // 保存ボタンの連打で同じ内容が 2 件できないように
        guard savingForms.insert(ObjectIdentifier(form)).inserted else { return }
        defer { savingForms.remove(ObjectIdentifier(form)) }
        let written = await recordSync.write(appId: appId, entity: entity, recordId: recordId, data: form.payload)
        records[entity] = written.records
        form.markSaved(queuedOffline: !recordSync.isOnline)
        leaveAfterSave(action, ctx)
    }

    /// 保存の後: `screen` があればそこへ、無ければ閉じる。
    private func leaveAfterSave(_ action: Action, _ ctx: ActionContext) {
        if let screen = action.screen {
            navigate(to: screen, presentation: action.presentation, recordId: nil, from: ctx.route)
        } else {
            close(ctx.route)
        }
    }

    // MARK: - メニューの「戻る」・Esc(#934)

    /// 今いちばん上に見えている画面: フルスクリーン → シート → 選択中タブ(無ければ main)の順。
    var topStack: String {
        if fullScreen != nil { return "full" }
        if sheet != nil { return "sheet" }
        guard dsl.app.hasTabs else { return "main" }
        return Route.tabStack(selectedTab.isEmpty ? dsl.initialTab : selectedTab)
    }

    /// 戻れる画面があるか(シートが開いている・スタックに積まれている)
    var canGoBack: Bool { sheet != nil || fullScreen != nil || !(paths[topStack] ?? []).isEmpty }

    /// back アクションと同じ: 積まれていれば 1 つ戻り、モーダルの最下層ならモーダルを閉じる。
    func goBack() {
        guard canGoBack else { return }
        close(Route(screen: "", stack: topStack))
    }

    /// Esc: 開いているモーダル(フルスクリーン → シートの順)を中のスタックごと閉じる。
    func dismissTopModal() {
        if fullScreen != nil {
            fullScreen = nil
            paths["full"] = nil
        } else if sheet != nil {
            sheet = nil
            paths["sheet"] = nil
        }
    }

    // MARK: - 画面マップ(#970)

    /// 今いちばん上に見えている画面の id(画面マップを開いた時に選んでおく)
    var currentScreen: String { NavigationResolver.top(navigationState, dsl: dsl).screen }

    /// 画面マップから画面を開く。モーダルを閉じ、根(タブ)から矢印をたどって開く(戻るで来た道を戻れる)
    func reveal(screen: String) {
        guard let next = NavigationResolver.reveal(screen, graph: ScreenGraph(dsl: dsl), dsl: dsl, from: navigationState) else { return }
        setNavigation(next)
    }

    // MARK: - 削除の取り消し(M11)

    func deleteUndoably(entity: String, recordId: String) async {
        let ticket = await recordSync.deleteUndoably(appId: appId, entity: entity, recordId: recordId,
                                                     grace: .seconds(Self.undoSeconds)).ticket
        await reloadLocal(entity)
        withAnimation { recordUndo = ticket }
        AccessibilityNotification.Announcement(L10n.tr("削除しました")).post() // iOS と同じく VoiceOver に知らせる
    }

    func undoRecordDelete() async {
        guard let ticket = recordUndo else { return }
        withAnimation { recordUndo = nil }
        guard await recordSync.undoDelete(ticket) != nil else { return }
        await reloadLocal(ticket.entity)
    }
}
