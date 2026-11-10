import SwiftUI

/// Mac の画面の骨格(#933): 左にアプリの一覧・中央に選んだアプリ・上にツールバー。
/// iOS で sheet だった本体の画面の出し方:
/// - アプリの切り替え(AppSwitcherSheet) → サイドバー(常に出す・閉じられる)
/// - 音声 / 文字で頼む(VoiceFab) → ツールバーのボタン + ショートカット。生成したアプリの中の FAB は残す
/// - 設定 → Mac の設定ウインドウ(⌘,)
/// - 画面の上のお知らせ → 中央の列の上(後の Issue)
/// キーボード(#934): ⌘1 / ⌘2 でサイドバーと中央を行き来・一覧は ↑↓ で選び Return で中央へ・⌥⌘↑↓ で前 / 次のアプリ。
struct MacRootView: View {
    enum Pane: Hashable { case sidebar, canvas }

    static let minWindowSize = CGSize(width: 720, height: 480)

    let apps: [SampleApp]
    @SceneStorage("selectedAppId") private var selectedAppId: String?
    @State private var columnVisibility = NavigationSplitViewVisibility.all
    @FocusState private var focus: Pane?

    private var selectedApp: SampleApp? { apps.first { $0.id == selectedAppId } }

    var body: some View {
        NavigationSplitView(columnVisibility: $columnVisibility) {
            AppSidebar(apps: apps, selection: $selectedAppId) { focus = .canvas }
                .focused($focus, equals: .sidebar)
                .navigationSplitViewColumnWidth(min: 180, ideal: 220, max: 320)
        } detail: {
            Group {
                if let app = selectedApp {
                    AppCanvas(app: app).id(app.id)
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
        .toolbar { ShellToolbar(hasApp: selectedApp != nil) }
        .frame(minWidth: Self.minWindowSize.width, minHeight: Self.minWindowSize.height)
        .focusedSceneValue(\.shellActions, shellActions)
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
            focusCanvas: { focus = .canvas }
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

/// ツールバー。動きは後の Issue で付けるので、今は置くだけ(押せない)。ショートカットはメニュー(MacCommands)が持つ。
struct ShellToolbar: ToolbarContent {
    let hasApp: Bool

    var body: some ToolbarContent {
        ToolbarItemGroup(placement: .primaryAction) {
            Button {} label: { Label("新しいアプリ", systemImage: "plus") }
                .help("新しいアプリを作る")
                .disabled(true)
            Button {} label: { Label("声で頼む", systemImage: "mic") }
                .help("声でアプリを作る・直す")
                .disabled(true)
            Button {} label: { Label("文字で頼む", systemImage: "text.cursor") }
                .help("文字でアプリを作る・直す")
                .disabled(true)
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
