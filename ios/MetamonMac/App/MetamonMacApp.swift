import SwiftUI
import MetamonDSL
import MetamonRenderer
import MetamonServices

/// Mac の殻(#929)。画面の骨格は #933(Shell/)、メニューとキーボードは #934(MacCommands)。
/// Firebase は #930、権限の説明は #931 で足す。
@main
struct MetamonMacApp: App {
    private let apps = SampleAppCatalog.load()
    /// 声で頼む(#966)。マイクは 1 つなのでウインドウをまたいで 1 つだけ持つ
    @State private var voice = MacVoiceInput()

    var body: some Scene {
        WindowGroup(id: MacCommands.mainWindowId) {
            MacRootView(apps: apps)
                .environment(voice)
                #if !APP_STORE_REVIEW
                .modifier(MacLinkReceiver()) // tailorit:// と共有のリンク(#975)
                #endif
        }
        .defaultSize(width: 1100, height: 720)
        .windowResizability(.contentMinSize)
        .commands { MacCommands() }

        Settings {
            MacEmptyState(pose: .tea, title: "設定はまだありません", mascotSize: MacTheme.Mascot.small)
                .frame(width: 420, height: 260)
        }
    }
}
