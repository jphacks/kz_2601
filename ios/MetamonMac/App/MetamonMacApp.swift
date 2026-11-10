import SwiftUI
import MetamonDSL
import MetamonRenderer
import MetamonServices

/// Mac の殻(#929)。画面の骨格は #933(Shell/)、メニューとキーボードは #934(MacCommands)。
/// Firebase は #930、権限の説明は #931 で足す。
@main
struct MetamonMacApp: App {
    private let apps = SampleAppCatalog.load()

    var body: some Scene {
        WindowGroup(id: MacCommands.mainWindowId) {
            MacRootView(apps: apps)
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
