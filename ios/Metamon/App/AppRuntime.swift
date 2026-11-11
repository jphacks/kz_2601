import SwiftUI
import Observation
import MetamonDSL
import MetamonRenderer
import MetamonServices

/// アプリ全体の単一ランタイム。DSL ドキュメント・レコードキャッシュ・ナビゲーション・アクション実行を担う。
/// Renderer には `DSLRuntime` protocol 越しに見せる(`.environment(\.dslRuntime, runtime)`)。
@MainActor
@Observable
final class AppRuntime: RuntimeNavigating {
    private(set) var document: AppDocument {
        didSet {
            if oldValue.appId != document.appId { reloadChatHistory() } // #210 アプリごとのチャット履歴
            recordSync.encryptedFields[document.appId] = document.dsl.encryptedFields // #233 暗号化して保存する項目
        }
    }
    var dsl: AppDSL { document.dsl }

    /// entity id → レコード(サーバーが正。ここはキャッシュ)
    private(set) var records: [String: [Record]] = [:] {
        didSet {
            if dsl.app.notifications.contains(where: { $0.recordEntity != nil }) { scheduleNotificationSync() } // #228 レコードの時刻・場所から通知
            if dsl.app.notifications.contains(where: \.isConditional) { scheduleConditionCheck() } // #265 条件で知らせる
            refreshLiveActivityStats() // #423 ロック画面の数字
        }
    }
    /// このアプリの次に届く通知(設定画面に出す・#228)。
    var upcomingNotifications: [UpcomingNotification] = []
    /// 通知が許可されていない(予約しても届かない)。
    var notificationsDenied = false
    /// このアプリで「着いたら知らせる」場所の数(near)と、位置情報が許可されず届かない状態(設定画面に静かに出す)。
    var notificationPlaces = 0
    var placesLocationOff = false
    @ObservationIgnored var notificationSyncTask: Task<Void, Never>?
    @ObservationIgnored var conditionCheckTask: Task<Void, Never>?
    private(set) var loading: Set<String> = []
    /// サーバーから取れず、ローカルにも無い entity(#85)。Renderer は空状態ではなく「読み込めませんでした(再試行)」を描く。
    private(set) var failed: Set<String> = []

    var paths: [String: [Route]] = [:]
    /// タブ表示時に選択中のタブ(= tab.screen)。
    var selectedTab: String = ""
    var sheet: Route?
    var fullScreen: Route?
    var showVoiceSheet = false
    /// 第三者 AI への送信の同意(#246)。アプリ本体(MetamonApp)が `.shared` を入れる。nil(ユニットテスト等)は同意済み扱い。
    @ObservationIgnored var aiConsent: AIConsentStore?
    /// 同意の画面を出す(初回起動 / 同意前に生成・対話しようとした時)。
    var showAIConsent = false
    /// 同意の前に話した最後の 1 件(M2)。同意して画面が閉じたら続けて送る(`resumeUtteranceAfterConsent`)
    @ObservationIgnored var utteranceAwaitingConsent: (text: String, source: ClarifyFlow.InputSource)?
    /// 音声モデルのダウンロード確認(#246・4.2.3(ii))。端末にモデルが無く未承認の時だけ立つ。
    var showModelDownloadConfirm = false
    /// 初回のチュートリアル(#274)。MetamonApp が `.shared` を入れる。nil(ユニットテスト等)は見た扱い。
    @ObservationIgnored var tutorial: FirstRunTutorialStore?
    /// チュートリアルを出す(初回の同意のあと / 設定の「使い方を見る」)。
    var showTutorial = false
    /// 気づきにくい操作の吹き出し(#1441)。MetamonApp が `.shared` を入れる。nil(ユニットテスト等)は出さない。
    @ObservationIgnored var coachMarks: CoachMarkStore?
    #if !APP_STORE_REVIEW // 審査用ビルドは β の案内を持たない(#313)
    /// β の初回の案内(#313)。MetamonApp が `.shared` を入れる。nil(ユニットテスト等)は出さない。
    @ObservationIgnored var betaWelcome: BetaWelcomeStore?
    /// β の案内を出す(チュートリアルのあと / 設定の「β 版のご案内」)。
    var showBetaWelcome = false
    #endif
    /// 最初のアプリができた直後の「こう言うと直せます」(#274・1 回だけ)。
    var showFirstEditHint = false
    /// 「頼み方のコツ」(#415)で選んだ例。音声画面か対話の入力欄が受け取って空に戻す(送信はしない)。
    var composerDraft: String?
    /// 変身一覧シート(#51: FAB 長押し / 「アプリ一覧」)。
    var showAppSwitcher = false
    /// 自分のアプリ一覧(`appsList()` が更新)。
    var apps: [AppSummary] = []
    /// 削除中のアプリを一覧から先に隠す(#180 楽観的 UI)。`AppRuntime+Delete.swift`
    let appDeletion = OptimisticAppDeletion()
    private(set) var appsLoading = false
    /// 初期(音声)画面に来た経緯。挨拶文言(`Greeting`)の出し分けに使う。
    enum HomeEntry { case launch, newApp, home }
    var homeEntry: HomeEntry = .launch

    /// 確認ダイアログの中身。Mac と共有(`Shared/Runtime/RuntimeConfirm.swift`)
    typealias Confirm = RuntimeConfirm
    var confirm: Confirm?
    /// 手で動かす編集モード(#1442・`AppRuntime+GridEdit.swift`)。nil なら普段の画面
    var gridEdit: GridEditSession?
    /// 「完了」で保存している間(二重に押させない)
    var gridEditSaving = false
    /// 編集を始めた時の版(この版に対して PATCH する)
    @ObservationIgnored var gridEditBaseVersion = 0
    /// 書き出したファイル(#421)。立つと共有シートを開く(`RootView` の HostSheets)。
    var exportFile: ExportFile?
    /// addToCalendar で開く「新規イベント」画面の中身(`AppRuntime+Calendar.swift`)。
    var calendarDraft: CalendarEventDraft?

    /// 共有アプリへの初めての投稿で表示名を聞く(#250)。答えたら `resume` で保存を続ける。
    struct AuthorNamePrompt: Identifiable {
        let id = UUID()
        var resume: @MainActor () async -> Void
    }
    var authorNamePrompt: AuthorNamePrompt?
    /// 軽微な通知(下部カプセル・保存失敗など)。生成の失敗はこちらではなく `errorCard`、生成の summary は `summaryBanner`。
    var toast: String? {
        didSet {
            #if DEBUG
            if let toast { print("[Metamon] toast: \(toast)") } // ヘッドレス E2E でエラー文言を拾うため
            #endif
            if let toast, toast != oldValue { VoiceOverAnnouncer.announce(toast) } // H8
            if fixOffer != nil, fixOffer?.message != toast { fixOffer = nil } // 別のトーストに替わったら外す
        }
    }
    /// いまのトーストに付ける「直してと頼む」(`AppRuntime+FixRequest.swift`)。
    var fixOffer: FixOffer?

    /// 一覧の記録を消した直後の「削除しました・元に戻す」(M11・`AppRuntime+RecordUndo.swift`)。
    var recordUndo: RecordUndo?

    /// 生成(morph)まわりのエラー **状態カード**(#60)。音声画面 / 生成後の画面上部に出し、主ボタンで復帰する。nil なら正常。
    var errorCard: ErrorPresentation? {
        didSet {
            #if DEBUG
            if let errorCard { print("[Metamon] errorCard: \(errorCard.kind) transcript=\(errorCard.transcript)") }
            else if oldValue != nil { print("[Metamon] errorCard: cleared") }
            #endif
            if let errorCard, errorCard.id != oldValue?.id { VoiceOverAnnouncer.announce(errorCard.announcement) } // H8
        }
    }
    /// 401 が再取得後も続いた時(セッション切れ)。`MetamonApp` が `AuthSession.expire()` を結ぶ。
    @ObservationIgnored var onSessionExpired: (@MainActor () -> Void)?
    /// 今のユーザーが匿名か(#276: 引き継ぎの案内)。`MetamonApp` が `AuthSession` を結ぶ。nil は匿名でない扱い。
    @ObservationIgnored var isAnonymousUser: (@MainActor () -> Bool)?
    /// 匿名ユーザーが初めてアプリを作った次の起動で 1 回だけ出す「引き継ぎのためにサインイン」(#276・H7・`AccountLinkPromptHost`)。
    var showLinkPrompt = false
    @ObservationIgnored var linkPromptDelay: Duration = .seconds(1.5)
    /// 圏外で送れなかった発話を 1 件保留し、復帰で 1 回だけ送る。
    @ObservationIgnored private let deferredMorph: DeferredSend<String>
    @ObservationIgnored private var connectivityTask: Task<Void, Never>?
    /// 生成中に圏外になって自分でキャンセルした(ユーザーのキャンセルと区別する)
    @ObservationIgnored private var interruptedByOffline = false

    /// 生成中の状態。nil なら生成していない。`create` は初期状態からの生成(全画面演出)、`edit` は 2 回目以降(薄いインジケータ)。
    struct MorphJob: Equatable {
        enum Kind { case create, edit }
        var kind: Kind
        var transcript: String
        var startedAt: Date
        /// SSE で届いた途中経過(#636・create だけ)。組み立て台に仮置きする
        var draft = BuildChoreography.LiveDraft()

        /// 生成中の見せ方。初期画面からの作成に加え、生成後の画面からの「〇〇アプリを作って」も作り直し(mode=full)になるので
        /// 全画面の「変身中」にする。それ以外の改変は今の画面を薄くして上に小さなインジケータ(#182)。
        static func kind(isVoiceOnly: Bool, transcript: String) -> Kind {
            isVoiceOnly || VoiceIntent.isWholeAppRequest(transcript) ? .create : .edit
        }
    }
    private(set) var morphJob: MorphJob?
    var isMorphing: Bool { morphJob != nil }
    /// create の結果が届いた後、本物の部品を 1 つずつ置く演出の間だけ立つ(#626・`AppRuntime+BuildStage.swift`)。
    /// 結果はもう適用済み(`morphJob` は nil)で、RootView は生成中の表示を続けて組み上げを見せる。
    var buildReveal: BuildReveal?
    /// 編集の直後に光らせる部品(#626)。Renderer の出現時に読む
    var changeHighlight: ChangeHighlight = .none
    @ObservationIgnored var buildRevealTask: Task<Void, Never>?
    /// Reduce Motion(テストで差し替える)
    @ObservationIgnored var reduceMotion: () -> Bool = { UIAccessibility.isReduceMotionEnabled }

