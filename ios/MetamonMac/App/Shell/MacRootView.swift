import SwiftUI
import AppKit
import MetamonDSL
import MetamonRenderer
import MetamonServices

/// Mac の画面の骨格(#933): 左にアプリの一覧・中央に選んだアプリ・上にツールバー。
/// iOS で sheet だった本体の画面の出し方:
/// - アプリの切り替え(AppSwitcherSheet) → サイドバー(常に出す・閉じられる)
/// - 音声 / 文字で頼む(VoiceFab) → ツールバーのボタン + ショートカット。生成したアプリの中の FAB は残す
///   文字で頼む(⌘K)・新しいアプリ(⌘N)は会話のシート(#967・ComposeSheet)。作ったアプリは一覧の先頭に足し、直したアプリは差し替える
///   声で頼む(⇧⌘M・#966)は同じシートを開いてすぐ録音する。文字にした文をそのまま送る
/// - 設定 → Mac の設定ウインドウ(⌘,)
/// - 完成の後のお知らせ(要約・「元に戻す」・最初のアプリの後の直し方の例 #1278)→ 中央の列の上に 1 つずつ(#968・順番は iOS と同じ `PostBuildNotice`)
///   その後に新着(#268)・上限に近い(#317)。サービスのお知らせ(#333)は列の上の段に閉じるまで出す(#1058)
/// - 生成したアプリの版の「元に戻す / やり直す」(#968)→「アプリ」メニュー(⌥⌘Z / ⇧⌥⌘Z)・完成の画面・要約のお知らせ。
///   ⌘Z / ⇧⌘Z は入力欄と記録の削除の取り消し(編集メニュー)のまま
/// キーボード(#934): ⌘1 / ⌘2 でサイドバーと中央を行き来・一覧は ↑↓ で選び Return で中央へ・⌥⌘↑↓ で前 / 次のアプリ。
/// 画面マップ(#970): ツールバー・表示メニュー(⇧⌘E)からシートで出す。開く → 中央でその画面へ、話す → その画面を文脈にした会話(⌘K と同じシート)。
/// 一覧(#969): 「作ったアプリ」(端末に残す・名前の変更 / 削除 / 並べ替え)と「同梱の例」に分ける。起動時は前に開いていたアプリを開き直す。
/// 会話で直した同梱の例も端末に残し、右クリックで元の例に戻せる(#1283)。
/// 行の写し(#1281): 中央に出したアプリを、出てから少し後(お知らせが閉じてから)に撮ってサイドバーの行に出す。
/// 検索(#1282): サイドバーの上の検索欄(⌘F で入る)。名前と説明で両方の区分を絞り込む。
struct MacRootView: View {
    enum Pane: Hashable { case sidebar, canvas }

    static let minWindowSize = CGSize(width: 720, height: 480)

    let bundledApps: [SampleApp]
    @SceneStorage("selectedAppId") private var selectedAppId: String?
    @State private var columnVisibility = NavigationSplitViewVisibility.all
    @FocusState private var focus: Pane?
    /// 会話で作った・直したアプリの DSL と版の履歴(#967・#968)・作ったアプリの一覧(#969)
    @Environment(MacAppLibrary.self) private var library
    /// 初回の案内(#971)。「直し方の例」を閉じたら β の案内を出す(#1215)。プレビューなど案内の無い所では nil
    @Environment(MacOnboarding.self) private var onboarding: MacOnboarding?
    /// β の案内の依頼の例(#1214)など、ほかのシート・ウインドウから入力欄に入れる文
    private let draftRequest = MacDraftRequest.shared
    /// ウインドウを開く(サービスメニューで文字が届いた時・#1849 / 生成アプリごとのウインドウ・#1853)
    @Environment(\.openWindow) private var openWindow
    /// 完成の後のお知らせ(#968)
    @State private var notices = MacPostBuildNotices()
    /// 最低対応版と強制アップデート(#981)。サービスのお知らせと同じ応答で判定する
    @State private var updateGate = MacUpdateGate()
    /// 前面に戻った回数。新着・上限のお知らせを数え直す(#1058)
    @State private var noticeCheck = 0
    @State private var compose: MacComposeSession?
    /// 閉じた会話のシート(閉じた後に完成の要約を出すため・#968)
    @State private var closingCompose: MacComposeSession?
    @State private var showsTips = false
    /// 中央のアプリの設定(記録の自動削除・自動の動き・#979)。設定ウインドウはアプリに属さないので、アプリごとの設定はこのシートに置く
    @State private var showsAppSettings = false
    /// サイドバーの行の写し(#1281)
    @State private var thumbs = MacAppThumbnailStore.shared
    @State private var thumbAnchor = MacThumbnailAnchor()
    /// サイドバーの検索(#1282)
    @State private var searchText = ""
    @FocusState private var searchFocused: Bool
    /// 画面マップ(#970・シートは中央の AppCanvas が出す)
    @State private var showsScreenMap = false
    /// 保存していない入力を、アプリの切り替え・ウインドウを閉じる前に確認する(#943)
    @State private var discardGuard = MacDiscardGuard()
    /// シートの間に押された通知の切り替え先。シートを閉じてから確認を通して切り替える(#943)
    @State private var notificationSwitch = SheetDeferredSwitch()
    /// 複製で持てるアプリの数に達した時の置き換えシート(#630)
    @State private var appLimit: MacAppLimitRequest?
    /// 会話で直した直後に光らせる部品(#626・#1337)。iOS と同じく出現が済んだ頃(2 秒)に消す
    @State private var highlight: (canvas: String, value: ChangeHighlight)?
    @Environment(MacVoiceInput.self) private var voice
    #if !APP_STORE_REVIEW
    /// 共有のリンクから取り込んだアプリ(#1287)を一覧に足して開く
    @State private var shareFlow = MacShareFlow.shared
    /// ツールバーの「共有」(#1035)のシート
    @State private var shareApp: MacShareAppModel?
    #endif
    @Environment(\.accessibilityReduceMotion) private var reduceMotion
    /// サイドバーの新着の数(#969)
    @State private var badges = MacSidebarBadges.shared
    /// このウインドウの印(新着の数で「どのウインドウで開いているか」を分ける)
    @State private var windowToken = UUID()
    /// 生成アプリごとのウインドウ(#1853)で開いたアプリ。メインのウインドウでは nil
    private let windowApp: AppID?

