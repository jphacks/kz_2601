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
    /// 記録の読み書き(`Shared/Runtime/RecordBook.swift`・iOS の単体テストで確かめる #1338)
    let book: RecordBook
    var records: [String: [Record]] { book.records }
    var loading: Set<String> { book.loading }
    var failed: Set<String> { book.failed }

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
    /// `mockRecordSync` の一時フォルダ。ランタイムを捨てる時(アプリの切り替え)に消す
    @ObservationIgnored private let previewDirectory: URL?
    /// ローカル通知(#965・MacRuntimeNotifications)
    @ObservationIgnored let notifications = MacRuntimeNotifications()

    init(appId: String, dsl: AppDSL, recordSync: RecordSyncEngine? = nil, ai: any AIClient = MacComposeSession.gatedAI(),
         isAIConsentGranted: @escaping @MainActor () -> Bool = { AIConsentStore.shared.isGranted }) {
        self.appId = appId
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
        book = RecordBook(appId: appId, dsl: dsl, recordSync: self.recordSync)
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

    func records(for entity: String) -> [Record] { book.records(for: entity) }

    func load(entities: Set<String>, force: Bool) async {
        await book.load(entities: entities, force: force)
    }

    func refresh(_ entity: String) async { await book.refresh(entity) }

    func updateRecord(entity: String, recordId: String, data: [String: JSONValue]) async -> Bool {
        switch await book.update(entity: entity, recordId: recordId, data: data, canEdit: { self.canEdit(entity: $0, record: $1) }) {
        case .saved: return true
        case .missing: return false
        case .readOnly:
            notifyReadOnly(entity: entity)
            return false
        }
    }

    func createRecord(entity: String, data: [String: JSONValue]) async -> Bool {
        await book.create(entity: entity, data: data)
    }

    func notifyReadOnly(entity: String) { toast = RecordActionSteps.readOnlyMessage }
    func notifySaveFailed(entity: String) { toast = RecordActionSteps.saveFailedMessage }

    private func record(_ entity: String, _ id: String) -> Record? { book.record(entity, id) }

    private func reloadLocal(_ entity: String) async { await book.reloadLocal(entity) }

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
        switch await book.save(entity: entity, form: form, recordId: recordId, canEdit: { self.canEdit(entity: $0, record: $1) }) {
        case .readOnly: notifyReadOnly(entity: entity)
        case .saved, .unchanged: leaveAfterSave(action, ctx) // 何も変えずに保存したら書かずに画面だけ進める(M10)
        case .inFlight: break // 保存ボタンの連打
        }
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
        let ticket = await book.deleteUndoably(entity: entity, recordId: recordId, grace: .seconds(Self.undoSeconds))
        withAnimation { recordUndo = ticket }
        AccessibilityNotification.Announcement(L10n.tr("削除しました")).post() // iOS と同じく VoiceOver に知らせる
    }

    func undoRecordDelete() async {
        guard let ticket = recordUndo else { return }
        withAnimation { recordUndo = nil }
        await book.undoDelete(ticket)
    }
}