    /// 変身直後に画面上部へ数秒出す要約(読み上げ文)。位置ヒントがあれば該当領域をハイライトする。
    struct SummaryBanner: Identifiable, Equatable {
        /// バナーに添えるボタン(#209)。undo = 「元に戻す」(修正の直後)/ redo = 「やり直す」(戻した直後)
        enum Action: Equatable { case undo, redo }
        let id = UUID()
        var text: String
        var highlight: UnitPoint?
        var action: Action? = nil
    }
    var summaryBanner: SummaryBanner? {
        didSet { if let summaryBanner, summaryBanner.id != oldValue?.id { VoiceOverAnnouncer.announce(summaryBanner.text) } } // H8
    }

    /// 「やり直す」の戻り先(#209)。戻す直前の版。サーバー経由なら `version` を restore、ローカルなら `document` を当て直す。
    struct RedoPoint {
        var version: Int
        var document: AppDocument
        var viaServer: Bool
    }
    /// 戻した直後だけ持つ。続けて戻すと積み、やり直すと 1 つずつ降ろす。新しい DSL が当たったら(`apply`)消える
    private(set) var redoStack: [RedoPoint] = []
    /// 次の「やり直す」の戻り先。
    var redoPoint: RedoPoint? { redoStack.last }

    /// サーバー(appconfig)に届かず、ローカルキャッシュの DSL を表示している。
    private(set) var isOffline = false

    // MARK: 生成前の対話(#75)。ロジックは `AppRuntime+Clarify.swift`

    /// チャットの吹き出し。user = 発話(書き起こし)/ assistant = AI の質問・答え・案の前置き。
    struct ChatBubble: Identifiable, Equatable {
        enum Role: Equatable { case user, assistant }
        let id = UUID()
        var role: Role
        var text: String
        /// assistant の時だけ。応答の kind(答えをもらった質問を依頼文から除くのに使う)
        var kind: ChatTurn.Kind? = nil
        /// 個人の情報を公開してよいかの確認(#253 `confirmPublic`)。吹き出しに公開の印を付ける
        var confirmsPublic = false
    }
    /// 会話中の吹き出し(空 = 対話していない)。初期画面(create)と生成後の音声シート(edit・#186)で使う。
    var chatBubbles: [ChatBubble] = [] {
        didSet { if chatBubbles != oldValue { persistChatHistory() } } // #210 再起動しても続きが見えるよう吹き出しごとに保存
    }
    /// 閉じた会話と反映の区切り(#210・`AppRuntime+ChatHistory.swift`)。今の会話(`chatBubbles`)は含まない
    var chatHistory: [ChatHistoryEntry] = []
    /// 履歴の保存先(テストで差し替える)
    @ObservationIgnored var chatHistoryStore: ChatHistoryStore = .forRuntime() {
        didSet { reloadChatHistory() }
    }
    /// `POST /v1/chat` の応答待ち
    var chatPending = false
    /// 直近の応答。question = チップ / proposal = 案のカード / ready = 要約 +「これでいく」(`ClarifyFlow.offer`)
    var chatTurn: ChatTurn?
    /// edit のチャットから生成に回した時の会話(#211)。生成しても何も変わらなかったら、この会話に理由の吹き出しを足して音声シートを開き直す
    var chatReturnOnUnchanged: [ChatBubble]?
    /// 上の会話を閉じる前の履歴。開き直す時はこれに戻す(閉じた時に履歴へ移した同じ会話が二重に残り、件数の上限で古い履歴を押し出していた)
    @ObservationIgnored var chatHistoryOnUnchanged: [ChatHistoryEntry]?
    /// 生成・改変の前にチャットで相談するか(設定で切替・既定 ON・#186)。OFF は create も edit も従来どおり即反映。
    var clarifyEnabled: Bool = ClarifySettings.enabled {
        didSet { ClarifySettings.enabled = clarifyEnabled }
    }
    /// 音声だけで進めている時、ready で自動生成するか(#160・既定 ON)。
    var clarifyAutoBuild: Bool = ClarifySettings.autoBuild {
        didSet { ClarifySettings.autoBuild = clarifyAutoBuild }
    }
    /// ready 後の自動生成待ち(要約カードを見せている 1.5 秒)。ボタンの文言を変える
    var chatAutoBuildPending = false
    /// 最後に吹き出しを足した入力手段(自動生成の判定)
    @ObservationIgnored var chatLastInput: ClarifyFlow.InputSource?
    /// 次のラリーの区間計測(録音停止・文字起こし完了の時刻。音声のときだけ)
    @ObservationIgnored var chatTiming = ChatTiming()
    @ObservationIgnored var chatTask: Task<Void, Never>?
    @ObservationIgnored var chatAutoBuildTask: Task<Void, Never>?
    /// リセットのたびに進める。古い応答(待っている間に「最初から」された等)を捨てるため
    @ObservationIgnored var chatEpoch = 0

    #if !APP_STORE_REVIEW // 審査用ビルドは App 内課金を持たない(#450 G1)
    // MARK: 有料機能の確認カード(#77)。ロジックは `Features/Purchase/AppRuntime+PaidFeature.swift`

    /// 「これでいく」で有料機能を含むと分かった時のカード(nil = 出していない)。チャットの「これでいく」の位置に出す
    var paidConfirm: PaidConfirmation?
    /// カードの購入処理中
    var paidConfirmBusy = false
    /// カードの下に出す一言(承認待ち・購入できなかった等)
    var paidConfirmMessage: String?
    /// 課金の司令塔(既定は共有の 1 つ。テストで差し替える)
    @ObservationIgnored var proGate: ProGate = .shared
    /// 無料枠切れで止めた新規作成(H3)。会話はそのまま残し、チャットに「続きから作る」を出す(`AppRuntime+HeldBuild.swift`)
    var heldBuild: HeldBuild?
    /// 共有の記録が入った版の課金画面を出すまでの間(H7: 要約バナーより先に課金の判断をしてもらう)
    var paidGatePending = false
    #endif
    /// 上限・402 で作らずに止めた時、create の会話を消さない(H3)。`runFromChat` が読んで戻す
    @ObservationIgnored var keepChatAfterStop = false
    /// 「最初から」で、これまでの会話も消すかの確認(M9)
    var showRestartConfirm = false

    /// 生成直後の「個人の情報も公開されます」の確認(#253・nil = 出していない)。`Features/Privacy/AppRuntime+PublicData.swift`
    var publicDataPrompt: PublicDataPrompt?
    /// 記録の保存期間の確認(#482・nil = 出していない)。`Features/Retention/AppRuntime+Retention.swift`
    var retentionPrompt: RetentionPrompt?
    /// 「あとで」にした確認("appId/entity=日数")。このセッションでは聞き直さない
    @ObservationIgnored var retentionDeferred: Set<String> = []
    /// 期限切れを消している最中の "appId/entity"(同じ行を 2 回消さない)
    @ObservationIgnored var retentionPurging: Set<String> = []

    let services: Services
    /// レコードのローカル DB + 未送信キュー(Issue #48)。`records` はここから読み、書きはここに即反映してから裏で送る。
    let recordSync: RecordSyncEngine
    @ObservationIgnored private var syncTask: Task<Void, Never>?
    @ObservationIgnored private var morphTask: Task<MorphResult, Error>?
    @ObservationIgnored private var isForeground = true
    /// 「元に戻して」のフォールバック(appconfig が versions を持たない mock / 未デプロイ時)。直近 10 件。
    /// 「元に戻す」ボタンの出し分けに使うので観測する(#209)。
    private var localHistory: [AppDocument] = []
    /// `openUrl` で外部に渡す処理(テストで差し替える)。
    @ObservationIgnored var openExternalURL: @MainActor (URL) async -> Bool = { await UIApplication.shared.open($0) }

    /// - Parameter recordSync: テストで到達性を固定したい時に渡す(既定は `RecordSyncFactory`)
    init(document: AppDocument, services: Services, recordSync injected: RecordSyncEngine? = nil) {
        self.document = document
        self.services = services
        self.recordSync = injected ?? RecordSyncFactory.make(client: services.appConfig)
        self.deferredMorph = DeferredSend(connectivity: recordSync.connectivity)
        self.selectedTab = document.dsl.initialTab
        recordSync.onEvent = { [weak self] event in self?.handle(event) }
        recordSync.encryptedFields[document.appId] = document.dsl.encryptedFields // #233
        reloadChatHistory()
    }

    // MARK: - Lifecycle