    init(apps: [SampleApp], window: AppID? = nil) {
        bundledApps = apps
        windowApp = window
        // アプリのウインドウはサイドバーを畳んで開く(⌃⌘S・ツールバーで出せる)
        _columnVisibility = State(initialValue: window == nil ? .all : .detailOnly)
    }

    /// 作ったアプリ(一覧の並び)。サーバーのアプリで中身をまだ取っていない行は、一覧の名前で出す(#1057)
    private var createdApps: [SampleApp] {
        library.createdIds.compactMap { id in
            if let document = library.generated[id] { return Self.entry(id: id, document) }
            return library.serverSummary(id).map { SampleApp(id: id, name: $0.name, description: nil, url: nil, version: $0.currentVersion) }
        }
    }

    /// 同梱の例(直したものは差し替え)
    private var exampleApps: [SampleApp] {
        bundledApps.map { app in library.generated[app.id].map { Self.entry(id: app.id, $0) } ?? app }
    }

    /// 一覧に出す順: 作ったアプリ → 同梱の例
    private var apps: [SampleApp] { createdApps + exampleApps }

    private static func entry(id: String, _ document: AppDocument) -> SampleApp {
        SampleApp(id: id, name: document.dsl.app.name, description: document.dsl.app.description, url: nil, version: document.version)
    }

    private var selectedApp: SampleApp? { apps.first { $0.id == selectedAppId } }

