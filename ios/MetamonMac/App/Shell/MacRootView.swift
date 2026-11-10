import SwiftUI

/// Mac の画面の骨格(#933): 左にアプリの一覧・中央に選んだアプリ・上にツールバー。
/// iOS で sheet だった本体の画面の出し方:
/// - アプリの切り替え(AppSwitcherSheet) → サイドバー(常に出す・閉じられる)
/// - 音声 / 文字で頼む(VoiceFab) → ツールバーのボタン + ショートカット。生成したアプリの中の FAB は残す
/// - 設定 → Mac の設定ウインドウ(⌘,)
/// - 画面の上のお知らせ → 中央の列の上(後の Issue)
struct MacRootView: View {
    static let minWindowSize = CGSize(width: 720, height: 480)

    let apps: [SampleApp]
    @SceneStorage("selectedAppId") private var selectedAppId: String?
    @State private var columnVisibility = NavigationSplitViewVisibility.all

    private var selectedApp: SampleApp? { apps.first { $0.id == selectedAppId } }

    var body: some View {
        NavigationSplitView(columnVisibility: $columnVisibility) {
            AppSidebar(apps: apps, selection: $selectedAppId)
                .navigationSplitViewColumnWidth(min: 180, ideal: 220, max: 320)
        } detail: {
            Group {
                if let app = selectedApp {
                    AppCanvas(app: app).id(app.id)
                } else {
                    ContentUnavailableView("アプリを選んでください", systemImage: "sidebar.left",
                                           description: Text("左の一覧から選ぶと、ここに表示します。"))
                }
            }
            .frame(minWidth: 480, minHeight: 400)
        }
        .toolbar { ShellToolbar(hasApp: selectedApp != nil) }
        .frame(minWidth: Self.minWindowSize.width, minHeight: Self.minWindowSize.height)
    }
}

/// ツールバー。動きは後の Issue で付けるので、今は置くだけ(押せない)。
struct ShellToolbar: ToolbarContent {
    let hasApp: Bool

    var body: some ToolbarContent {
        ToolbarItemGroup(placement: .primaryAction) {
            Button {} label: { Label("新しいアプリ", systemImage: "plus") }
                .keyboardShortcut("n", modifiers: .command)
                .help("新しいアプリを作る")
                .disabled(true)
            Button {} label: { Label("声で頼む", systemImage: "mic") }
                .keyboardShortcut("m", modifiers: [.command, .shift])
                .help("声でアプリを作る・直す")
                .disabled(true)
            Button {} label: { Label("文字で頼む", systemImage: "text.cursor") }
                .keyboardShortcut("k", modifiers: .command)
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