    /// 起動時: live ならサーバーの最新 DSL を取得しホットリロード開始。文字起こしモデルも温めておく。
    /// サーバーに届かなければ最後に取得した DSL(`DocumentCache`)を出し、トーストで「オフライン」を知らせる。
    /// 通知の同期(許可ダイアログを出し得る)は復元・同期の **後ろで await せずに** 回す(#86: ダイアログ待ちで復元が止まらない)。
    func bootstrap() async {
        prepareTranscriberIfAllowed()
        recordSync.start()
        observeConnectivity()
        if services.isLive {
            await restoreFromServer()
            if DeviceAppID.issuedThisLaunch { await resumeLatestAppIfEmpty() } // #276: 再インストール後のサインイン
            startSync()
        }
        noteRetention() // #482 起動時の DSL(モック・オフラインでは apply を通らない)
        presentDeferredAccountLinkIfNeeded() // #276・H7 前の起動で最初のアプリを作っていたら引き継ぎの案内
        Task { await syncNotificationsNow() }
        #if DEBUG
        autoNavigateIfRequested() // `METAMON_AUTO_NAV`(#113)
        // ヘッドレス E2E 用: `METAMON_E2E_PRESS_PRIMARY_AFTER=<秒>` で、その時点で状態カードが出ていれば主ボタン(もう一度 等)を押す(#60)
        if let raw = ProcessInfo.processInfo.environment["METAMON_E2E_PRESS_PRIMARY_AFTER"], let seconds = Double(raw), seconds > 0 {
            Task { [weak self] in
                try? await Task.sleep(for: .seconds(seconds))
                guard let self, let card = errorCard, let primary = card.primary else { print("[Metamon] E2E: no error card to act on"); return }
                print("[Metamon] E2E: pressing primary \(primary) on \(card.kind)")
                handle(primary, of: card)
            }
        }
        // スクショ用: `METAMON_SHOW_ERROR_CARD=1` で圏外の状態カードを出す(焦りのツル・#102)
        if ProcessInfo.processInfo.environment["METAMON_SHOW_ERROR_CARD"] == "1" {
            errorCard = ErrorPresentation(kind: .offlineQueued, transcript: "家計簿アプリが欲しい")
        }
        // ヘッドレス E2E 用: `METAMON_AUTO_TRANSCRIPT="家計簿アプリが欲しい"` で起動直後に 1 回だけ発話を投げる。
        // 対話が ON(#75)なら質問に `METAMON_AUTO_REPLIES`(| 区切り)で答え、ready で「この内容で作る」まで自動で進めて生成を待つ
        if let text = ProcessInfo.processInfo.environment["METAMON_AUTO_TRANSCRIPT"], !text.isEmpty {
            await autoDrive(firstUtterance: text)
        }
        // ヘッドレス E2E 用: `METAMON_AUTO_RECORD='{"entity":"expense","data":{"amount":980,...}}'` で起動直後に 1 件保存する
        if let raw = ProcessInfo.processInfo.environment["METAMON_AUTO_RECORD"], let data = raw.data(using: .utf8),
           let spec = try? JSONDecoder().decode(AutoRecord.self, from: data) {
            records[spec.entity] = await recordSync.save(appId: document.appId, entity: spec.entity, recordId: nil, data: spec.data,
                                                         author: postAuthor(for: spec.entity))
            print("[Metamon] auto record saved locally: entity=\(spec.entity) count=\(records[spec.entity]?.count ?? 0) sync=\(recordSync.status.label)")
        }
        // スクショ用: `METAMON_AUTO_AUTHOR_PROMPT=1` で起動直後に「投稿に表示する名前」を聞くアラートを出す(#250)
        if ProcessInfo.processInfo.environment["METAMON_AUTO_AUTHOR_PROMPT"] == "1" {
            try? await Task.sleep(for: .seconds(2))
            authorNamePrompt = AuthorNamePrompt {}
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
            if errorCard?.title == ErrorCopy.unreachableTitle { errorCard = nil } // つながったら起動時の知らせを下げる
            if remote.version > document.version || (remote.version == document.version && document.version == 0) {
                apply(remote, animated: false)
            }
            print("[Metamon] restore: server version=\(remote.version) shown=\(document.version) screens=\(dsl.screens.map(\.id))")
        } catch {
            print("[Metamon] restore failed: \(error)")
            if let cached = DocumentCache.shared.load(appId: document.appId), cached.version >= document.version {
                apply(cached, animated: false)
                isOffline = true
                toast = ErrorCopy.offlineRestored
            } else {
                isOffline = true
                // H9: OS のエラー文は出さず、消えない状態カードで次の一手を(中身は上の restore failed のログ)
                errorCard = .notice(icon: "wifi.exclamationmark", title: ErrorCopy.unreachableTitle, body: ErrorCopy.unreachableBody)
            }
        }
    }

    /// 到達性を見張る(#60): 生成中に圏外になったら即キャンセルして「通信が切れました」、復帰したら未取得の entity を読み直す。
    private func observeConnectivity() {
        connectivityTask?.cancel()
        connectivityTask = Task { [weak self] in
            guard let self else { return }
            var previous: Bool?
            for await online in recordSync.connectivity.updates() {
                defer { previous = online }
                if !online, morphJob != nil, morphTask != nil {
                    interruptedByOffline = true
                    morphTask?.cancel()
                }
                if online, previous == false {
                    let retry = failed
                    for entity in retry { await refresh(entity) }
                    if isOffline, services.isLive { await restoreFromServer() }
                }
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
            for await update in services.sync.events(appId: appId, after: version) {
                guard document.appId == appId else { break }
                switch update {
                case .document(let doc):
                    hotReload(doc)
                case .records(let entity):
                    scheduleRecordsRefresh(entity)
                case .realtime(let on):
                    let resumed = on && realtimeWasLost
                    realtimeWasLost = !on
                    isRealtime = on
                    print("[Metamon] realtime: \(on ? "listening" : "lost → polling")")
                    // 切れている間の他ユーザー投稿を取りこぼさないよう、表示中の entity を読み直す
                    if resumed { for entity in records.keys { scheduleRecordsRefresh(entity) } }
                }
            }
        }
    }

    /// リアルタイム購読(SSE)が稼働中か。false の間はポーリング(5 秒)で拾う。
    private(set) var isRealtime = false
    @ObservationIgnored private var realtimeWasLost = false
    /// 入力中で差し替えを保留している DSL(閉じたら当てる)。
    @ObservationIgnored private var pendingHotReload: AppDocument?
    @ObservationIgnored private var pendingHotReloadTask: Task<Void, Never>?
    @ObservationIgnored private var recordsRefreshTasks: [String: Task<Void, Never>] = [:]
    /// 保存中のフォーム(saveRecord の連打で同じ記録を 2 件作らない)
    @ObservationIgnored private let saveFlight = SaveFlight()
    /// 最後に書き込んだフォーム。送った後にサーバーが断ったら「保存しました」を取り消す(M10 × #317 / #346 / #321)
    @ObservationIgnored var lastSavedForm: SavedForm?
    #if !APP_STORE_REVIEW
    /// roleField の無い aiChat の対(仮 id → 相方)。片方が断られたら相方も消す(AppRuntime+AIChat.swift)
    @ObservationIgnored let aiChatPairs = AIChatPairLedger()
    #endif
    struct SavedForm {
        let appId: String
        let entity: String
        /// 書いた行の id(新規は仮 id `local-…`)。断られた操作の `SyncEvent` の recordId と一致した時だけ取り消す
        let recordId: String
        weak var form: FormDraft?
        /// 送った入力。根のフォームは保存の後に空に戻すので、断られたらこれを戻す(#1723)
        var payload: [String: JSONValue]?
    }

    /// シート / 全画面 / push 先 / 音声シートが開いている = 入力や操作の途中。DSL を差し替えるとフォームの下書きが消えるので待つ。
    private var isInteracting: Bool {
        sheet != nil || fullScreen != nil || showVoiceSheet || paths.values.contains { !$0.isEmpty }
            || gridEdit != nil // 手で動かしている間に画面が差し替わらない(#1442)
    }

    /// 他端末 / ai 直叩きで進んだ DSL を当てる。操作中なら閉じるまで保留し、最新の 1 件だけ当てる。
    func hotReload(_ doc: AppDocument) {
        guard doc.version > document.version, morphJob == nil else { return }
        guard !isInteracting else {
            if doc.version > (pendingHotReload?.version ?? 0) { pendingHotReload = doc }
            print("[Metamon] hot reload deferred (interacting): version \(doc.version)")
            if pendingHotReloadTask == nil {
                pendingHotReloadTask = Task { [weak self] in
                    while let self, !Task.isCancelled, let pending = pendingHotReload {
                        try? await Task.sleep(for: .seconds(1))
                        guard !isInteracting else { continue }
                        pendingHotReload = nil
                        pendingHotReloadTask = nil
                        hotReload(pending)
                        return
                    }
                }
            }
            return
        }
        print("[Metamon] hot reload: version \(document.version) → \(doc.version)")
        apply(doc)
        summaryBanner = SummaryBanner(text: L10n.tr("最新の内容に更新しました"), highlight: nil)
    }

    /// records の変化通知はまとめて 1 回だけ取り直す(連投で pull を重ねない)。未表示の entity は画面が開いた時に読むので無視。
    private func scheduleRecordsRefresh(_ entity: String) {
        guard records[entity] != nil else { return }
        recordsRefreshTasks[entity]?.cancel()
        recordsRefreshTasks[entity] = Task { [weak self] in
            try? await Task.sleep(for: .milliseconds(250))
            guard let self, !Task.isCancelled else { return }
            recordsRefreshTasks[entity] = nil
            print("[Metamon] realtime: records changed entity=\(entity)")
            await refresh(entity)
        }
    }

    /// 前面の時だけ購読する。背面では止め、戻ったら今の version から張り直す。
    func setForeground(_ active: Bool) {
        guard isForeground != active else { return }
        isForeground = active
        recordSync.setForeground(active)
        if !active { recordUndo = nil } // 背面では削除を送り切るので、戻せない
        if active {
            startSync()
            Task { await refreshNotifications() } // #228 届いた分の空き枠を埋める
            Task { await catchUpAfterBackground() } // #38: 背面中の取りこぼしを即取り直す(AppRuntime+Background)
            Task { await purgeAllExpired() } // #482 背面の間に期限を過ぎた記録
        } else {
            syncTask?.cancel()
            // 背面の間の変化は通知されないので、復帰で購読が戻った時に表示中の records を読み直す
            isRealtime = false
            realtimeWasLost = true
        }
    }

    /// 新しい DSL に差し替える。ナビ・キャッシュは初期化(画面構成が変わるため)。通知は DSL に同期。
    /// `reveal`: create の結果の組み上げ演出(#626)。`highlight`: 編集で変わった部品(#626)。どちらも無い apply では消す。
    func apply(_ doc: AppDocument, animated: Bool = true, reveal: BuildReveal? = nil, highlight: ChangeHighlight = .none) {
        // 別アプリへの切替・新規 appId・生成完了では、進行中の対話を捨てる(サーバー側も DELETE)
        if !chatBubbles.isEmpty || chatPending { resetChat() }
        buildRevealTask?.cancel()
        let swap = {
            self.buildReveal = reveal
            self.changeHighlight = highlight
            self.document = doc
            self.records = [:]
            self.failed = []
            self.errorCard = nil
            self.paths = [:]
            self.sheet = nil
            self.fullScreen = nil
            self.showVoiceSheet = false
            self.selectedTab = doc.dsl.initialTab
            self.morphJob = nil
            self.gridEdit = nil // 手で動かしていた下書きは前の DSL のもの(別のアプリ・元に戻す後に残さない・#1442)
            self.redoStack = [] // 戻した後に別の版が当たったら「やり直す」は無効(undo / redo は apply の後で付け直す)
        }
        if animated {
            withAnimation(.spring(duration: 0.65, bounce: 0.12), swap)
        } else {
            swap()
        }
        scheduleBuildRevealEnd(reveal, highlight: highlight)
        scheduleNotificationSync()
        noteRetention() // #482 保存期間の確認と期限切れの削除
        noteAppMembers() // #272 参加者と管理者(member の選択肢・管理者だけのボタン)
        WidgetSnapshotWriter.write(dsl: doc.dsl, records: records) // ホーム画面ウィジェットへ stat を反映
        IntentHost.shared.appsMayHaveChanged() // Siri のフレーズ候補(アプリ名)を引き直す(#80)
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
        case .listApps:
            openSwitcher(afterSheetDismiss: !dsl.isVoiceOnly)
            return
        case .switchApp(let name):
            await switchApp(named: name)
            return
        case .morph:
            break
        }
        await generate(text)
    }