    var body: some View {
        NavigationSplitView(columnVisibility: $columnVisibility) {
            AppSidebar(created: sidebarSections.own.filter { $0.matches(searchText) },
                       shared: sidebarSections.shared.filter { $0.matches(searchText) },
                       examples: exampleApps.filter { $0.matches(searchText) },
                       newItems: { badges.count($0) },
                       thumbnail: thumbnail(for:), searchText: searchText, selection: guardedSelection,
                       onOpen: { focus = .canvas }, onDelete: deleteApp, onDuplicate: duplicateApp,
                       onResetExample: resetExample, onOpenInNewWindow: openAppWindow,
                       loadingServerApps: library.serverListState == .loading,
                       offline: library.serverListState == .offline)
                .focused($focus, equals: .sidebar)
                .navigationSplitViewColumnWidth(min: 180, ideal: 220, max: 320)
        } detail: {
            Group {
                if let app = selectedApp, library.isServerOnly(app.id) {
                    MacServerAppLoading(app: app, failed: library.downloadFailed.contains(app.id)) {
                        Task { await library.download(app.id) }
                    }
                    .task(id: app.id) { await library.download(app.id) }
                } else if let app = selectedApp {
                    AppCanvas(app: app, dsl: library.generated[app.id]?.dsl, keepsRecords: library.isCreated(app.id),
                              server: library.isCreated(app.id) && library.isOnServer(app.id) ? MacServices.shared.appConfig : nil,
                              showsScreenMap: $showsScreenMap,
                              onTalk: { talk(about: $0) }, onOpenVoice: { speak() }, discardGuard: discardGuard)
                        .environment(\.changeHighlight, highlight?.canvas == Self.canvasKey(app) ? highlight?.value ?? .none : .none)
                        .id(Self.canvasKey(app))
                        .background { MacThumbnailAnchorView(anchor: thumbAnchor) }
                        .task(id: Self.canvasKey(app)) {
                            // 描き終わった頃に撮る(iOS は版が変わった 1.2 秒後)
                            try? await Task.sleep(for: .seconds(1.2))
                            guard !Task.isCancelled else { return }
                            captureThumbnail()
                        }
                } else {
                    MacEmptyState(pose: .point, title: "アプリを選んでください",
                                  message: "左の一覧から選ぶと、ここに表示します。")
                }
            }
            .frame(minWidth: 480, minHeight: 400)
            .overlay(alignment: .top) { noticeBar }
            .safeAreaInset(edge: .top, spacing: 0) { serviceNoticeBar }
            .task(id: appNoticeKey) { await checkAppNotices() }
            .task(id: badgeKey) { await refreshBadges() }
            .task(id: noticeCheck) { await checkServiceNotice() }
            // 中央の列をフォーカスの行き先にする(Tab で中の入力欄・ボタンへ進む)
            .focusable()
            .focusEffectDisabled()
            .focused($focus, equals: .canvas)
            // 列全体の既定の輪は消し、キーボードで来た時だけ内側に輪を描く(フルキーボードアクセス・#937)
            .macPaneFocusRing(focus == .canvas)
        }
        .searchable(text: $searchText, placement: .sidebar, prompt: Text("アプリを探す"))
        .modifier(SidebarSearchFocus(focused: $searchFocused))
        .toolbar {
            ShellToolbar(hasApp: selectedApp != nil, canCompose: canCompose, newApp: newApp, ask: ask, speak: speak,
                         canShowScreenMap: canShowScreenMap, showScreenMap: showScreenMap,
                         canShare: canShare, share: openShare,
                         canShowAppSettings: canShowScreenMap, showAppSettings: { showsAppSettings = true })
        }
        .frame(minWidth: Self.minWindowSize.width, minHeight: Self.minWindowSize.height)
        .focusedSceneValue(\.shellActions, shellActions)
        .macNotificationRouting(selection: notificationSelection, appIds: apps.map(\.id)) // 押された通知のアプリを開く(#965)。入力中なら確認を通す・シートの間は閉じるまで待つ(#943)
        .macIntentRouting(selection: notificationSelection, appIds: apps.map(\.id), talk: speak) // ショートカット・Siri の「開く / 話しかける」(#1034)
        .onChange(of: apps.map(\.name)) { IntentHost.shared.appsMayHaveChanged() } // Siri のフレーズのアプリ名を引き直す(#1034)
        .background { WindowCloseGuard(discardGuard: discardGuard) } // ⌘W・閉じるボタン(#943)
        .background { if let windowApp { WindowFrameAutosave(name: MacAppWindows.frameAutosaveName(windowApp)) } } // #1853
        .modifier(MacUpdateGateModifier(gate: updateGate)) // 古い版は閉じられない更新のシート(#981)
        #if !APP_STORE_REVIEW
        .onChange(of: shareFlow.adopted?.id) { adoptShared() }
        // フィードバックに添える今のアプリ(#1330)。サーバーにあるアプリの時だけ(選択・版・保存先が変わったら入れ替える)
        .onChange(of: feedbackAppKey, initial: true) { MacFeedbackCenter.shared.currentApp = feedbackApp }
        #endif
        #if !APP_STORE_REVIEW
        .sheet(item: $shareApp) { model in
            MacShareAppSheet(model: model, onClose: { shareApp = nil })
        }
        #endif
        .sheet(item: $compose, onDismiss: noteFinishedCompose) { session in
            ComposeSheet(session: session, onClose: { compose = nil }, undo: finishedUndo(session))
                .interactiveDismissDisabled(session.isBusy)
                .environment(voice)
                .environment(\.macAppReplace, appReplaceContext) // 持てるアプリの数の上限の置き換え(#630)
        }
        #if !APP_STORE_REVIEW
        .modifier(MacPaywallPresenter()) // 中央のアプリの部品が Pro を求めた時の課金画面(iOS と同じ商品)
        #endif
        .task { voice.prepareIfAllowed() } // モデルが端末にあれば読み込む(無ければ起動では聞かない)
        // サインインした・アカウントが変わった時と、前面に戻った時にサーバーの一覧を取り直す(#1057)
        .task(id: MacAccount.shared.session.user?.uid) { await refreshServerApps() }
        .onReceive(NotificationCenter.default.publisher(for: NSApplication.didBecomeActiveNotification)) { _ in
            Task { await refreshServerApps() }
            noticeCheck += 1
        }
        .onDisappear {
            MacSpeechReader.shared.stop() // ウインドウを閉じたら読み上げを止める(#1210)
            badges.closeWindow(windowToken)
        }
        .onAppear {
            reopenLastApp()
            // サービスメニュー(#1849): ウインドウが無い時に届いた文字は、開いたこのウインドウが受け取る
            MacTextServiceProvider.shared.openMainWindow = { [openWindow] in openWindow(id: MacCommands.mainWindowId) }
            takeDraftRequest()
            badges.setOpen(selectedAppId, window: windowToken)
            // self ごと捕まえると見張り(State)との循環で残るので、シートの Binding だけ渡す
            let map = $showsScreenMap, tips = $showsTips
            discardGuard.beforeAsking = { map.wrappedValue = false; tips.wrappedValue = false }
        }
        .onChange(of: notices.isShowing) { _, showing in
            guard !showing else { return }
            // お知らせ(要約・直し方の例)が写り込まないよう、全部閉じて消え終わってから撮り直す
            Task { @MainActor in
                try? await Task.sleep(for: .seconds(0.4))
                captureThumbnail()
            }
        }
        .onChange(of: library.wipeCount) { // サインアウト・アカウント削除(#972): 作ったアプリを開いていたら初めの状態に戻す
            compose?.close() // 走っている返事・生成が消した履歴を書き戻さないように止める
            compose = nil
            #if !APP_STORE_REVIEW
            shareApp?.close() // 読み込み中の参加者を書き戻さない
            shareApp = nil
            #endif
            notices.resetAfterWipe()
            reopenLastApp()
        }
        .onChange(of: selectedAppId) { _, id in
            if let id, windowApp == nil { library.lastAppId = id } // 次の起動で開き直すのはメインのウインドウの選択だけ
            badges.setOpen(id, window: windowToken) // 開いたら新着の印を消す(#969)。どのウインドウで開いていても数えない
            showsScreenMap = false
        }
        .sheet(item: $appLimit) { request in
            MacAppLimitSheet(limit: request.limit, context: appReplaceContext, onFreed: request.retry)
        }
        .onChange(of: draftRequest.serial) {
            // 頼まれた文を入力欄に入れて会話を開く(送らない)。頼んだシートが閉じるのを待つ(重なると表示されない・iOS と同じ 0.6 秒)
            takeDraftRequest()
        }
        #if !APP_STORE_REVIEW
        .onChange(of: notices.showsEditHint) { wasShowing, showing in
            // 最初のアプリの後の「直し方の例」が閉じたら β の案内を出す(iOS の `BetaWelcomeHost` と同じ順・#1215)
            guard wasShowing, !showing else { return }
            Task { @MainActor in
                try? await Task.sleep(for: .milliseconds(700))
                onboarding?.presentBetaWelcomeIfNeeded()
            }
        }
        #endif
        .sheet(isPresented: $showsAppSettings, onDismiss: switchAfterSheet) {
            if let app = selectedApp, let dsl = library.generated[app.id]?.dsl ?? (try? app.loadDSL()) {
                MacAppSettingsSheet(appId: app.id, appName: app.name, dsl: dsl,
                                    keepsRecords: library.isCreated(app.id),
                                    isOnServer: library.isCreated(app.id) && library.isOnServer(app.id),
                                    onConfirmRetention: { notifyRetentionConfirmed(app.id) },
                                    onRequest: { text, send in
                                        showsAppSettings = false
                                        Task { @MainActor in
                                            try? await Task.sleep(for: .milliseconds(600)) // シートが閉じてから(重ねない)
                                            if send { sendFix(text) } else { ask(draft: text) }
                                        }
                                    },
                                    onClose: { showsAppSettings = false })
            }
        }
        .sheet(isPresented: $showsTips, onDismiss: switchAfterSheet) {
            PromptTipsSheet { tip in
                showsTips = false
                ask(draft: tip)
            }
        }
    }

    // MARK: - 一覧(#969)

    /// 作ったアプリを「作ったアプリ」と「共有されたアプリ」に分ける(並びは一覧のまま)
    private var sidebarSections: (own: [SampleApp], shared: [SampleApp]) {
        let origins = MacAppOrigins()
        let split = MacSidebarSections.split(library.createdIds, isShared: origins.isShared)
        let byId = Dictionary(createdApps.map { ($0.id, $0) }, uniquingKeysWith: { a, _ in a })
        return (split.own.compactMap { byId[$0] }, split.shared.compactMap { byId[$0] })
    }

    /// 新着の数を数え直すきっかけ: アカウント・一覧(共有の記録があるアプリ)・前面に戻った。
    /// 実際に問い合わせるかは `MacSidebarBadges.refreshIfDue` が決める(全ウインドウで 1 本・数分に 1 回)
    private var badgeKey: String {
        let ids = badgeTargets.map(\.appId)
        return "\(MacAccount.shared.session.user?.uid ?? "")|\(ids.joined(separator: ","))|\(noticeCheck)"
    }

