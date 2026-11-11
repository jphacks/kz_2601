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
/// - 生成したアプリの版の「元に戻す / やり直す」(#968)→「アプリ」メニュー(⌥⌘Z / ⇧⌥⌘Z)・完成の画面・要約のお知らせ。
///   ⌘Z / ⇧⌘Z は入力欄と記録の削除の取り消し(編集メニュー)のまま
/// キーボード(#934): ⌘1 / ⌘2 でサイドバーと中央を行き来・一覧は ↑↓ で選び Return で中央へ・⌥⌘↑↓ で前 / 次のアプリ。
/// 画面マップ(#970): ツールバー・表示メニュー(⇧⌘E)からシートで出す。開く → 中央でその画面へ、話す → その画面を文脈にした会話(⌘K と同じシート)。
/// 一覧(#969): 「作ったアプリ」(端末に残す・名前の変更 / 削除 / 並べ替え)と「同梱の例」に分ける。起動時は前に開いていたアプリを開き直す。
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
    /// 完成の後のお知らせ(#968)
    @State private var notices = MacPostBuildNotices()
    @State private var compose: MacComposeSession?
    /// 閉じた会話のシート(閉じた後に完成の要約を出すため・#968)
    @State private var closingCompose: MacComposeSession?
    @State private var showsTips = false
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
    /// 会話で直した直後に光らせる部品(#626・#1337)。iOS と同じく出現が済んだ頃(2 秒)に消す
    @State private var highlight: (canvas: String, value: ChangeHighlight)?
    @Environment(MacVoiceInput.self) private var voice
    @Environment(\.accessibilityReduceMotion) private var reduceMotion

    init(apps: [SampleApp]) { bundledApps = apps }

    /// 作ったアプリ(一覧の並び)
    private var createdApps: [SampleApp] {
        library.createdIds.compactMap { id in library.generated[id].map { Self.entry(id: id, $0) } }
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
            AppSidebar(created: createdApps.filter { $0.matches(searchText) },
                       examples: exampleApps.filter { $0.matches(searchText) },
                       thumbnail: thumbnail(for:), searchText: searchText, selection: guardedSelection,
                       onOpen: { focus = .canvas }, onDelete: deleteApp)
                .focused($focus, equals: .sidebar)
                .navigationSplitViewColumnWidth(min: 180, ideal: 220, max: 320)
        } detail: {
            Group {
                if let app = selectedApp {
                    AppCanvas(app: app, dsl: library.generated[app.id]?.dsl, showsScreenMap: $showsScreenMap,
                              onTalk: { talk(about: $0) }, discardGuard: discardGuard)
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
            // 中央の列をフォーカスの行き先にする(Tab で中の入力欄・ボタンへ進む)
            .focusable()
            .focusEffectDisabled()
            .focused($focus, equals: .canvas)
        }
        .searchable(text: $searchText, placement: .sidebar, prompt: Text("アプリを探す"))
        .modifier(SidebarSearchFocus(focused: $searchFocused))
        .toolbar {
            ShellToolbar(hasApp: selectedApp != nil, canCompose: canCompose, newApp: newApp, ask: ask, speak: speak,
                         canShowScreenMap: canShowScreenMap, showScreenMap: showScreenMap)
        }
        .frame(minWidth: Self.minWindowSize.width, minHeight: Self.minWindowSize.height)
        .focusedSceneValue(\.shellActions, shellActions)
        .macNotificationRouting(selection: notificationSelection, appIds: apps.map(\.id)) // 押された通知のアプリを開く(#965)。入力中なら確認を通す・シートの間は閉じるまで待つ(#943)
        .background { WindowCloseGuard(discardGuard: discardGuard) } // ⌘W・閉じるボタン(#943)
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
        .onAppear {
            reopenLastApp()
            // self ごと捕まえると見張り(State)との循環で残るので、シートの Binding だけ渡す
            let map = $showsScreenMap, tips = $showsTips
            discardGuard.beforeAsking = { map.wrappedValue = false; tips.wrappedValue = false }
        }
        .onChange(of: notices.summary == nil) { _, closed in
            guard closed else { return }
            // お知らせが写り込んだ時は、閉じて消え終わってから撮り直す
            Task { @MainActor in
                try? await Task.sleep(for: .seconds(0.4))
                captureThumbnail()
            }
        }
        .onChange(of: selectedAppId) { _, id in
            if let id { library.lastAppId = id }
            showsScreenMap = false
        }
        .sheet(isPresented: $showsTips, onDismiss: switchAfterSheet) {
            PromptTipsSheet { tip in
                showsTips = false
                ask(draft: tip)
            }
        }
    }

    // MARK: - 一覧(#969)

    /// 起動時: ウインドウが覚えている選択(SceneStorage)が無い・もう無いアプリなら、前に開いていたアプリを開き直す
    private func reopenLastApp() {
        let ids = apps.map(\.id)
        if let id = selectedAppId, ids.contains(id) { return }
        selectedAppId = LocalAppLibrary.reopen(library.lastAppId, among: ids)
    }

    /// 置き換えシートに並べるアプリ(この Mac で作ったアプリ)と消し方(#630)。版が 1 の空のアプリとして数えないよう 2 以上にする
    private var appReplaceContext: MacAppReplaceContext {
        let summaries = library.createdIds.compactMap { id in
            library.generated[id].map { AppSummary(id: id, name: $0.dsl.app.name, currentVersion: max($0.version, 2)) }
        }
        return MacAppReplaceContext(apps: summaries, current: selectedAppId, delete: deleteApp)
    }

    /// 作ったアプリを消す(確認はサイドバーで済んでいる)。開いていたら一覧で隣のアプリへ移る
    private func deleteApp(_ id: String) {
        let name = library.generated[id]?.dsl.app.name ?? ""
        if selectedAppId == id { selectedAppId = LocalAppLibrary.neighbor(deleting: id, in: apps.map(\.id)) }
        if notices.summary?.appId == id { notices.summary = nil }
        library.delete(id)
        thumbs.remove(appId: id)
        Task { await MacRuntimeNotifications.removeApp(id) } // そのアプリの通知とタイマーの予約(#1324)
        MacVoiceOver.announce(L10n.tr("「\(name)」を削除しました"))
    }

    // MARK: - 行の写し(#1281)

    private func thumbnail(for appId: String) -> NSImage? {
        _ = thumbs.revision // 撮り直しで描き直す
        return thumbs.image(for: appId)
    }

    /// 中央のアプリを撮る。お知らせ・会話のシートが出ている間は撮らない(閉じた後に撮り直す)
    private func captureThumbnail() {
        guard let app = selectedApp, notices.summary == nil, compose == nil else { return }
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

    private func ask(draft: String, voice: Bool = false) {
        if let app = selectedApp {
            open(.edit(appId: app.id, name: app.name, version: app.version), draft: draft, voice: voice)
        } else {
            open(.create, draft: draft, voice: voice)
        }
    }

    private func open(_ target: MacComposeSession.Target, draft: String, voice: Bool = false, screen: ScreenContext? = nil) {
        guard canCompose else { return }
        // 会話の結果は中央のアプリを作り直す(直す・作る どちらも)。入力があれば会話を始める前に確認する(#943)。
        // シートの間は中央に触れないので、ここで「破棄」と答えたら閉じるまで聞き直さない(確認をシートの裏に出さない)
        discardGuard.run {
            let session = MacComposeSession(target: target, draft: draft)
            session.startsWithVoice = voice
            session.screen = screen
            session.onResult = { document, envelope in apply(document, envelope: envelope, target: target) }
            notices.summary = nil // 会話を始めたら前の要約は下げる
            closingCompose = session
            compose = session
            discardGuard.released = true
        }
    }

    /// 結果を中央に当てる。作ったアプリは一覧の先頭に足して選ぶ・直したアプリはその場で差し替える(中央を作り直す)
    private func apply(_ document: AppDocument, envelope: Data?, target: MacComposeSession.Target) {
        let previous = target == .create ? nil
            : library.generated[document.appId]?.dsl ?? (try? apps.first(where: { $0.id == document.appId })?.loadDSL())
        library.apply(document, envelope: envelope, created: target == .create)
        showChangeHighlight(previous: previous, next: document)
        // 最初のアプリの印(iOS と同じ `hasShownEditHint`)。β の案内(`MacOnboarding`)は次の起動でこれを見て 1 回出す。
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

    /// 会話のシートを開ける・版を戻せるか。画面マップのシートが出ている間は開かない(シートを重ねない)
    private var canCompose: Bool { compose == nil && !showsScreenMap }

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
        notices.summary = .init(appId: appId, text: summary, action: library.canUndo(appId) ? .undo : nil)
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

    /// 版を戻す・やり直すと中央のアプリを作り直す。入力があれば先に確認する(#943)
    private func undoVersion(_ appId: String? = nil) {
        guard let id = appId ?? selectedAppId, library.canUndo(id) else { return }
        discardGuard.run {
            guard library.undo(id) else { return }
            selectedAppId = id
            notices.summary = .init(appId: id, text: L10n.tr("元に戻しました"), action: .redo)
        }
    }

    private func redoVersion(_ appId: String? = nil) {
        guard let id = appId ?? selectedAppId, library.canRedo(id) else { return }
        discardGuard.run {
            guard library.redo(id) else { return }
            selectedAppId = id
            notices.summary = .init(appId: id, text: L10n.tr("やり直しました"), action: library.canUndo(id) ? .undo : nil)
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
                                    dismiss: { withAnimation { notices.dismissActive() } })
                    .task(id: summary.id) {
                        MacVoiceOver.announce(summary.text)
                        try? await Task.sleep(for: .seconds(MacPostBuildNotices.holdSeconds(summary)))
                        guard !Task.isCancelled, notices.summary == summary else { return }
                        withAnimation { notices.dismissActive() }
                    }
            } else if compose == nil, notices.active == .editHint {
                MacEditHintBar(ask: {
                                   withAnimation { notices.dismissActive() }
                                   ask()
                               },
                               dismiss: { withAnimation { notices.dismissActive() } })
                    .task { MacVoiceOver.announce(L10n.tr("こう頼むと直せます")) }
            }
        }
        .padding(.top, 12)
        .padding(.horizontal, 16)
        .animation(reduceMotion ? nil : .easeInOut(duration: 0.2), value: notices.summary)
        .animation(reduceMotion ? nil : .easeInOut(duration: 0.2), value: notices.showsEditHint)
    }

    /// サイドバーの選択。中央のアプリに保存していない入力があれば、確認して「破棄」の時だけ切り替える(#943)
    private var guardedSelection: Binding<String?> {
        Binding(get: { selectedAppId }, set: { id in switchApp(to: id) })
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

    private var shellActions: ShellActions {
        let index = selectedIndex
        return ShellActions(
            hasSelection: index != nil,
            canSelectPrevious: canSwitchApp && (index ?? 0) > 0,
            canSelectNext: canSwitchApp && (index.map { $0 + 1 < apps.count } ?? !apps.isEmpty),
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

    /// 前 / 次のアプリ。何も選んでいなければ先頭を選ぶ。
    private func select(offset: Int) {
        guard canSwitchApp, !apps.isEmpty else { return }
        let next = selectedIndex.map { $0 + offset } ?? 0
        guard apps.indices.contains(next) else { return }
        switchApp(to: apps[next].id)
    }
}

/// ツールバー。ショートカットはメニュー(MacCommands)が持つ。
/// 新しいアプリ・文字で頼む・声で頼むは会話のシート(#967・#966)。画面マップはシート(#970)。共有は後の Issue なので今は押せない。
struct ShellToolbar: ToolbarContent {
    let hasApp: Bool
    var canCompose = true
    var newApp: () -> Void = {}
    var ask: () -> Void = {}
    var speak: () -> Void = {}
    var canShowScreenMap = false
    var showScreenMap: () -> Void = {}

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
            Button {} label: { Label("共有", systemImage: "square.and.arrow.up") }
                .help("このアプリを共有する")
                .disabled(true)
            SettingsLink { Label("設定", systemImage: "gearshape") }
                .help("設定を開く")
        }
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