    /// 意図判定を済ませた依頼文を生成に回す(対話の ready からも直接ここへ: AI がまとめた文を「元に戻して」等と誤判定しないため)。
    func generate(_ text: String) async {
        guard morphJob == nil, requireAIConsent() else { return }
        // 圏外(#60): 送らずに保留し、状態カード「接続が戻ったら自動で送ります」。復帰で 1 回だけ自動送信(モックは常にオンライン)
        if services.isLive || recordSync.connectivity is ManualConnectivity,
           deferredMorph.holdIfOffline(text, send: { [weak self] queued in await self?.sendDeferred(queued) }) {
            withAnimation(.easeInOut(duration: 0.3)) { errorCard = ErrorPresentation(kind: .offlineQueued, transcript: text) }
            return
        }
        await sendMetered(text)
    }

    /// 課金(#77): 新規作成(初期画面から)は 1 日の無料枠を消費する。枠切れなら上限シートを出し、買えば同じ依頼でそのまま作る。
    /// 閉じたら送らず、会話を残して「続きから作る」を出す(H3・`holdBuild`)。成功した時だけ数える。
    /// 圏外の保留の自動送信・状態カードの「もう一度」もここを通す(#414: 枠を素通りしない)。
    /// 審査用ビルド(#450 G1)は端末で数えない。上限はサーバーの ratelimit だけ(429 は状態カードの「今日はここまでです」)。
    private func sendMetered(_ text: String) async {
        #if APP_STORE_REVIEW
        await send(text)
        #else
        let metered = dsl.isVoiceOnly
        if metered {
            let wasOpen = showVoiceSheet
            showVoiceSheet = false
            guard await proGate.admitNewApp(after: .milliseconds(wasOpen ? 450 : 250)) else {
                holdBuild(text)
                return
            }
        }
        await send(text)
        if metered, !dsl.isVoiceOnly { proGate.recordNewApp() }
        #endif
    }

    /// 復帰時の自動送信(`DeferredSend` から)。保留が生きていれば生成に進む。
    private func sendDeferred(_ text: String) async {
        guard errorCard?.kind == .offlineQueued, errorCard?.transcript == text else { return }
        print("[Metamon] back online → sending deferred transcript")
        await sendMetered(text)
    }

    /// ai に送って適用する本体。失敗は `FailureKind` で分類して状態カードに(トーストは使わない)。
    /// `gatePaid`: 適用の後に、共有の記録が新しく入っていれば課金画面を通す(#414・`AppRuntime+PaidGate.swift`)
    func send(_ text: String, gatePaid: Bool = true) async {
        guard morphJob == nil, requireAIConsent() else { return }
        withAnimation(.easeInOut(duration: 0.35)) {
            errorCard = nil
            morphJob = MorphJob(kind: MorphJob.kind(isVoiceOnly: dsl.isVoiceOnly, transcript: text), transcript: text, startedAt: .now)
        }
        #if DEBUG
        if morphJob?.kind == .create { MorphProbe.createStarted += 1 }
        #endif
        interruptedByOffline = false
        let previous = document
        let createdFromInitial = previous.dsl.isVoiceOnly
        // create は途中経過(#636)を受けて組み立て台に仮置きする。届いた順に MainActor で当てる
        let progress = morphJob?.kind == .create ? AsyncStream.makeStream(of: GenerateProgress.self) : nil
        let progressTask = progress.map { pair in
            Task { @MainActor [weak self, startedAt = morphJob?.startedAt] in
                for await p in pair.stream { self?.noteBuildProgress(p, startedAt: startedAt) }
            }
        }
        let task = Task { [services, appId = document.appId, version = document.version, locale = Self.generationLocale, sink = progress?.continuation] in
            if let sink {
                return try await services.ai.morph(transcript: text, appId: appId, currentVersion: version, locale: locale) { _ = sink.yield($0) }
            }
            return try await services.ai.morph(transcript: text, appId: appId, currentVersion: version, locale: locale)
        }
        morphTask = task
        defer {
            if morphTask == task { morphTask = nil }
            progress?.continuation.finish()
            progressTask?.cancel()
        }
        do {
            let result = try await task.value
            // 待っている間に別のアプリ・初期状態へ移っていたら、前のアプリの結果を今の画面に当てない
            guard document.appId == previous.appId else {
                print("[Metamon] morph result dropped: app changed \(previous.appId) → \(document.appId)")
                return
            }
            if result.unchanged {
                presentUnchanged(result.summary)
                return
            }
            remember(previous)
            // withAnimation の中で morphJob = nil も行う。create は本物の部品を置く演出を挟む・edit は変わった部品を光らせる(#626)
            let stage = buildStagePlan(job: morphJob, previous: previous, next: result.document)
            apply(result.document, reveal: stage.reveal, highlight: stage.highlight)
            if previous.dsl.isVoiceOnly { offerAccountLinkIfNeeded() } // #276 次の起動に予約(H7)
            noteChatApplied(version: result.document.version, summary: result.summary) // #210 反映の区切り
            notePublicPersonalData(result) // #253 公開の記録に個人の情報が入ったら確認
            startSync()
            if let summary = result.summary?.trimmingCharacters(in: .whitespacesAndNewlines), !summary.isEmpty {
                // 部品が光る時は位置の円を出さない(M7・二重に指さない)
                summaryBanner = SummaryBanner(text: summary, highlight: stage.highlight.isEmpty ? PositionHint.anchor(in: summary) : nil, action: canUndo ? .undo : nil)
                SpeechReader.shared.speak(summary) // #43(設定 OFF なら何もしない)
            } else {
                // 要約が無くても完成は伝える(H8・バナーの didSet が読み上げる要約の代わり)
                VoiceOverAnnouncer.announce(ErrorCopy.completedAnnouncement(appName: result.document.dsl.app.name, created: createdFromInitial))
            }
            scheduleFirstEditHint(createdFromInitial: createdFromInitial) // #274 最初のアプリの直後に 1 回だけ
            print("[Metamon] morph ok: version=\(result.document.version) mode=\(result.mode ?? "-") screens=\(result.document.dsl.screens.map(\.id)) summary=\(result.summary ?? "")")
            if gatePaid { await gatePaidGeneration(before: previous) } // #414
        } catch {
            guard document.appId == previous.appId else { return } // 移った先に前のアプリの失敗を出さない
            #if !APP_STORE_REVIEW
            // #630: コインが足りない → お店・枠を使い切った → コインの設定。会話は残す(買ってから送り直せる)
            if !interruptedByOffline, let denial = CoinDenial(error), CoinGate.shared.handle(denial) {
                withAnimation(.easeInOut(duration: 0.3)) { morphJob = nil }
                keepChatAfterStop = true
                return
            }
            #endif
            if !interruptedByOffline, let pro = ProRequired(error) { // #316: 有料の機能が入るので保存されなかった
                withAnimation(.easeInOut(duration: 0.3)) { morphJob = nil }
                await presentProRequired(pro, transcript: text, gatePaid: gatePaid)
                return
            }
            let failure: FailureKind = interruptedByOffline ? .offline : FailureKind.classify(error)
            interruptedByOffline = false
            print("[Metamon] morph failed: \(failure) (\(error))")
            withAnimation(.easeInOut(duration: 0.3)) { morphJob = nil }
            switch failure { // H3・M3: 上限・無料プランの停止でも会話は消さない(カードから次の一手)
            case .rateLimited, .planLimit, .appLimit, .freePlanPaused: keepChatAfterStop = true
            default: break
            }
            await present(failure, transcript: text)
        }
    }

    /// 途中経過(#636)を今の create の組み立て台に当てる。別の生成・終わった後に届いた分は捨てる。
    private func noteBuildProgress(_ p: GenerateProgress, startedAt: Date?) {
        guard let job = morphJob, job.kind == .create, job.startedAt == startedAt else { return }
        morphJob?.draft.apply(p)
    }