    /// 数える相手: サーバーにあり中身を取ったアプリの、共有の entity だけ
    private var badgeTargets: [MacSidebarBadges.Target] {
        library.createdIds.compactMap { id in
            guard library.isOnServer(id), let document = library.generated[id] else { return nil }
            return MacSidebarBadges.target(appId: id, entities: document.dsl.entities)
        }
    }

    /// サイドバーの新着の数(#969)
    private func refreshBadges() async {
        await badges.refreshIfDue(badgeTargets, uid: MacAccount.shared.session.user?.uid,
                                  source: MacServices.shared.appConfig as? SharedRecordStampSource)
    }

    /// サーバーの一覧を取り直す(#1057)。サインインしていなければ取らない(サインアウトの後は wipe が空にしている)
    private func refreshServerApps() async {
        guard MacAccount.shared.session.user != nil else { return }
        await library.refreshServerApps()
        reopenLastApp() // 前に開いていたサーバーのアプリが一覧に載ったら開き直す
        // 待っていた通知のアプリを開く。取り直しても無ければ諦める
        if let id = MacNotificationRouter.shared.resolve(among: apps.map(\.id), listRefreshed: true) {
            notificationSelection.wrappedValue = id
        }
    }

    /// 起動時: ウインドウが覚えている選択(SceneStorage)が無い・もう無いアプリなら、前に開いていたアプリを開き直す
    private func reopenLastApp() {
        let ids = apps.map(\.id)
        if let id = selectedAppId, ids.contains(id) { return }
        if windowApp != nil { // アプリのウインドウ(#1853)はそのアプリを開く。まだ一覧に無ければ載るまで待つ
            selectedAppId = MacAppWindows.initialSelection(window: windowApp, restored: selectedAppId, appIds: ids)
            return
        }
        selectedAppId = LocalAppLibrary.reopen(library.lastAppId, among: ids)
    }

    /// 置き換えシートに並べるアプリ(この Mac で作ったアプリ)と消し方(#630)。版が 1 の空のアプリとして数えないよう 2 以上にする
    private var appReplaceContext: MacAppReplaceContext {
        let summaries = library.createdIds.compactMap { id in
            library.generated[id].map { AppSummary(id: id, name: $0.dsl.app.name, currentVersion: max($0.version, 2)) }
                ?? library.serverSummary(id).map { AppSummary(id: id, name: $0.name, currentVersion: max($0.currentVersion, 2)) }
        }
        return MacAppReplaceContext(apps: summaries, current: selectedAppId, delete: deleteApp)
    }

    /// 作ったアプリを消す(確認はサイドバーで済んでいる)。開いていたら一覧で隣のアプリへ移る
    private func deleteApp(_ id: String) {
        let name = library.generated[id]?.dsl.app.name ?? ""
        if selectedAppId == id { selectedAppId = LocalAppLibrary.neighbor(deleting: id, in: apps.map(\.id)) }
        if notices.summary?.appId == id { notices.summary = nil }
        let onServer = library.isOnServer(id)
        library.delete(id)
        MacAppOrigins().remove(id)
        badges.markRead(id)
        // 記録も消す(#1284)。開いているウインドウのエンジンを止めて手放してから消す(動いたままだと書き戻される)
        Task { await MacRecordEngines.shared.purge(appId: id) }
        // サーバーにあるアプリはサーバーからも消す(iOS の deleteApp と同じ)。届かなくても端末からは消す
        if onServer, let appConfig = MacServices.shared.appConfig {
            Task {
                do { try await appConfig.deleteApp(appId: id) } catch { print("[Metamon] Mac: deleteApp failed: \(error)") }
                await refreshServerApps()
            }
        }
        thumbs.remove(appId: id)
        Task { await MacRuntimeNotifications.removeApp(id) } // そのアプリの通知とタイマーの予約(#1324)
        MacVoiceOver.announce(.appDeleted(name: name))
    }

    #if !APP_STORE_REVIEW
    private var feedbackAppKey: String {
        guard let id = selectedAppId else { return "" }
        return "\(id)#\(library.generated[id]?.version ?? 0)#\(library.isOnServer(id))"
    }

    private var feedbackApp: MacFeedbackContext.CurrentApp? {
        guard let id = selectedAppId, library.isOnServer(id), let document = library.generated[id] else { return nil }
        return .init(document: document, envelope: library.envelope(for: id))
    }

    /// 共有のリンクから取り込んだ・参加したアプリ(#1287)。サーバーにあるアプリとして一覧に足し、入力の確認を通して開く
    private func adoptShared() {
        guard let adopted = shareFlow.adopted else { return }
        shareFlow.adopted = nil
        let id = adopted.document.appId
        MacAppOrigins().markShared(id) // サイドバーの「共有されたアプリ」(#969)
        library.apply(adopted.document, envelope: adopted.envelope, created: true, onServer: true)
        switchApp(to: id)
        notices.summary = .init(appId: id, text: adopted.banner, action: nil)
    }
    #endif

    /// 複製する(#1285)。iOS と同じく切り替えずに知らせる。
    /// 持てるアプリの数に達していれば、作る時と同じ置き換えシートを出し、1 つ空いたら複製し直す(#630・iOS の `duplicateApp`)
    private func duplicateApp(_ id: String) {
        if let limit = library.limitForAdding(planApps: MacPlanApps.current) {
            appLimit = MacAppLimitRequest(limit: limit) { duplicateApp(id) }
            return
        }
        guard let name = library.duplicate(id, bundled: bundledApps.first { $0.id == id }) else { return }
        MacVoiceOver.announce(.appCreated(name: name))
    }

    /// 会話で直した同梱の例を元に戻す(#1283)。開いていれば中央を作り直すので、入力があれば先に確認する(#943)
    private func resetExample(_ id: String) {
        let name = bundledApps.first { $0.id == id }?.name ?? ""
        discardGuard.run {
            if notices.summary?.appId == id { notices.summary = nil }
            library.resetExample(id)
            thumbs.remove(appId: id) // 写しも撮り直す
            MacVoiceOver.announce(.appRestored(name: name))
        }
    }

    // MARK: - 行の写し(#1281)

    private func thumbnail(for appId: String) -> NSImage? {
        _ = thumbs.revision // 撮り直しで描き直す
        return thumbs.image(for: appId)
    }

    /// 中央のアプリを撮る。お知らせ・会話のシートが出ている間は撮らない(閉じた後に撮り直す)
    private func captureThumbnail() {
        guard let app = selectedApp, !notices.isShowing, compose == nil else { return }
        let voiceOnly = library.generated[app.id]?.dsl.isVoiceOnly ?? false
        thumbs.capture(appId: app.id, isVoiceOnly: voiceOnly, anchor: thumbAnchor)
    }

