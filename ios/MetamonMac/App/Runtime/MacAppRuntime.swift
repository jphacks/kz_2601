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
/// 記録の置き場は `MacRecordVault`(#1284): サーバーにある作ったアプリはサーバーの appconfig(`MacServices.appConfig`)、
/// それ以外の作ったアプリは端末だけの appconfig。同梱の例は iOS の `METAMON_MOCK=1` と同じく
/// メモリ上の appconfig(`InMemoryAppConfigClient`)に持つ。
/// ローカル通知は `MacRuntimeNotifications`(#965)。AI の部品(aiButton・aiGenerate・aiChat)は `MacAppRuntime+AIText.swift`(#962)。
/// 話して入力(aiChat のマイク・transcribeField)は `MacAppRuntime+Dictation.swift`(#1328)。
/// 会話・印・通報・参加者・仲間探しは `MacAppRuntime+Social.swift`(#963)。課金・フォローなど iOS 固有の機能は持たない(未対応のアクションは一言で知らせる)。export は保存・共有・プリント(`+Export`)。
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
    /// もう読み上げた toast(`AppCanvas` は読み直さない)
    @ObservationIgnored var announcedToast: String?
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
    /// 作ったアプリの記録のエンジンを借りている(`MacRecordEngines`)。同じアプリを開いた別のウインドウと同じエンジンを使う
    @ObservationIgnored private let recordLease: MacRecordEngineLease?
    /// AI の部品のオンライン版の送り先(#962)
    @ObservationIgnored let ai: any AIClient
    @ObservationIgnored let isAIConsentGranted: @MainActor () -> Bool
    #if !APP_STORE_REVIEW
    /// feed の問い合わせ先(`MacAppRuntime+Feeds.swift`・#1288)。サーバーにつながっていない時は nil
    @ObservationIgnored var feedClient: (any FeedClient)?
    #endif
    #if !APP_STORE_REVIEW
    /// 人とつながる部品(#963・`MacAppRuntime+Social.swift`)の問い合わせ先と状態。モックではクライアントが無く、ボタンを出さない
    @ObservationIgnored var social: MacSocial = .shared
    #endif
    /// lookupBook の問い合わせ先(#1276)。サーバーにつながっている時は `MacServices.books`、モックの時は nil
    @ObservationIgnored var books: (any BookLookupClient)?
    @ObservationIgnored var calendarWriter: any CalendarWriting = EventKitCalendarWriter()
    /// `mockRecordSync` の一時フォルダ。ランタイムを捨てる時(アプリの切り替え)に消す
    @ObservationIgnored private let previewDirectory: URL?
    /// ローカル通知(#965・MacRuntimeNotifications)
    @ObservationIgnored let notifications = MacRuntimeNotifications()
    #if !APP_STORE_REVIEW
    /// roleField の無い aiChat の対(仮 id → 相方)。片方が断られたら相方も消す(`MacAppRuntime+AIText.swift`・#1326)
    @ObservationIgnored let aiChatPairs = AIChatPairLedger()
    #endif
    /// 話して入力(aiChat のマイク・transcribeField・#1328)。声で頼む(`MacVoiceInput`)と同じ認識を使う。nil なら使えない
    @ObservationIgnored let dictationSource: TranscriberDictation?
    /// 録音と再生は同時に 1 つだけ(#952)。audioRecorder の録音中は話して入力・録音ファイルの文字起こしを始めない
    @ObservationIgnored var audioFocus: AudioFocus = .shared
    /// openVoice(#946): 中央のアプリを直す会話を声で開く(⇧⌘M と同じ)。`AppCanvas` が `MacRootView` の会話につなぐ
    @ObservationIgnored var onOpenVoice: (@MainActor () -> Void)?
    /// モーダルの中の openVoice。モーダルを閉じ終えてから会話を開く(シートを重ねない)
    @ObservationIgnored var pendingOpenVoice = false
    /// Field.health を Mac では書かないことの一言(#958・アプリごとに 1 回)
    @ObservationIgnored var healthNote = MacHealthNote()

    /// `recordLease` を渡したらそのエンジンを使い(止めるのは `MacRecordEngines`)、`recordSync` を渡したらこのランタイムだけのエンジン
    init(appId: String, dsl: AppDSL, recordSync: RecordSyncEngine? = nil, recordLease: MacRecordEngineLease? = nil,
         ai: any AIClient = MacComposeSession.gatedAI(),
         isAIConsentGranted: @escaping @MainActor () -> Bool = { AIConsentStore.shared.isGranted },
         dictation: TranscriberDictation? = nil) {
        self.appId = appId
        self.dictationSource = dictation
        self.dsl = dsl
        self.ai = ai
        self.isAIConsentGranted = isAIConsentGranted
        selectedTab = dsl.initialTab // iOS と同じ: initialScreen がタブならそのタブから
        self.recordLease = recordLease
        if let recordLease {
            self.recordSync = recordLease.engine
            previewDirectory = nil
        } else if let recordSync {
            self.recordSync = recordSync
            previewDirectory = nil
        } else {
            let dir = Self.newPreviewDirectory()
            self.recordSync = Self.mockRecordSync(directory: dir)
            previewDirectory = dir
        }
        book = RecordBook(appId: appId, dsl: dsl, recordSync: self.recordSync)
        book.postAuthor = { [weak self] entity in self?.postAuthor(for: entity) } // 共有の投稿に作者を付ける(#963)
        if let recordLease {
            recordLease.onEvent = { [weak self] event in self?.handle(event) } // 同じエンジンの通知は借りている全員に届く
        } else {
            self.recordSync.onEvent = { [weak self] event in self?.handle(event) }
        }
        self.recordSync.start()
        notifications.start(self)
    }

    /// アプリを切り替えるとランタイムごと作り直す(`AppCanvas` の `.id`)。購読のタスクがエンジンを握り続けないよう止め、
    /// 一時フォルダも消す(切り替えのたびに増えないように)
    deinit {
        let engine = recordSync
        let lease = recordLease
        let dir = previewDirectory
        Task { @MainActor in
            // 借りたエンジンは返すだけ(最後のウインドウが返したら `MacRecordEngines` が止める)
            if let lease { lease.release() } else { engine.stop() }
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

    func records(for entity: String) -> [Record] {
        #if APP_STORE_REVIEW
        book.records(for: entity)
        #else
        hidingBlocked(book.records(for: entity), entity: entity) // ブロックした人の投稿は隠す(#963)
        #endif
    }

    /// 共有レコードの作者判定(#249)と「自分の投稿」(#963)。サーバーにつながっていない時は nil
    var currentUserId: String? {
        #if APP_STORE_REVIEW
        nil
        #else
        social.currentUserId()
        #endif
    }

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
        await createRecordId(entity: entity, data: data) != nil
    }

    /// `createRecord` と同じ。作った行の id(仮 id `local-…`)を返す。保存できなければ nil(iOS と同じ)。
    func createRecordId(entity: String, data: [String: JSONValue]) async -> String? {
        await book.createId(entity: entity, data: data)
    }

    func notifyReadOnly(entity: String) { toast = RecordActionSteps.readOnlyMessage }
    func notifySaveFailed(entity: String) { toast = RecordActionSteps.saveFailedMessage }

    private func record(_ entity: String, _ id: String) -> Record? { book.record(entity, id) }

    func reloadLocal(_ entity: String) async { await book.reloadLocal(entity) }

    /// 同期キューからの通知。送れたらローカルを読み直し、競合はサーバーから引き直す(iOS と同じくサーバー優先)。
    private func handle(_ event: SyncEvent) {
        Task { @MainActor in
            #if !APP_STORE_REVIEW
            await revertAIChatPartner(for: event)
            #endif
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
                if let refused = event.refusedWrite, refused.appId == appId { await refresh(refused.entity) }
            }
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

        case .export:
            await export(action, ctx) // CSV / PDF を保存・共有・プリントで出す(`MacAppRuntime+Export.swift`・#1143・#946・#953)

        case .startActivity:
            // ライブアクティビティ(#423)は iPhone のロック画面のもの。Mac には無いので一言だけ
            toast = L10n.tr("ライブアクティビティは iPhone だけで表示されます")

        case .endActivity:
            break // 出していないので止めるものも無い

        case .openVoice:
            openVoice()

        case .follow, .unfollow:
            // フォロー(#270)は投稿のある iOS のアプリの機能。Mac には投稿の画面もサインインの相手も無い
            toast = L10n.tr("フォローは Mac では使えません")

        default:
            toast = L10n.tr("このボタンは Mac ではまだ使えません") // 知らない種類(.unknown)
        }
    }

    /// openVoice: 声で頼む会話を開く。モーダルの中からは、入力を確かめてモーダルを閉じ、閉じ終えてから開く
    func openVoice() {
        guard let onOpenVoice else {
            toast = L10n.tr("このボタンは Mac ではまだ使えません")
            return
        }
        guard sheet != nil || fullScreen != nil else { return onOpenVoice() }
        confirmDiscard(leaving: { $0.stack == "sheet" || $0.stack == "full" }) { [weak self] in
            guard let self else { return }
            pendingOpenVoice = true
            while sheet != nil || fullScreen != nil { dismissTopModal() }
        }
    }

    /// モーダルが閉じ終わった(`AppCanvas`)。openVoice を待っていれば会話を開く
    func openVoiceAfterModal() {
        guard pendingOpenVoice, sheet == nil, fullScreen == nil else { return }
        pendingOpenVoice = false
        onOpenVoice?()
    }

    private func save(_ action: Action, _ ctx: ActionContext) async {
        let target = RecordActionSteps.saveTarget(action, ctx, dsl: dsl) { self.records(for: $0) }
        guard case .ready(let entity, let form, let recordId) = target else {
            if case .invalid(let message) = target { toast = message }
            return
        }
        let skipsHealth = MacHealthNote.applies(entity: dsl.entity(entity), data: form.payload, isNew: recordId == nil)
        switch await book.save(entity: entity, form: form, recordId: recordId, canEdit: { self.canEdit(entity: $0, record: $1) }) {
        case .readOnly: notifyReadOnly(entity: entity)
        case .saved:
            // 保存の後は画面が閉じる・進むので、VoiceOver には先に結果を伝える(#937)
            MacVoiceOver.announce(.saved)
            // HealthKit が無いので書かない(#958)。一言はアプリごとに 1 回
            if skipsHealth, let note = healthNote.noteOnce(appId: appId) {
                MacVoiceOver.announce(.healthSkipped) // 「保存しました」に続けて読む。toast では読み直さない
                announcedToast = note
                toast = note
            }
            leaveAfterSave(action, ctx)
        case .unchanged: leaveAfterSave(action, ctx) // 何も変えずに保存したら書かずに画面だけ進める(M10)
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
    func reveal(screen: String, recordId: String? = nil) {
        guard let next = NavigationResolver.reveal(screen, recordId: recordId, graph: ScreenGraph(dsl: dsl), dsl: dsl,
                                                   from: navigationState) else { return }
        setNavigation(next)
    }

    // MARK: - 削除の取り消し(M11)

    func deleteUndoably(entity: String, recordId: String) async {
        let ticket = await book.deleteUndoably(entity: entity, recordId: recordId, grace: .seconds(Self.undoSeconds))
        withMacAnimation { recordUndo = ticket } // 「削除しました」の読み上げは AppCanvas の取り消しの一言が出す(二重に読まない・#937)
    }

    func undoRecordDelete() async {
        guard let ticket = recordUndo else { return }
        withMacAnimation { recordUndo = nil }
        await book.undoDelete(ticket)
    }
}