    /// 生成しても何も変わらなかった時(#211)。保存されていないので適用も履歴も触らず、理由を伝える。
    /// チャットから来たなら会話に吹き出しを足して開き直し(続けて話せる)、それ以外は要約バナーに出す。
    private func presentUnchanged(_ summary: String?) {
        let trimmed = summary?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
        let reason = trimmed.isEmpty ? L10n.tr("変更できませんでした。何をどう変えるか、もう少し詳しく教えてください。") : trimmed
        print("[Metamon] morph unchanged: \(reason)")
        withAnimation(.easeInOut(duration: 0.3)) { morphJob = nil }
        if let bubbles = chatReturnOnUnchanged {
            chatReturnOnUnchanged = nil
            if let saved = chatHistoryOnUnchanged { chatHistory = saved } // 吹き出しの didSet が保存するので先に戻す
            chatHistoryOnUnchanged = nil
            withAnimation(.spring(duration: 0.4, bounce: 0.15)) {
                chatTurn = nil
                chatBubbles = bubbles + [ChatBubble(role: .assistant, text: reason, kind: .answer)]
            }
            showVoiceSheet = true
        } else {
            summaryBanner = SummaryBanner(text: reason, highlight: nil)
        }
        SpeechReader.shared.speak(reason)
    }

    /// 失敗の分類ごとに「次の一手」を出す。conflict は最新を取り直して短いバナー、cancelled は軽微なのでトースト、それ以外は状態カード。
    private func present(_ failure: FailureKind, transcript: String) async {
        switch failure {
        case .cancelled:
            toast = ErrorCopy.cancelledToast
        case .conflict:
            // appconfig の version 競合(ai 側の baseVersion 無しリトライでも解けなかった)。最新 DSL を取り直して再適用
            if services.isLive, let latest = try? await services.appConfig.fetchDocument(appId: document.appId) {
                if latest.version != document.version { apply(latest) }
                startSync()
            }
            summaryBanner = SummaryBanner(text: ErrorCopy.conflictBanner, highlight: nil)
        case .sessionExpired:
            withAnimation(.easeInOut(duration: 0.3)) { errorCard = ErrorPresentation.from(failure, transcript: transcript) }
            onSessionExpired?()
        case .appLimit:
            // #630: カードを残し(閉じても「置き換える」で開き直せる)、置き換えシートをすぐ出す
            guard let card = ErrorPresentation.from(failure, transcript: transcript) else { return }
            withAnimation(.easeInOut(duration: 0.3)) { errorCard = card }
            handle(.replaceApp, of: card)
        default:
            withAnimation(.easeInOut(duration: 0.3)) { errorCard = ErrorPresentation.from(failure, transcript: transcript) }
        }
    }

    /// 状態カードの主ボタン。もう一度 / 今すぐ送る = 同じ発話で再送、サインイン = セッション切れ処理。
    func handle(_ primary: ErrorPresentation.Primary, of card: ErrorPresentation) {
        if card.origin == .chat, primary == .retry {
            withAnimation(.easeInOut(duration: 0.3)) { errorCard = nil }
            retryChat()
            return
        }
        switch primary {
        case .retry, .sendNow:
            deferredMorph.cancel()
            withAnimation(.easeInOut(duration: 0.3)) { errorCard = nil }
            Task { await sendMetered(card.transcript) }
        case .signIn:
            withAnimation(.easeInOut(duration: 0.3)) { errorCard = nil }
            onSessionExpired?()
        case .signInApple:
            showLinkPrompt = true // 匿名の上限(M3): Apple に引き継ぐと上限が増える。カードは残す(サインイン後に「これでいく」)
        case .upgradePro:
            #if !APP_STORE_REVIEW
            Task { await upgradeFromRateLimit(card) }
            #else
            break // 審査用の版はこのボタンを出さない(`ErrorPresentation.rateLimitPrimary`)
            #endif
        case .replaceApp:
            guard case .appLimit(let limit) = card.kind else { return }
            presentAppLimit(limit) { [weak self] in
                guard let self, self.errorCard == card else { return } // 待っている間に閉じた・話し直した
                withAnimation(.easeInOut(duration: 0.3)) { self.errorCard = nil }
                if card.origin == .chat {
                    self.retryChat()
                } else if !card.transcript.isEmpty {
                    await self.sendMetered(card.transcript)
                }
            }
        }
    }

    #if !APP_STORE_REVIEW
    /// 上限のカードの「Pro にする」(M3)。買えば(サーバーが Pro を知ってから)同じ依頼で続ける。閉じたらカードのまま。
    private func upgradeFromRateLimit(_ card: ErrorPresentation) async {
        // 音声シートの中のカード(edit の対話)はシートの中から出す(根の課金画面はシートに隠れる)
        var reason = PaywallReason.limitReached
        if case .planLimit = card.kind { reason = .settings } // #317: 今日の上限ではなくプランの上限
        guard await proGate.requestPro(reason, placement: showVoiceSheet ? .chat : .root) else { return }
        await proGate.store.serverSyncTask?.value
        guard errorCard == card else { return } // 待っている間に閉じた・話し直した
        withAnimation(.easeInOut(duration: 0.3)) { errorCard = nil }
        if card.transcript.isEmpty { return } // 記録の保存で当たった上限(#317)は再送する発話が無い
        if card.origin == .chat {
            retryChat()
        } else if isChatting, dsl.isVoiceOnly {
            await runFromChat(card.transcript)
        } else {
            await sendMetered(card.transcript)
        }
    }
    #endif

    /// 状態カードの副ボタン。やめる = 保留を取り消す、閉じる = カードを消す(発話は残らない)。
    func handle(_ secondary: ErrorPresentation.Secondary, of card: ErrorPresentation) {
        if case .discard = secondary { deferredMorph.cancel() }
        withAnimation(.easeInOut(duration: 0.3)) { errorCard = nil }
    }

    /// 生成の中断(30 秒超の長引きで UI から呼ぶ)。URLSession は Task キャンセルで切れる。
    func cancelMorph() {
        morphTask?.cancel()
    }

    /// 「元に戻す」を押せるか(#209)。作ったアプリで、サーバーに前の版がある(version > 1)か端末の履歴がある時。
    var canUndo: Bool {
        !dsl.isVoiceOnly && ((services.isLive && document.version > 1) || !localHistory.isEmpty)
    }

    /// 「元に戻して」(G11)。appconfig の `GET /versions` から戻す先を選び(`UndoWalk`)`POST /versions/{n}/restore`。
    /// restore の版は中身の元に読み替えるので、続けて押すと v5 → v4 → v3 … と遡る(直前 2 つの往復にならない)。
    /// サーバーが履歴を返せない時(mock / 未デプロイ)はローカル履歴にフォールバックする。
    /// 戻している間は `morphJob` が立つので二重には走らない。戻した後は「やり直す」用に戻す前の版を覚える(#209)。
    /// `expectedVersion`: ボタンを押した時点の版。押した後に版が変わっていたら(連打で 2 回目が来た等)何もしない。
    func undo(expectedVersion: Int? = nil) async {
        guard morphJob == nil, expectedVersion == nil || expectedVersion == document.version else { return }
        let before = document
        let stack = redoStack
        withAnimation(.easeInOut(duration: 0.35)) {
            morphJob = MorphJob(kind: .edit, transcript: L10n.tr("元に戻して"), startedAt: .now)
        }
        if services.isLive {
            do {
                let versions = try await services.appConfig.listVersions(appId: document.appId)
                guard !versions.isEmpty else { throw APIError.server(status: 404, code: "not_found", message: "no versions") }
                let byVersion = Dictionary(versions.map { ($0.version, $0) }, uniquingKeysWith: { a, _ in a })
                let shown = UndoWalk.origin(of: before.version, in: byVersion)
                guard let target = UndoWalk.undoTarget(current: before.version, versions: versions) else {
                    withAnimation(.easeInOut(duration: 0.3)) { morphJob = nil }
                    toast = L10n.tr("これ以上前の状態はありません")
                    return
                }
                let doc = try await services.appConfig.restore(appId: document.appId, toVersion: target)
                apply(doc)
                startSync()
                // restore は新しい版を作るので、「やり直す」は戻す前の中身の版(shown)をもう一度 restore すればよい
                redoStack = stack + [RedoPoint(version: shown, document: before, viaServer: true)]
                summaryBanner = SummaryBanner(text: L10n.tr("元に戻しました"), highlight: nil, action: .redo)
                print("[Metamon] undo ok: restored from version \(target) → new version \(doc.version)")
                await gatePaidRestore(before: before) // #414 共有の版に戻した時も課金画面を通す
                return
            } catch {
                print("[Metamon] undo via server failed, falling back to local history: \(error)")
            }
        }
        guard let previous = localHistory.popLast() else {
            withAnimation(.easeInOut(duration: 0.3)) { morphJob = nil }
            toast = L10n.tr("これ以上前の状態はありません")
            return
        }
        apply(previous)
        redoStack = stack + [RedoPoint(version: before.version, document: before, viaServer: false)]
        summaryBanner = SummaryBanner(text: L10n.tr("元に戻しました"), highlight: nil, action: .redo)
        await gatePaidRestore(before: before)
    }

    /// 「やり直す」(#209): 戻す前の版に戻る(続けて戻した分は 1 つずつ前へ進む)。サーバー経由で戻したならその版を restore、端末の履歴で戻したなら当て直す。
    func redo(expectedVersion: Int? = nil) async {
        guard morphJob == nil, expectedVersion == nil || expectedVersion == document.version, let point = redoStack.last else { return }
        let rest = redoStack.dropLast()
        let current = document
        withAnimation(.easeInOut(duration: 0.35)) {
            morphJob = MorphJob(kind: .edit, transcript: L10n.tr("やり直す"), startedAt: .now)
        }
        if point.viaServer {
            do {
                let doc = try await services.appConfig.restore(appId: document.appId, toVersion: point.version)
                apply(doc)
                startSync()
            } catch {
                print("[Metamon] redo failed: \(error)")
                withAnimation(.easeInOut(duration: 0.3)) { morphJob = nil }
                toast = L10n.tr("やり直せませんでした")
                return
            }
        } else {
            remember(current)
            apply(point.document)
        }
        redoStack = Array(rest) // 続けて戻していたら、残りもやり直せる
        summaryBanner = SummaryBanner(text: L10n.tr("やり直しました"), highlight: nil, action: canUndo ? .undo : nil)
        await gatePaidRestore(before: current)
    }