    // MARK: - 作る・直す(#967)

    /// ⌘N: 新しいアプリを作る会話
    private func newApp() { open(.create, draft: "") }

    /// ⌘K: 文字で頼む。中央にアプリがあればそのアプリを直す、無ければ作る
    private func ask() { ask(draft: "") }

    /// ⇧⌘M: 声で頼む。⌘K と同じ相手(中央のアプリ・無ければ新しいアプリ)の会話を開き、すぐ録音する
    private func speak() { ask(draft: "", voice: true) }

    /// 頼まれた文(β の案内・サービスメニュー)を受け取る。頼んだ時に前に出したウインドウだけ
    private func takeDraftRequest() {
        guard let draft = draftRequest.take(in: discardGuard.window) else { return }
        Task { @MainActor in
            try? await Task.sleep(for: .milliseconds(600))
            if let compose {
                // 会話が開いていれば、その入力欄に足す(書きかけは消さない・#1849)
                compose.draft = MacTextServiceProvider.appending(draft, to: compose.draft)
            } else {
                ask(draft: draft)
            }
        }
    }

    private func ask(draft: String, voice: Bool = false) {
        if let app = selectedApp {
            open(.edit(appId: app.id, name: app.name, version: app.version), draft: draft, voice: voice)
        } else {
            open(.create, draft: draft, voice: voice)
        }
    }

    /// 公開の確認・保存期間の「やめる」の直す指示を、中央のアプリを直す会話で送る(iOS の `generate(text)` と同じく相談を挟まず作る)
    private func sendFix(_ instruction: String) {
        guard let app = selectedApp else { return }
        open(.edit(appId: app.id, name: app.name, version: app.version), draft: "", build: instruction)
    }

    /// 保存期間を確認した: 開いているランタイムが期限切れを消す(#482)
    private func notifyRetentionConfirmed(_ appId: String) {
        NotificationCenter.default.post(name: MacAppRuntime.retentionConfirmed, object: appId)
    }

    private func open(_ target: MacComposeSession.Target, draft: String, voice: Bool = false, screen: ScreenContext? = nil,
                      build: String? = nil) {
        guard canCompose else { return }
        // 会話の結果は中央のアプリを作り直す(直す・作る どちらも)。入力があれば会話を始める前に確認する(#943)。
        // シートの間は中央に触れないので、ここで「破棄」と答えたら閉じるまで聞き直さない(確認をシートの裏に出さない)
        discardGuard.run {
            let session = MacComposeSession(target: target, draft: draft)
            session.startsWithVoice = voice
            session.screen = screen
            session.onResult = { document, envelope in apply(document, envelope: envelope, target: target) }
            // 競合(409・#1279): サーバーにあるアプリは最新を取り直して当てる(iOS と同じ)。直す会話は取り直した版で送り直す
            if case .edit(let appId, _, _) = target, library.isOnServer(appId) {
                session.onConflict = { await library.refreshFromServer(appId) }
            }
            notices.summary = nil // 会話を始めたら前の要約は下げる
            MacSpeechReader.shared.stop() // 読み上げも止める(#1210)
            closingCompose = session
            compose = session
            discardGuard.released = true
            if let build { session.build(build) }
        }
    }

    /// 結果を中央に当てる。作ったアプリは一覧の先頭に足して選ぶ・直したアプリはその場で差し替える(中央を作り直す)
    private func apply(_ document: AppDocument, envelope: Data?, target: MacComposeSession.Target) {
        let previous = target == .create ? nil
            : library.generated[document.appId]?.dsl ?? (try? apps.first(where: { $0.id == document.appId })?.loadDSL())
        // 本物の AI の結果はサーバーにも保存されている(記録・版をサーバーにつなぐ印)
        library.apply(document, envelope: envelope, created: target == .create, onServer: MacServices.shared.isLive)
        if target == .create, MacServices.shared.isLive { Task { await refreshServerApps() } } // サーバーの並びに入れる(#1057)
        showChangeHighlight(previous: previous, next: document)
        // 最初のアプリの印(iOS と同じ `hasShownEditHint`)。β の案内(`MacOnboarding`)は「直し方の例」を閉じた後
        // (出さなかった時は次の起動)にこれを見て 1 回出す。
        // 最初の 1 回だけ「こう頼むと直せます」を予約する(要約が閉じてから出る・#1278)
        if target == .create {
            let tutorial = FirstRunTutorialStore.shared
            if tutorial.shouldShowEditHint(createdFromInitial: true), !document.dsl.isVoiceOnly { notices.showsEditHint = true }
            tutorial.markEditHintShown()
        }
        selectedAppId = document.appId
    }

    /// 中央のアプリを作り直す印(版が変わったら作り直す)
    private static func canvasKey(_ app: SampleApp) -> String { "\(app.id)#\(app.version)" }

    /// 直した結果で変わった部品を光らせる(iOS の `buildStagePlan` の edit と同じ判定)。作った・元に戻した時は光らせない
    private func showChangeHighlight(previous: AppDSL?, next: AppDocument) {
        guard let previous, !previous.isVoiceOnly else {
            highlight = nil
            return
        }
        let value = BuildChoreography.editHighlight(old: previous, new: next.dsl)
        let canvas = "\(next.appId)#\(next.version)"
        highlight = value.isEmpty ? nil : (canvas, value)
        guard !value.isEmpty else { return }
        Task { @MainActor in
            try? await Task.sleep(for: .seconds(2))
            if highlight?.canvas == canvas, highlight?.value == value { highlight = nil } // 後でタブを開いた時に光らせ直さない
        }
    }

    // MARK: - 画面マップ(#970)

    /// 中央にアプリがあり、会話のシートが閉じている時だけ開ける
    private var canShowScreenMap: Bool { selectedApp != nil && compose == nil }

    /// ツールバーの「共有」(#1035)を押せるか。サーバーに保存した自分のアプリだけ
    private var canShare: Bool {
        #if APP_STORE_REVIEW
        false
        #else
        guard let app = selectedApp, compose == nil, !updateGate.isBlocking else { return false }
        return MacShareAppModel.canShare(isCreated: library.isCreated(app.id), isOnServer: library.isOnServer(app.id),
                                         signedIn: MacAccount.shared.session.user != nil,
                                         sharingAvailable: MacServices.shared.sharing != nil)
        #endif
    }

