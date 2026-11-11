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

    init() {
        // UI テスト・手元の確認用(DEBUG のみ・iOS と同じ環境変数): METAMON_AI_CONSENT / METAMON_TUTORIAL / METAMON_BETA_WELCOME(#971)
        AIConsentDebug.applyLaunchOverride()
        FirstRunTutorialStore.shared.applyLaunchOverride()
        #if !APP_STORE_REVIEW
        BetaWelcomeStore.shared.applyLaunchOverride()
        #endif
    }

    var body: some Scene {
        WindowGroup(id: MacCommands.mainWindowId) {
            // 設定の「言語」で即時切替(#973)・初回の案内(#971)・声で頼む(#966)
            MacLanguageRoot { MacRootView(apps: apps).macOnboarding().environment(voice) }
        }
        .defaultSize(width: 1100, height: 720)
        .windowResizability(.contentMinSize)
        .commands { MacCommands() }

        Settings {
            MacLanguageRoot { MacSettingsView() } // ⌘,(#973)
        }
    }
}