    func remember(_ doc: AppDocument) {
        localHistory.append(doc)
        if localHistory.count > 10 { localHistory.removeFirst() }
    }

    /// 「別のアプリ。」: appId を新規採番して初期状態にする(サーバー上の前のアプリは残る)。続けて ai に送る前提。
    /// 変身一覧の「+ 新しいアプリ」「Tailor It に戻る」からも呼ぶ(`entry` で挨拶を出し分け・アニメ付き)。
    func startNewApp(entry: HomeEntry = .newApp, animated: Bool = false) {
        syncTask?.cancel()
        localHistory.removeAll()
        var fresh = (try? DSLExamples.document(DSLExamples.initial, version: 0)) ?? .fallback
        fresh.appId = DeviceAppID.rotate()
        homeEntry = entry
        apply(fresh, animated: animated)
        print("[Metamon] new app: appId=\(fresh.appId) entry=\(entry)")
    }

    // MARK: - Switcher (#51 複数アプリの切替)

    /// 変身一覧シートを開く(FAB 長押し / 「アプリ一覧」)。開く直前に今の画面をサムネに撮る。
    /// 音声シートが出ている(いた)時は閉じ終わるのを待ってから撮って開く(シート越しのサムネ・二重 present を避ける)。
    func openSwitcher(afterSheetDismiss: Bool = false) {
        guard !showAppSwitcher else { return }
        let waitForSheet = showVoiceSheet || afterSheetDismiss
        showVoiceSheet = false
        Task {
            if waitForSheet { try? await Task.sleep(for: .milliseconds(500)) }
            AppThumbnailStore.shared.capture(appId: document.appId, isVoiceOnly: dsl.isVoiceOnly)
            showAppSwitcher = true
        }
    }

    /// `GET /v1/apps`。失敗したら `AppListCache`(オフライン)に落ちる。結果は `apps` にも残す。
    @discardableResult
    func appsList() async -> [AppSummary] {
        appsLoading = apps.isEmpty
        defer { appsLoading = false }
        do {
            apps = try await services.appConfig.listApps()
        } catch {
            print("[Metamon] listApps failed: \(error)")
            let cached = AppListCache.shared.load()
            if !cached.isEmpty {
                apps = cached
                toast = L10n.pick(L10n.tr("オフラインです。保存済みの変身一覧を表示しています"), review: L10n.tr("オフラインです。保存済みの記録帳の一覧を表示しています"))
            } else if apps.isEmpty {
                toast = ErrorCopy.failure(L10n.pick(L10n.tr("変身一覧を読み込めませんでした"), review: L10n.tr("記録帳の一覧を読み込めませんでした")), error, context: "apps list")
            }
        }
        return apps
    }

    /// 一覧に出すアプリ(archived・未生成・削除中を除き、現在のアプリを先頭に)。
    var visibleApps: [AppSummary] { AppSummary.visible(appDeletion.filter(apps), current: document.appId) }

    /// 別のアプリに切り替える: 今の画面をサムネに撮り → `DeviceAppID` を差し替え → `GET /dsl` → `apply()`(変身アニメ)。
    /// 記録(records)は `apply()` で破棄され、画面が要求する entity を `load(entities:)` の既存経路で読み直す。
    func switchApp(to appId: String, afterSheetDismiss: Bool = false) async {
        let fromSwitcher = showAppSwitcher // 一覧から来たなら開く直前に撮ってある(今撮るとシートが写る)
        showAppSwitcher = false
        guard morphJob == nil, appId != document.appId else { return }
        if !fromSwitcher {
            if afterSheetDismiss { try? await Task.sleep(for: .milliseconds(500)) }
            AppThumbnailStore.shared.capture(appId: document.appId, isVoiceOnly: dsl.isVoiceOnly)
        }
        let previousId = document.appId
        withAnimation(.easeInOut(duration: 0.35)) {
            morphJob = MorphJob(kind: .edit, transcript: L10n.pick(L10n.tr("アプリを切り替えています…"), review: L10n.tr("記録帳を切り替えています…")), startedAt: .now)
        }
        syncTask?.cancel()
        var next: AppDocument?
        do {
            next = try await services.appConfig.fetchDocument(appId: appId)
            isOffline = false
        } catch {
            print("[Metamon] switch fetch failed: \(error)")
            next = DocumentCache.shared.load(appId: appId)
            if next != nil { isOffline = true }
        }
        guard var doc = next else {
            withAnimation(.easeInOut(duration: 0.3)) { morphJob = nil }
            toast = L10n.pick(L10n.tr("アプリを読み込めませんでした。接続を確認してください"), review: L10n.tr("記録帳を読み込めませんでした。接続を確認してください"))
            startSync()
            return
        }
        doc.appId = appId
        DeviceAppID.set(appId)
        localHistory.removeAll()
        apply(doc)
        startSync()
        // recordSync(オフライン DB)は呼び出しごとに appId を受けるので再起動不要。旧 appId の未送信はキューが引き続き送り、
        // 新 appId のレコードは refresh() がローカル先出し → サーバー pull で読み直す(イベントは document.appId で絞られる)
        summaryBanner = SummaryBanner(text: L10n.pick(L10n.tr("「\(doc.dsl.app.name)」に変身しました"), review: L10n.tr("「\(doc.dsl.app.name)」に切り替えました")) + (isOffline ? L10n.tr("(オフライン・キャッシュ)") : ""), highlight: nil)
        print("[Metamon] switched app: \(previousId) → \(appId) version=\(doc.version)")
    }

    /// 音声「○○に切り替えて」: `GET /apps` の name と照合。1 件に決まれば切替、曖昧/無ければ一覧を開く。
    func switchApp(named name: String) async {
        let fromVoiceSheet = !dsl.isVoiceOnly // 生成後の音声シートからの発話(シートは閉じ始めている)
        await appsList()
        let candidates = visibleApps
        switch AppNameMatch.match(name, in: candidates.map(\.name)) {
        case .one(let i):
            let target = candidates[i]
            if target.id == document.appId {
                summaryBanner = SummaryBanner(text: L10n.tr("いま使っているのが「\(target.name)」です"), highlight: nil)
                return
            }
            await switchApp(to: target.id, afterSheetDismiss: fromVoiceSheet)
        case .ambiguous:
            toast = L10n.pick(L10n.tr("「\(name)」に近いアプリが複数あります。一覧から選んでください"), review: L10n.tr("「\(name)」に近い記録帳が複数あります。一覧から選んでください"))
            openSwitcher(afterSheetDismiss: fromVoiceSheet)
        case .none:
            toast = L10n.pick(L10n.tr("「\(name)」というアプリは見つかりませんでした"), review: L10n.tr("「\(name)」という記録帳は見つかりませんでした"))
            openSwitcher(afterSheetDismiss: fromVoiceSheet)
        }
    }

    /// 変身一覧の「+ 新しいアプリ」(`.newApp`)と「Tailor It に戻る(初期画面)」(`.home`)。
    /// どちらも appId を作り直して初期(音声)画面へ。前のアプリはサーバーと一覧に残り、いつでも戻せる(サムネは一覧を開く時に撮ってある)。
    func startFresh(_ entry: HomeEntry) {
        showAppSwitcher = false
        startNewApp(entry: entry, animated: true)
        summaryBanner = nil
    }

    /// アプリを初期状態(音声画面のみ)に戻す。appId を作り直すので ai/appconfig 側からも「新規端末」に見える(旧データは削除しない)。
    func resetToInitial() {
        syncTask?.cancel()
        localHistory.removeAll()
        DocumentCache.shared.remove(appId: document.appId)
        var fresh = (try? DSLExamples.document(DSLExamples.initial, version: 0)) ?? .fallback
        fresh.appId = DeviceAppID.rotate()
        homeEntry = .home
        apply(fresh)
        summaryBanner = nil
        toast = L10n.tr("初期状態に戻しました")
    }

    // MARK: - Records

    func records(for entity: String) -> [Record] {
        #if APP_STORE_REVIEW
        records[entity] ?? []
        #else
        hidingBlocked(records[entity] ?? [], entity: entity) // #321: ブロックした人の投稿はすぐ隠す
        #endif
    }

    /// 共有レコードの作者判定(#249)と、`Field.hidden`(#262)の入力を作者のフォームにだけ出すのに使う uid。
    var currentUserId: String? { services.auth.currentUser?.uid }

    /// shared / public の entity に作るレコードの投稿者(#250): ローカル行の作者(uid)と、送る表示名(設定の名前だけ・未設定は nil)。
    /// private の entity は nil(表示名を送らない)。
    func postAuthor(for entity: String) -> (uid: String?, name: String?)? {
        guard dsl.entity(entity)?.isShared == true else { return nil }
        return (currentUserId, AuthorProfile.name)
    }

    /// 他人の共有レコードを変えようとした時の一言(#249)。端末で止めた時もサーバーの 403 でも同じ文言。
    func notifyReadOnly(entity: String) {
        toast = RecordActionSteps.readOnlyMessage
    }

    /// 一覧のチェックなど、その場で描いた変更を保存できず戻した時の一言(read-only と同じ下部の toast)。
    func notifySaveFailed(entity: String) {
        toast = RecordActionSteps.saveFailedMessage
    }

    private func record(_ entity: String, _ id: String) -> Record? {
        records[entity]?.first { $0.id == id }
    }

    /// 一覧のワンタップ チェック(#108 list.item.toggle)。saveRecord と同じくローカル DB に即反映し、送信は同期キュー。
    /// 表示中に無いレコード(削除済み等)は保存しない(save は recordId が見つからないと新規作成になるため)。
    func updateRecord(entity: String, recordId: String, data: [String: JSONValue]) async -> Bool {
        guard let current = record(entity, recordId) else { return false }
        guard canEdit(entity: entity, record: current) else {
            notifyReadOnly(entity: entity)
            return false
        }
        records[entity] = await recordSync.save(appId: document.appId, entity: entity, recordId: recordId, data: data)
        WidgetSnapshotWriter.write(dsl: dsl, records: records)
        return true
    }