    private func openShare() {
        #if !APP_STORE_REVIEW
        guard canShare, let app = selectedApp else { return }
        shareApp = MacShareAppModel(appId: app.id, appName: app.name, sharing: MacServices.shared.sharing,
                                    membership: MacSocial.shared.members, social: .shared)
        #endif
    }

    /// 会話のシートを開ける・版を戻せるか。画面マップのシートが出ている間は開かない(シートを重ねない)
    private var canCompose: Bool { compose == nil && !showsScreenMap && !updateGate.isBlocking }

    private func showScreenMap() {
        guard canShowScreenMap else { return }
        showsScreenMap = true
    }

    /// マップの「この画面について話す」: 中央のアプリを直す会話を、その画面を文脈にして開く
    private func talk(about screen: ScreenContext) {
        guard let app = selectedApp else { return }
        open(.edit(appId: app.id, name: app.name, version: app.version), draft: "", screen: screen)
    }

    // MARK: - 完成の後のお知らせ・版の取り消し(#968)

    /// シートが閉じた。完成していたら要約のお知らせを出す(iOS の要約バナーと同じく、直した時は「元に戻す」付き)
    private func noteFinishedCompose() {
        discardGuard.released = false
        defer {
            closingCompose = nil
            switchAfterSheet()
        }
        guard let session = closingCompose, case .finished(let summary, false) = session.phase,
              let appId = session.resultAppId else { return }
        // 完成の読み上げは会話のシート(`.built`)で済んでいる
        notices.summary = .init(appId: appId, text: summary, action: library.canUndo(appId) ? .undo : nil, announced: true)
        if let data = session.resultPublicData { // 公開の確認(#253)は要約が閉じてから
            notices.notePublicData(data.fields, appId: appId, version: data.version)
        }
        MacSpeechReader.shared.speak(summary) // #1210(設定 OFF・VoiceOver の時は喋らない)
    }

    /// 完成の画面の「元に戻す」。戻せる版がある時だけ
    private func finishedUndo(_ session: MacComposeSession) -> (() -> Void)? {
        guard let appId = session.resultAppId, library.canUndo(appId) else { return nil }
        return {
            closingCompose = nil // 閉じた後に「できあがりました」を出さない
            compose = nil
            undoVersion(appId)
        }
    }

    /// 版を戻す・やり直すと中央のアプリを作り直す。入力があれば先に確認する(#943)。
    /// サーバーにあるアプリはサーバーの版で戻す(#1279・届かなければ端末の履歴)
    private func undoVersion(_ appId: String? = nil) {
        guard let id = appId ?? selectedAppId, library.canUndo(id) else { return }
        discardGuard.run {
            Task {
                switch await library.undo(id) {
                case .moved:
                    selectedAppId = id
                    notices.summary = .init(appId: id, text: L10n.tr("元に戻しました"), action: .redo)
                case .nothing, .failed:
                    notices.summary = .init(appId: id, text: L10n.tr("これ以上前の状態はありません"), action: nil)
                }
            }
        }
    }

    private func redoVersion(_ appId: String? = nil) {
        guard let id = appId ?? selectedAppId, library.canRedo(id) else { return }
        discardGuard.run {
            Task {
                switch await library.redo(id) {
                case .moved:
                    selectedAppId = id
                    notices.summary = .init(appId: id, text: L10n.tr("やり直しました"), action: library.canUndo(id) ? .undo : nil)
                case .nothing, .failed:
                    notices.summary = .init(appId: id, text: L10n.tr("やり直せませんでした"), action: nil)
                }
            }
        }
    }

    @ViewBuilder private var noticeBar: some View {
        VStack {
            if compose == nil, notices.active == .summary, let summary = notices.summary {
                MacSummaryNoticeBar(summary: summary,
                                    canUndo: library.canUndo(summary.appId),
                                    canRedo: library.canRedo(summary.appId),
                                    undo: { undoVersion(summary.appId) },
                                    redo: { redoVersion(summary.appId) },
                                    dismiss: { withMacAnimation { notices.dismissActive() } })
                    .task(id: summary.id) {
                        if !summary.announced { MacVoiceOver.announce(summary.text) }
                        try? await Task.sleep(for: .seconds(MacPostBuildNotices.holdSeconds(summary)))
                        guard !Task.isCancelled, notices.summary == summary else { return }
                        withMacAnimation { notices.dismissActive() }
                    }
            } else if compose == nil, notices.active == .publicData, let publicData = notices.publicData {
                MacPublicDataBar(notice: publicData) { choice in
                    let text = withMacAnimation {
                        notices.resolvePublicData(choice, currentAppId: selectedApp?.id,
                                                  currentVersion: library.generated[publicData.appId]?.version)
                    }
                    if let text { sendFix(text) }
                }
                .task(id: publicData.id) { MacVoiceOver.announce(L10n.tr("個人の情報も公開されます")) }
            } else if compose == nil, notices.active == .retention, let retention = notices.retention {
                MacRetentionBar(prompt: retention,
                                confirm: {
                                    if let appId = withMacAnimation(.default, { notices.confirmRetention() }) { notifyRetentionConfirmed(appId) }
                                },
                                stop: {
                                    if let text = withMacAnimation(.default, { notices.stopRetention() }) { sendFix(text) }
                                },
                                later: { withMacAnimation { notices.deferRetention() } })
                    .task(id: retention.id) { MacVoiceOver.announce(L10n.tr("古い記録を自動で消しますか?")) }
            } else if compose == nil, notices.active == .editHint {
                MacEditHintBar(ask: {
                                   withMacAnimation { notices.dismissActive() }
                                   ask()
                               },
                               dismiss: { withMacAnimation { notices.dismissActive() } })
                    .task { MacVoiceOver.announce(L10n.tr("こう頼むと直せます")) }
            } else if compose == nil, notices.top == .newItems, let newItems = notices.newItems {
                MacNewItemsBar(notice: newItems, dismiss: { withMacAnimation { notices.dismissActive() } })
                    .task(id: newItems.id) {
                        // VoiceOver へは 1 回だけ(読み上げの声 #1210 は要約だけ・重ねない)
                        MacVoiceOver.announce(L10n.tr("新着 \(newItems.count) 件"))
                        try? await Task.sleep(for: .seconds(MacVoiceOver.holdSeconds(6)))
                        guard !Task.isCancelled, notices.newItems == newItems else { return }
                        withMacAnimation { notices.dismissActive() }
                    }
            } else if compose == nil, notices.top == .usage, let usage = notices.usage {
                MacUsageNoticeBar(notice: usage, upgrade: usageUpgrade(usage),
                                  dismiss: { withMacAnimation { notices.dismissActive() } })
                    .task(id: usage.id) {
                        notices.markUsageShown(uid: MacAccount.shared.session.user?.uid) // 見えた時点で数える
                        MacVoiceOver.announce(UsageCopy.notice(usage))
                        try? await Task.sleep(for: .seconds(MacVoiceOver.holdSeconds(8)))
                        guard !Task.isCancelled, notices.usage == usage else { return }
                        withMacAnimation { notices.dismissActive() }
                    }
            }
        }
        .padding(.top, 12)
        .padding(.horizontal, 16)
        .animation(reduceMotion ? nil : .easeInOut(duration: 0.2), value: notices.summary)
        .animation(reduceMotion ? nil : .easeInOut(duration: 0.2), value: notices.showsEditHint)
        .animation(reduceMotion ? nil : .easeInOut(duration: 0.2), value: notices.publicData)
        .animation(reduceMotion ? nil : .easeInOut(duration: 0.2), value: notices.retention)
        .animation(reduceMotion ? nil : .easeInOut(duration: 0.2), value: notices.newItems)
        .animation(reduceMotion ? nil : .easeInOut(duration: 0.2), value: notices.usage)
    }

