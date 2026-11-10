import SwiftUI
import MetamonDSL

/// Mac の画面の骨格(#933): 左にアプリの一覧・中央に選んだアプリ・上にツールバー。
/// iOS で sheet だった本体の画面の出し方:
/// - アプリの切り替え(AppSwitcherSheet) → サイドバー(常に出す・閉じられる)
/// - 音声 / 文字で頼む(VoiceFab) → ツールバーのボタン + ショートカット。生成したアプリの中の FAB は残す
///   文字で頼む(⌘K)・新しいアプリ(⌘N)は会話のシート(#967・ComposeSheet)。作ったアプリは一覧の先頭に足し、直したアプリは差し替える
///   声で頼む(⇧⌘M・#966)は同じシートを開いてすぐ録音する。文字にした文をそのまま送る
/// - 設定 → Mac の設定ウインドウ(⌘,)
/// - 完成の後のお知らせ(要約・「元に戻す」)→ 中央の列の上に 1 つずつ(#968・順番は iOS と同じ `PostBuildNotice`)
/// - 生成したアプリの版の「元に戻す / やり直す」(#968)→「アプリ」メニュー(⌥⌘Z / ⇧⌥⌘Z)・完成の画面・要約のお知らせ。
///   ⌘Z / ⇧⌘Z は入力欄と記録の削除の取り消し(編集メニュー)のまま
/// キーボード(#934): ⌘1 / ⌘2 でサイドバーと中央を行き来・一覧は ↑↓ で選び Return で中央へ・⌥⌘↑↓ で前 / 次のアプリ。
/// 一覧(#969): 「作ったアプリ」(端末に残す・名前の変更 / 削除 / 並べ替え)と「同梱の例」に分ける。起動時は前に開いていたアプリを開き直す。
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
            AppSidebar(created: createdApps, examples: exampleApps, selection: $selectedAppId,
                       onOpen: { focus = .canvas }, onDelete: deleteApp)
                .focused($focus, equals: .sidebar)
                .navigationSplitViewColumnWidth(min: 180, ideal: 220, max: 320)
        } detail: {
            Group {
                if let app = selectedApp {
                    AppCanvas(app: app, dsl: library.generated[app.id]?.dsl).id("\(app.id)#\(app.version)")
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
        .toolbar { ShellToolbar(hasApp: selectedApp != nil, canCompose: compose == nil, newApp: newApp, ask: ask, speak: speak) }
        .frame(minWidth: Self.minWindowSize.width, minHeight: Self.minWindowSize.height)
        .focusedSceneValue(\.shellActions, shellActions)
        .sheet(item: $compose, onDismiss: noteFinishedCompose) { session in
            ComposeSheet(session: session, onClose: { compose = nil }, undo: finishedUndo(session))
                .interactiveDismissDisabled(session.isBusy)
                .environment(voice)
        }
        .task { voice.prepareIfAllowed() } // モデルが端末にあれば読み込む(無ければ起動では聞かない)
        .onAppear(perform: reopenLastApp)
        .onChange(of: selectedAppId) { _, id in
            if let id { library.lastAppId = id }
        }
        .sheet(isPresented: $showsTips) {
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

    /// 作ったアプリを消す(確認はサイドバーで済んでいる)。開いていたら一覧で隣のアプリへ移る
    private func deleteApp(_ id: String) {
        let name = library.generated[id]?.dsl.app.name ?? ""
        if selectedAppId == id { selectedAppId = LocalAppLibrary.neighbor(deleting: id, in: apps.map(\.id)) }
        if notices.summary?.appId == id { notices.summary = nil }
        library.delete(id)
        MacVoiceOver.announce(L10n.tr("「\(name)」を削除しました"))
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

    private func open(_ target: MacComposeSession.Target, draft: String, voice: Bool = false) {
        guard compose == nil else { return }
        let session = MacComposeSession(target: target, draft: draft)
        session.startsWithVoice = voice
        session.onResult = { document, envelope in apply(document, envelope: envelope, target: target) }
        notices.summary = nil // 会話を始めたら前の要約は下げる
        closingCompose = session
        compose = session
    }

    /// 結果を中央に当てる。作ったアプリは一覧の先頭に足して選ぶ・直したアプリはその場で差し替える(中央を作り直す)
    private func apply(_ document: AppDocument, envelope: Data?, target: MacComposeSession.Target) {
        library.apply(document, envelope: envelope, created: target == .create)
        selectedAppId = document.appId
    }

    // MARK: - 完成の後のお知らせ・版の取り消し(#968)

    /// シートが閉じた。完成していたら要約のお知らせを出す(iOS の要約バナーと同じく、直した時は「元に戻す」付き)
    private func noteFinishedCompose() {
        defer { closingCompose = nil }
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

    private func undoVersion(_ appId: String? = nil) {
        guard let id = appId ?? selectedAppId, library.undo(id) else { return }
        selectedAppId = id
        notices.summary = .init(appId: id, text: L10n.tr("元に戻しました"), action: .redo)
    }

    private func redoVersion(_ appId: String? = nil) {
        guard let id = appId ?? selectedAppId, library.redo(id) else { return }
        selectedAppId = id
        notices.summary = .init(appId: id, text: L10n.tr("やり直しました"), action: library.canUndo(id) ? .undo : nil)
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
            }
        }
        .padding(.top, 12)
        .padding(.horizontal, 16)
        .animation(reduceMotion ? nil : .easeInOut(duration: 0.2), value: notices.summary)
    }

    private var selectedIndex: Int? { apps.firstIndex { $0.id == selectedAppId } }

    private var shellActions: ShellActions {
        let index = selectedIndex
        return ShellActions(
            hasSelection: index != nil,
            canSelectPrevious: (index ?? 0) > 0,
            canSelectNext: index.map { $0 + 1 < apps.count } ?? !apps.isEmpty,
            selectPrevious: { select(offset: -1) },
            selectNext: { select(offset: 1) },
            focusSidebar: {
                if columnVisibility == .detailOnly { columnVisibility = .all }
                focus = .sidebar
            },
            focusCanvas: { focus = .canvas },
            canCompose: compose == nil,
            canUndoVersion: compose == nil && selectedAppId.map(library.canUndo) == true,
            canRedoVersion: compose == nil && selectedAppId.map(library.canRedo) == true,
            undoVersion: { undoVersion() },
            redoVersion: { redoVersion() },
            newApp: newApp,
            ask: ask,
            speak: speak,
            showTips: { showsTips = true }
        )
    }

    /// 前 / 次のアプリ。何も選んでいなければ先頭を選ぶ。
    private func select(offset: Int) {
        guard !apps.isEmpty else { return }
        let next = selectedIndex.map { $0 + offset } ?? 0
        guard apps.indices.contains(next) else { return }
        selectedAppId = apps[next].id
    }
}

/// ツールバー。ショートカットはメニュー(MacCommands)が持つ。
/// 新しいアプリ・文字で頼む・声で頼むは会話のシート(#967・#966)。画面マップ・共有は後の Issue なので今は押せない。
struct ShellToolbar: ToolbarContent {
    let hasApp: Bool
    var canCompose = true
    var newApp: () -> Void = {}
    var ask: () -> Void = {}
    var speak: () -> Void = {}

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
            Button {} label: { Label("画面マップ", systemImage: "map") }
                .help("画面のつながりを見る")
                .disabled(true)
            Button {} label: { Label("共有", systemImage: "square.and.arrow.up") }
                .help("このアプリを共有する")
                .disabled(true)
            SettingsLink { Label("設定", systemImage: "gearshape") }
                .help("設定を開く")
        }
    }
}