    /// ローカル DB の内容で entity の表示を差し替える(印の付け外し #255: AppRuntime+Marks.swift)。
    func reloadCached(_ entity: String) async {
        records[entity] = await recordSync.cached(appId: document.appId, entity: entity)
    }

    /// `locationTracker`(#168)の位置 1 点。フォームを通さず 1 件足す(saveRecord と同じくローカル DB に即反映・送信は同期キュー)。
    func createRecord(entity: String, data: [String: JSONValue]) async -> Bool {
        await createRecordId(entity: entity, data: data) != nil
    }

    /// `createRecord` と同じ。作った行の id(仮 id `local-…`)を返す。保存できなければ nil。
    func createRecordId(entity: String, data: [String: JSONValue]) async -> String? {
        guard dsl.entity(entity) != nil else { return nil }
        let written = await recordSync.write(appId: document.appId, entity: entity, recordId: nil, data: data, author: postAuthor(for: entity))
        records[entity] = written.records
        WidgetSnapshotWriter.write(dsl: dsl, records: records)
        await purgeExpired(entity) // #482
        return written.recordId
    }

    func load(entities: Set<String>, force: Bool = false) async {
        for e in entities where force || records[e] == nil {
            await refresh(e)
        }
    }

    /// type=ref の options(初期の選択肢・#194)を、参照先 entity がサーバーでも空の時に端末ごと一度だけレコードにする。
    private func seedRefOptions(_ entity: String, appId: String) async {
        guard let tf = dsl.entity(entity)?.displayTitleField else { return }
        let names = dsl.entities.flatMap(\.fields).filter { ($0.type == .ref || $0.type == .refs) && $0.refEntity == entity }.flatMap { $0.options ?? [] }
        let key = "refSeed.\(appId).\(entity)"
        guard !names.isEmpty, !UserDefaults.standard.bool(forKey: key) else { return }
        UserDefaults.standard.set(true, forKey: key)
        var seen = Set<String>()
        for name in names where seen.insert(name).inserted {
            records[entity] = await recordSync.save(appId: appId, entity: entity, recordId: nil, data: [tf: .string(name)], author: postAuthor(for: entity))
        }
    }

    /// ローカル先出し → サーバー全件で置換 → UI 更新。サーバーに届かなければローカル表示のまま(通信断は toast しない)。
    func refresh(_ entity: String) async {
        let appId = document.appId
        if records[entity] == nil {
            let cached = await recordSync.cached(appId: appId, entity: entity)
            if !cached.isEmpty, document.appId == appId {
                records[entity] = cached
                WidgetSnapshotWriter.write(dsl: dsl, records: records) // ローカル先出し分もウィジェットへ
            }
        }
        loading.insert(entity)
        defer { loading.remove(entity) }
        do {
            let fresh = try await recordSync.pull(appId: appId, entity: entity)
            guard document.appId == appId else { return }
            records[entity] = fresh
            failed.remove(entity)
            if fresh.isEmpty { await seedRefOptions(entity, appId: appId) }
            await purgeExpired(entity) // #482 サーバーから届いた期限切れ(別の端末で作った自分の記録など)
            print("[Metamon] records loaded: entity=\(entity) count=\(fresh.count)")
            WidgetSnapshotWriter.write(dsl: dsl, records: records) // ホーム画面ウィジェットへ stat を反映
        } catch {
            guard document.appId == appId else { return }
            // ローカルにも無ければ「読み込めませんでした」(#85: 空状態と区別する)。ローカルがあればそのまま見せる
            if records[entity]?.isEmpty ?? true {
                records[entity] = []
                failed.insert(entity)
            }
            print("[Metamon] records pull failed (showing local): entity=\(entity) \(error)")
            // 通信断は画面内の placeholder に任せ、トーストは 4xx(権限・入力不備)だけ
            if case .rejected = FailureKind.classify(error) { toast = message(for: error, entity: entity) }
        }
    }

    /// ローカル DB の内容で表示を差し替える(画面外からの追加: Siri / ショートカット #80)。今のアプリ以外は何もしない。
    func reloadLocal(_ entity: String, appId: String) async {
        guard appId == document.appId else { return }
        let cached = await recordSync.cached(appId: appId, entity: entity)
        guard appId == document.appId else { return }
        records[entity] = cached
        WidgetSnapshotWriter.write(dsl: dsl, records: records)
    }

    /// 同期キューからの通知。送信完了はローカル DB を読み直し、競合はサーバーから引き直す(サーバー優先)。
    private func handle(_ event: SyncEvent) {
        Task { @MainActor in
            revokeSaveFeedback(for: event)
            #if !APP_STORE_REVIEW
            await revertAIChatPartner(for: event)
            #endif
            switch event {
            case .sent(let appId, let entity):
                PlanUsageStore.shared.invalidateApp(appId) // #317 記録・写真の量を読み直す
                guard appId == document.appId else { return }
                let cached = await recordSync.cached(appId: appId, entity: entity)
                guard appId == document.appId else { return } // 読む間に別のアプリへ切り替わった
                records[entity] = cached
                WidgetSnapshotWriter.write(dsl: dsl, records: records)
            case .conflict(let appId, let entity, _):
                guard appId == document.appId else { return }
                toast = L10n.tr("他の端末で変更されていたため、サーバーの内容に合わせました")
                await refresh(entity)
            case .forbidden(let appId, let entity, _):
                guard appId == document.appId else { return }
                notifyReadOnly(entity: entity)
                await refresh(entity) // 楽観的に書いた値をサーバーの内容に戻す
            case .rejected(let appId, let entity, let message, let field, _):
                guard appId == document.appId else { return }
                toast = rejectedToast(entity: entity, field: field, message: message)
                let cached = await recordSync.cached(appId: appId, entity: entity)
                guard appId == document.appId else { return } // 読む間に別のアプリへ切り替わった
                records[entity] = cached
                WidgetSnapshotWriter.write(dsl: dsl, records: records)
            case .paused(let pending):
                toast = L10n.tr("送信に失敗しました(未送信 \(pending) 件)。接続を確認してください")
            case .drained:
                break
            case .notMember(let appId, let entity, _):
                guard appId == document.appId else { return }
                toast = SharedAccessDenial.notMember.message // 送信待ちには残さず、楽観的に出した分を戻す
                let cached = await recordSync.cached(appId: appId, entity: entity)
                guard appId == document.appId else { return } // 読む間に別のアプリへ切り替わった
                records[entity] = cached
                WidgetSnapshotWriter.write(dsl: dsl, records: records)
            case .denied(let appId, let entity, let denial, _):
                guard appId == document.appId else { return }
                toast = denialMessage(denial, entity: entity) // #272 #273
                await refresh(entity) // 楽観的に書いた値をサーバーの内容に戻す
            case .proRequired(let appId, let entity, let pro, _):
                guard appId == document.appId else { return }
                presentProRequiredWrite(pro) // #316: 書いた分は戻した。作成者なら課金画面、参加者なら案内
                records[entity] = await recordSync.cached(appId: appId, entity: entity)
                WidgetSnapshotWriter.write(dsl: dsl, records: records)
            case .planLimit(let appId, let entity, let limit, _):
                guard appId == document.appId else { return }
                // #317: 書いた分は戻した。課金画面は自動で開かない(作成者はカードの「Pro にする」から・参加者には出さない)
                withAnimation(.easeInOut(duration: 0.3)) { errorCard = ErrorPresentation(kind: .planLimit(limit), transcript: "") }
                records[entity] = await recordSync.cached(appId: appId, entity: entity)
                WidgetSnapshotWriter.write(dsl: dsl, records: records)
            case .appStopped(let appId, let entity, _):
                guard appId == document.appId else { return }
                toast = ErrorCopy.appStoppedBody // #346
                records[entity] = await recordSync.cached(appId: appId, entity: entity)
                WidgetSnapshotWriter.write(dsl: dsl, records: records)
            case .textNotAllowed(let appId, let entity, let field, _):
                guard appId == document.appId else { return }
                toast = textNotAllowedToast(entity: entity, field: field) // #321
                records[entity] = await recordSync.cached(appId: appId, entity: entity)
                WidgetSnapshotWriter.write(dsl: dsl, records: records)
            case .ageUnconfirmed(let appId, let entity, let data, _):
                guard appId == document.appId else { return }
                records[entity] = await recordSync.cached(appId: appId, entity: entity)
                WidgetSnapshotWriter.write(dsl: dsl, records: records)
                #if !APP_STORE_REVIEW
                profilePostNeedsAge(entity: entity, data: data) // #507: 18 歳以上の確認 → 同意したら投稿し直す
                #endif
            }
        }
    }

    /// サーバーが書き込みを断った(破棄された)イベントが、最後に保存したフォームの操作そのものなら「保存しました」を取り消す。
    /// 保存は送信待ちに積んだ時点で数えるので、断られた分はここで戻す(画面を閉じた後なら toast・カードだけが残る)。
    /// 同じ entity の前に積んだ別の記録が断られても、今のフォームの表示は変えない(recordId で見分ける)。
    func revokeSaveFeedback(for event: SyncEvent) {
        guard let refused = event.refusedWrite, let recordId = refused.recordId,
              let saved = lastSavedForm, saved.appId == refused.appId, saved.entity == refused.entity,
              saved.recordId == recordId else { return }
        saved.form?.markSaveRefused(restoring: saved.payload) // 空に戻したフォームにも入力を戻す(入れ直しは消さない)
        lastSavedForm = nil
    }

    // MARK: - Navigation
    // navigate / close / pathBinding は Mac と共有(`Shared/Runtime/RuntimeNavigating.swift`)

    // MARK: - Actions

    func run(_ action: Action, _ ctx: ActionContext) async {
        if let message = action.confirmMessage {
            let button = Confirm.button(for: action.type)
            confirm = Confirm(message: message, primaryLabel: button.label, isDestructive: button.destructive) { [weak self] in await self?.perform(action, ctx) }
            return
        }
        await perform(action, ctx)
    }