    /// サービスのお知らせ(#333)。列を押し下げる段に、閉じるまで出す(iOS の `UpdateGateModifier` と同じ)
    @ViewBuilder private var serviceNoticeBar: some View {
        if let notice = notices.visibleService {
            MacServiceNoticeBar(notice: notice, dismiss: { withMacAnimation { notices.dismissService() } })
                .padding(.top, 12)
                .padding(.horizontal, 16)
                .task(id: notice.id) {
                    let text = notice.text(languageCode: LanguagePreference.shared.code)
                    MacVoiceOver.announce(notice.severity == .warning ? L10n.tr("重要なお知らせ: \(text)") : L10n.tr("お知らせ: \(text)"))
                }
                .animation(reduceMotion ? nil : .easeInOut(duration: 0.2), value: notices.visibleService)
        }
    }

    /// 新着・上限のお知らせを数え直すきっかけ: 中央のアプリ(版)・アカウント・前面に戻った
    private var appNoticeKey: String {
        "\(selectedApp.map(Self.canvasKey) ?? "")|\(MacAccount.shared.session.user?.uid ?? "")|\(noticeCheck)"
    }

    /// 中央のアプリを開いた時の新着(#268)と上限(#317)。iOS と同じく上限は開いた直後のバナーと重ねないよう少し待つ
    private func checkAppNotices() async {
        notices.usage = nil
        // 保存期間の確認(#482): 記録を端末に残す作ったアプリだけ(同梱の例の記録は一時フォルダ)。確認は端末ごと・アプリごと
        if let app = selectedApp, library.isCreated(app.id), let document = library.generated[app.id] {
            notices.noteRetention(appId: app.id, version: document.version, dsl: document.dsl)
        } else {
            notices.retention = nil
        }
        guard let app = selectedApp, library.isCreated(app.id), library.isOnServer(app.id) else {
            notices.newItems = nil
            return
        }
        let uid = MacAccount.shared.session.user?.uid
        let entities = library.generated[app.id]?.dsl.entities.map(\.id) ?? []
        await notices.checkNewItems(appId: app.id, entities: entities, uid: uid,
                                    source: MacServices.shared.appConfig as? SharedRecordStampSource)
        try? await Task.sleep(for: .seconds(1.5))
        guard !Task.isCancelled, let source = MacServices.shared.appConfig as? PlanUsageSource else { return }
        let store = PlanUsageStore.shared
        let usage = await store.refreshApp(appId: app.id, source: source)
        let me = await store.refreshMe(source: source)
        guard !Task.isCancelled, selectedApp?.id == app.id else { return }
        notices.offerUsage(MacPostBuildNotices.usageCandidates(app: usage, me: me), uid: uid)
    }

    /// サービスのお知らせ(#333)と Mac の最低対応版(#981)を 1 回で読む。appconfig につながっていない時は何もしない(fail open)
    private func checkServiceNotice() async {
        #if DEBUG
        updateGate.applyDebugOverride() // METAMON_FORCE_UPDATE の見た目の確認(通信なしでも出す)
        #endif
        guard let url = MacServices.shared.appConfigURL else { return }
        let result = await MacUpdateGate.checker(baseURL: url).check()
        guard !Task.isCancelled else { return }
        notices.updateService(result.notice)
        updateGate.apply(result)
    }

    /// 上限のお知らせの「Pro にする」。Pro・審査用の版・上がらない上限では出さない
    private func usageUpgrade(_ notice: UsageNotice) -> (() -> Void)? {
        #if APP_STORE_REVIEW
        return nil
        #else
        guard UsageCopy.showsProButton(notice, isPro: RemainingTodayPlan.isPro) else { return nil }
        return {
            withMacAnimation { notices.dismissActive() }
            MacPro.shared.requestPro(.planLimit)
        }
        #endif
    }

    /// サイドバーの選択。中央のアプリに保存していない入力があれば、確認して「破棄」の時だけ切り替える(#943)
    private var guardedSelection: Binding<String?> {
        Binding(get: { selectedAppId }, set: { id in
            // ⌥クリックは選ばずに別のウインドウで開く(#1853)
            if let id, id != selectedAppId, MacAppWindows.opensNewWindow(NSApp.currentEvent) { return openAppWindow(id) }
            switchApp(to: id)
        })
    }

    /// 生成アプリを別のウインドウで開く(#1853)。もう開いていればそのウインドウが前に出る
    private func openAppWindow(_ id: String) {
        openWindow(id: MacAppWindows.windowId, value: AppID(rawValue: id))
    }

    private func switchApp(to id: String?) {
        guard id != selectedAppId else { return }
        discardGuard.run { selectedAppId = id }
    }

    /// シート(会話・頼み方の例)を開いている間は ⌥⌘↑↓ で切り替えない。確認がシートの裏に出るため(#943)
    private var canSwitchApp: Bool { compose == nil && !showsTips }

