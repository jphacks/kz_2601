import SwiftUI
import MetamonDSL

/// Mac の画面の骨格(#933): 左にアプリの一覧・中央に選んだアプリ・上にツールバー。
/// iOS で sheet だった本体の画面の出し方:
/// - アプリの切り替え(AppSwitcherSheet) → サイドバー(常に出す・閉じられる)
/// - 音声 / 文字で頼む(VoiceFab) → ツールバーのボタン + ショートカット。生成したアプリの中の FAB は残す
///   文字で頼む(⌘K)・新しいアプリ(⌘N)は会話のシート(#967・ComposeSheet)。作ったアプリは一覧の先頭に足し、直したアプリは差し替える
///   声で頼む(⇧⌘M・#966)は同じシートを開いてすぐ録音する。文字にした文をそのまま送る
/// - 設定 → Mac の設定ウインドウ(⌘,)
/// - 画面の上のお知らせ → 中央の列の上(後の Issue)
/// キーボード(#934): ⌘1 / ⌘2 でサイドバーと中央を行き来・一覧は ↑↓ で選び Return で中央へ・⌥⌘↑↓ で前 / 次のアプリ。
struct MacRootView: View {
    enum Pane: Hashable { case sidebar, canvas }

    static let minWindowSize = CGSize(width: 720, height: 480)

    let bundledApps: [SampleApp]
    @SceneStorage("selectedAppId") private var selectedAppId: String?
    @State private var columnVisibility = NavigationSplitViewVisibility.all
    @FocusState private var focus: Pane?
    /// 会話で作った・直したアプリの DSL(#967)。サーバーが無い間はウインドウを閉じれば消える(iOS の METAMON_MOCK と同じ)
    @State private var generated: [String: AppDocument] = [:]
    /// 会話で作ったアプリの id(新しい順)
    @State private var createdIds: [String] = []
    @State private var compose: MacComposeSession?
    @State private var showsTips = false
    @Environment(MacVoiceInput.self) private var voice

    init(apps: [SampleApp]) { bundledApps = apps }

    /// 一覧に出すアプリ: 作ったアプリ(新しい順)+ 同梱の例(直したものは差し替え)
    private var apps: [SampleApp] {
        createdIds.compactMap { id in generated[id].map { Self.entry(id: id, $0) } }
            + bundledApps.map { app in generated[app.id].map { Self.entry(id: app.id, $0) } ?? app }
    }

    private static func entry(id: String, _ document: AppDocument) -> SampleApp {
        SampleApp(id: id, name: document.dsl.app.name, description: document.dsl.app.description, url: nil, version: document.version)
    }

    private var selectedApp: SampleApp? { apps.first { $0.id == selectedAppId } }

    var body: some View {
        NavigationSplitView(columnVisibility: $columnVisibility) {
            AppSidebar(apps: apps, selection: $selectedAppId) { focus = .canvas }
                .focused($focus, equals: .sidebar)
                .navigationSplitViewColumnWidth(min: 180, ideal: 220, max: 320)
        } detail: {
            Group {
                if let app = selectedApp {
                    AppCanvas(app: app, dsl: generated[app.id]?.dsl).id("\(app.id)#\(app.version)")
                } else {
                    MacEmptyState(pose: .point, title: "アプリを選んでください",
                                  message: "左の一覧から選ぶと、ここに表示します。")
                }
            }
            .frame(minWidth: 480, minHeight: 400)
            // 中央の列をフォーカスの行き先にする(Tab で中の入力欄・ボタンへ進む)
            .focusable()
            .focusEffectDisabled()
            .focused($focus, equals: .canvas)
        }
        .toolbar { ShellToolbar(hasApp: selectedApp != nil, canCompose: compose == nil, newApp: newApp, ask: ask, speak: speak) }
        .frame(minWidth: Self.minWindowSize.width, minHeight: Self.minWindowSize.height)
        .focusedSceneValue(\.shellActions, shellActions)
        .macNotificationRouting(selection: $selectedAppId, appIds: apps.map(\.id)) // 押された通知のアプリを開く(#965)
        .sheet(item: $compose) { session in
            ComposeSheet(session: session) { compose = nil }
                .interactiveDismissDisabled(session.isBusy)
                .environment(voice)
        }
        .task { voice.prepareIfAllowed() } // モデルが端末にあれば読み込む(無ければ起動では聞かない)
        .sheet(isPresented: $showsTips) {
            PromptTipsSheet { tip in
                showsTips = false
                ask(draft: tip)
            }
        }
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
        session.onResult = { document in apply(document, target: target) }
        compose = session
    }

    /// 結果を中央に当てる。作ったアプリは一覧の先頭に足して選ぶ・直したアプリはその場で差し替える(中央を作り直す)
    private func apply(_ document: AppDocument, target: MacComposeSession.Target) {
        generated[document.appId] = document
        if target == .create, !createdIds.contains(document.appId) { createdIds.insert(document.appId, at: 0) }
        // 最初のアプリの印(iOS と同じ `hasShownEditHint`)。β の案内(`MacOnboarding`)は次の起動でこれを見て 1 回出す
        if target == .create { FirstRunTutorialStore.shared.markEditHintShown() }
        selectedAppId = document.appId
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