    private func perform(_ action: Action, _ ctx: ActionContext) async {
        switch action.type {
        case .navigate:
            guard let screen = action.screen else { return }
            if let ref = action.prefillRef {
                // 親から子を作る(#256): 今のレコードを子フォームの ref に積み、子は新規で開く(recordId は渡さない)
                guard let parentId = ctx.currentRecordId else {
                    toast = unsavedParentToast(ctx)
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
            // フォーム・必須(空の欄に印 M10)・single の上書き先(#267)は Mac と共有
            let target = RecordActionSteps.saveTarget(action, ctx, dsl: dsl) { self.records[$0] ?? [] }
            guard case .ready(let entity, let form, let recordId) = target else {
                if case .invalid(let message) = target {
                    // フォームの無い画面の保存ボタンはアプリの作りの問題 → 「直してと頼む」を添える。入力漏れは利用者が直す
                    if ctx.form == nil { reportFixable(message, ctx: ctx, part: L10n.tr("保存")) } else { toast = message }
                }
                return
            }
            // 書き込みの手順は Mac と共有(RecordBook.save・#1141)。iOS だけの処理はフックで足す
            let appId = document.appId
            let hooks = RecordBook.SaveHooks(
                beforeNew: { [self] in
                    guard postAuthor(for: entity) != nil, !AuthorProfile.asked else { return true }
                    // 共有アプリへの初めての投稿: 一覧に出る名前を先に決めてもらう(#250)
                    authorNamePrompt = AuthorNamePrompt { [weak self] in await self?.perform(action, ctx) }
                    return false
                },
                afterSave: { [self] saved in
                    // 送った後にサーバーが断ったら「保存しました」を取り消す
                    lastSavedForm = SavedForm(appId: appId, entity: entity, recordId: saved.recordId, form: form, payload: saved.payload)
                    WidgetSnapshotWriter.write(dsl: dsl, records: records)
                    #if !APP_STORE_REVIEW
                    writeHealthFields(entity: entity, data: saved.payload, isNew: saved.isNew) // Field.health(#167)
                    #endif
                    Task { await purgeExpired(entity) } // #482 保存の後(画面遷移を待たせない)
                })
            let outcome = await RecordBook.save(
                appId: appId, entity: entity, form: form, recordId: recordId,
                recordSync: recordSync, flight: saveFlight, author: postAuthor(for: entity), hooks: hooks,
                canEdit: { self.canEdit(entity: $0, record: $1) },
                current: { self.record(entity, $0) }, store: { self.records[entity] = $0 })
            switch outcome {
            case .readOnly: notifyReadOnly(entity: entity) // 送信待ちに積まない(#249)
            // 既存の記録を何も変えずに保存: 書き込まず(「保存しました」も出さず)に画面だけ進める(M10)
            case .unchanged: leaveAfterSave(action, ctx, entity: entity, form: form, recordId: recordId, wrote: false)
            case .saved: leaveAfterSave(action, ctx, entity: entity, form: form, recordId: recordId, wrote: true)
            case .inFlight, .held: break // 連打・名前を聞いている間
            }

        case .deleteRecord:
            guard let target = RecordActionSteps.deleteTarget(action, ctx, dsl: dsl) else { return } // single の 1 件は消さない(#267)
            let (entity, id) = (target.entity, target.recordId)
            if let current = record(entity, id), !canDelete(entity: entity, record: current) {
                notifyReadOnly(entity: entity)
                return
            }
            await deleteUndoably(entity: entity, recordId: id) // M11: 数秒は送らず「元に戻す」を出す
            if ctx.recordId == nil { close(ctx.route) } // フォーム上からの削除は画面を閉じる

        case .openVoice:
            showVoiceSheet = true

        case .openUrl:
            // openUrlButton / link / webView と同じ判定
            guard let url = ExternalURLPolicy.openable(action.url) else {
                reportFixable(L10n.tr("openUrl: url は https か line/slack/discord/tel/mailto/maps のみ"), ctx: ctx, part: L10n.tr("リンク"))
                return
            }
            _ = await openExternalURL(url)

        case .resetForm:
            ctx.form?.reset()

        case .follow, .unfollow:
            await runFollow(action, ctx) // #270: Features/Follows

        case .scheduleNotification:
            guard let spec = action.notification else { return }
            await attempt {
                let next = try await services.notifications.schedule(spec, appId: document.appId)
                await refreshNotifications()
                toast = next.map { L10n.tr("通知を予約しました(\(NotificationCopy.when($0)))") } ?? L10n.tr("通知を予約できませんでした")
            }

        case .cancelNotification:
            guard let id = action.notificationId else { return }
            await services.notifications.cancel(id: id, appId: document.appId)
            await refreshNotifications()

        case .export:
            await export(action, ctx)
        case .startActivity:
            guard let spec = action.activity else { return }
            await startActivity(spec)

        case .endActivity:
            await endActivity()

        case .addToCalendar:
            addToCalendar(action, ctx)

        case .lookupBook:
            await lookupBook(action, ctx) // Features/Books

        case .unknown:
            reportFixable(L10n.tr("未対応のアクション: \(action.rawType)"), ctx: ctx)
        }
    }

    /// 保存の後: `screen` があればそこへ、無ければ閉じる。タブ / main の根なら留まり、書き込めた時だけ「保存しました」(#1719)。
    /// single の entity(#267)は 1 件を上書きし続けるので、最初の保存(recordId なし)でも新規として空に戻さない。
    private func leaveAfterSave(_ action: Action, _ ctx: ActionContext, entity: String, form: FormDraft, recordId: String?, wrote: Bool) {
        let isNewRecord = recordId == nil && dsl.entity(entity)?.single != true
        switch NavigationResolver.afterSave(navigationState, from: ctx.route, screen: action.screen, isNewRecord: isNewRecord) {
        case .navigate(let screen):
            navigate(to: screen, presentation: action.presentation, recordId: nil, from: ctx.route)
        case .close:
            close(ctx.route)
        case .stay(let resetForm):
            guard wrote else { return } // 何も変えずに保存: 何も出さない(M10)
            if resetForm { form.reset() } // 新規: 続けて入れられるよう空に戻す。既存の記録は値を残す
            toast = L10n.tr("保存しました") // VoiceOver も読む(H8)
        }
    }

    /// prefillRef の親が未保存(新規フォームの button / toolbar)。子は親の id が無いと作れないので先に保存してもらう。
    func unsavedParentToast(_ ctx: ActionContext) -> String {
        RecordActionSteps.unsavedParentMessage(ctx) // Mac と共有
    }

    private func attempt(entity: String? = nil, _ body: () async throws -> Void) async {
        do { try await body() } catch { toast = message(for: error, entity: entity) }
    }

    /// appconfig のエラー封筒を人向けに。422 `invalid_record` は `details.field` を DSL の Field 名に引き直して「どの項目か」を出す。
    /// 同期キューで保存が弾かれた時の toast。項目が分かれば「払った人」のように名前で出す(英語の field id を見せない)。
    func rejectedToast(entity: String, field: String?, message: String) -> String {
        // H9: サーバーの文はログにだけ残し、画面には決まった文を出す
        ErrorLog.record(message, context: "save \(entity)\(field.map { ".\($0)" } ?? "")")
        guard let field else { return L10n.tr("保存できませんでした。入力を確認して、もう一度お試しください") }
        if message == FieldCipher.unsavedMessage { return message } // #233 鍵がまだ同期されていない
        let name = dsl.entity(entity)?.field(field)?.name ?? field
        return L10n.tr("保存できませんでした。「\(name)」の入力を確認してください")
    }

    /// 422 `text_not_allowed`(#321)。項目が分かれば名前で出す(英語の field id は見せない)。投稿者名は「表示名」
    func textNotAllowedToast(entity: String?, field: String?) -> String {
        guard let field else { return ErrorCopy.textNotAllowed }
        if field == "authorName" { return ErrorCopy.textNotAllowed(fieldName: L10n.tr("表示名")) }
        guard let name = entity.flatMap({ dsl.entity($0)?.field(field)?.name }) else { return ErrorCopy.textNotAllowed }
        return ErrorCopy.textNotAllowed(fieldName: name)
    }

    /// 利用者に出す 1 文(H9)。OS のエラー文・サーバーの文は出さず、ログにだけ残す。
    func message(for error: Error, entity: String?) -> String {
        guard let api = error as? APIError else { return ErrorCopy.failure(L10n.tr("うまくいきませんでした"), error, context: "action") }
        if let denial = SharedAccessDenial(api) { return denial.message } // #254
        if let denial = RecordWriteDenial(api), let entity { return denialMessage(denial, entity: entity) } // #272 #273
        #if !APP_STORE_REVIEW
        if let denial = CoinDenial(api) { // #630 書き込みの枠・コイン。自分が払う時はお店 / コインの設定も開く
            CoinGate.shared.handle(denial)
            return CoinCopy.denialBody(denial)
        }
        #endif
        if let pro = ProRequired(api) { return pro.writeMessage } // #316(印を付けるなど)
        if let limit = PlanLimit(api) { return ErrorCopy.planLimitBody(limit) } // #317
        if api.isAppStopped { return ErrorCopy.appStoppedBody } // #346
        if api.isFreePlanPaused { return ErrorCopy.freePlanPausedBody(resumeAt: api.resumeAt) } // #317
        if api.isTextNotAllowed { return textNotAllowedToast(entity: entity, field: api.field) } // #321
        if api.code == "invalid_record" || (api.status == 400 && api.field != nil), let fieldId = api.field {
            let name = entity.flatMap { dsl.entity($0)?.field(fieldId)?.name } ?? fieldId
            ErrorLog.record(error, context: "action \(entity ?? "-").\(fieldId)")
            return L10n.tr("「\(name)」の入力を確認してください")
        }
        if api.isAppCheckFailure { return ErrorCopy.appCheckFailedBody } // #318: サインインでは直らない
        if api.isAIPaused { return ErrorCopy.aiPausedBody } // #511
        if api.status == 401 { return ErrorCopy.sessionExpiredSignIn }
        return ErrorCopy.failure(L10n.tr("うまくいきませんでした"), error, context: "action")
    }
}