    /// 押された通知の切り替え先。シートの間は覚えておき、閉じた後に `switchAfterSheet` で切り替える(#943)
    private var notificationSelection: Binding<String?> {
        Binding(get: { selectedAppId }, set: { id in
            guard let id, let now = notificationSwitch.request(id, sheetOpen: !canSwitchApp) else { return }
            switchApp(to: now)
        })
    }

    /// シートを閉じた。待っていた通知の切り替えがあれば、確認を通して切り替える
    private func switchAfterSheet() {
        guard let id = notificationSwitch.sheetClosed(sheetOpen: !canSwitchApp), apps.contains(where: { $0.id == id }) else { return }
        switchApp(to: id)
    }

    private var selectedIndex: Int? { apps.firstIndex { $0.id == selectedAppId } }

    /// サイドバーに見えているアプリの id(検索で絞った後・#1282)。前 / 次のアプリはこの中で移る
    private var visibleAppIds: [String] { apps.filter { $0.matches(searchText) }.map(\.id) }

    private var shellActions: ShellActions {
        let visible = visibleAppIds
        return ShellActions(
            hasSelection: selectedIndex != nil,
            canSelectPrevious: canSwitchApp && Self.step(from: selectedAppId, offset: -1, in: visible) != nil,
            canSelectNext: canSwitchApp && Self.step(from: selectedAppId, offset: 1, in: visible) != nil,
            selectPrevious: { select(offset: -1) },
            selectNext: { select(offset: 1) },
            focusSidebar: {
                if columnVisibility == .detailOnly { columnVisibility = .all }
                focus = .sidebar
            },
            focusCanvas: { focus = .canvas },
            focusSearch: focusSearch,
            canCompose: canCompose,
            canUndoVersion: canCompose && selectedAppId.map(library.canUndo) == true,
            canRedoVersion: canCompose && selectedAppId.map(library.canRedo) == true,
            undoVersion: { undoVersion() },
            redoVersion: { redoVersion() },
            newApp: newApp,
            ask: ask,
            speak: speak,
            showTips: { showsTips = true },
            canShowScreenMap: canShowScreenMap,
            showScreenMap: showScreenMap
        )
    }

    /// ⌘F: サイドバーを出して検索欄に入る(#1282)
    private func focusSearch() {
        if columnVisibility == .detailOnly { columnVisibility = .all }
        if #available(macOS 15.0, *) {
            searchFocused = true
        } else {
            // macOS 14 は検索欄のフォーカスを SwiftUI から動かせないので、ウインドウの検索欄を直接選ぶ
            DispatchQueue.main.async {
                guard let window = NSApp.keyWindow, let field = Self.searchField(in: window.contentView) else { return }
                window.makeFirstResponder(field)
            }
        }
    }

    private static func searchField(in view: NSView?) -> NSSearchField? {
        guard let view else { return nil }
        if let field = view as? NSSearchField { return field }
        for child in view.subviews {
            if let field = searchField(in: child) { return field }
        }
        return nil
    }

    /// 前 / 次のアプリ。サイドバーに見えている中で移る(検索で隠れたアプリには移らない)。
    private func select(offset: Int) {
        guard canSwitchApp, let next = Self.step(from: selectedAppId, offset: offset, in: visibleAppIds) else { return }
        switchApp(to: next)
    }

    /// `visible` の中で `selected` から `offset` 先の id。端より先は nil。
    /// 選んでいない(または検索で隠れている)時は、次なら先頭・前なら nil。
    static func step(from selected: String?, offset: Int, in visible: [String]) -> String? {
        guard let index = selected.flatMap({ visible.firstIndex(of: $0) }) else {
            return offset > 0 ? visible.first : nil
        }
        let next = index + offset
        return visible.indices.contains(next) ? visible[next] : nil
    }
}

/// ツールバー。ショートカットはメニュー(MacCommands)が持つ。
/// 新しいアプリ・文字で頼む・声で頼むは会話のシート(#967・#966)。画面マップはシート(#970)。
/// 共有はシェアリンクのシート(#1035)。サーバーに保存した自分のアプリの時だけ押せる。
struct ShellToolbar: ToolbarContent {
    let hasApp: Bool
    var canCompose = true
    var newApp: () -> Void = {}
    var ask: () -> Void = {}
    var speak: () -> Void = {}
    var canShowScreenMap = false
    var showScreenMap: () -> Void = {}
    var canShare = false
    var share: () -> Void = {}
    var canShowAppSettings = false
    var showAppSettings: () -> Void = {}

    var body: some ToolbarContent {
        ToolbarItemGroup(placement: .primaryAction) {
            Button(action: newApp) { Label("新しいアプリ", systemImage: "plus") }
                .help("新しいアプリを作る")
            Button(action: speak) { Label("声で頼む", systemImage: "mic") }
                .help(hasApp ? "声でこのアプリを直す" : "声でアプリを作る")
                .disabled(!canCompose)
            Button(action: ask) { Label("文字で頼む", systemImage: "text.cursor") }
                .help(hasApp ? "文字でこのアプリを直す" : "文字でアプリを作る")
        }
        ToolbarItemGroup(placement: .secondaryAction) {
            Button(action: showScreenMap) { Label("画面マップ", systemImage: "map") }
                .help("画面のつながりを見る")
                .disabled(!canShowScreenMap)
            Button(action: share) { Label("共有", systemImage: "square.and.arrow.up") }
                .help(shareHelp)
                .disabled(!canShare)
            Button(action: showAppSettings) { Label("このアプリの設定", systemImage: "slider.horizontal.3") }
                .help("記録の自動削除・自動の動きなど、このアプリの設定を開く")
                .disabled(!canShowAppSettings)
            SettingsLink { Label("設定", systemImage: "gearshape") }
                .help("設定を開く")
        }
    }
}

extension ShellToolbar {
    var shareHelp: String {
        #if APP_STORE_REVIEW
        String(localized: "このアプリを共有する")
        #else
        MacShareAppModel.help(canShare: canShare, hasApp: hasApp)
        #endif
    }
}

/// 検索欄のフォーカス(macOS 15 から)。⌘F で入るのに使う(#1282)
private struct SidebarSearchFocus: ViewModifier {
    var focused: FocusState<Bool>.Binding

    func body(content: Content) -> some View {
        if #available(macOS 15.0, *) {
            content.searchFocused(focused)
        } else {
            content
        }
    }
}
