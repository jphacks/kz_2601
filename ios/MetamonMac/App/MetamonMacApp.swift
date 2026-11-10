import SwiftUI
import AppKit
import MetamonDSL
import MetamonRenderer
import MetamonServices

/// Mac の殻(#929)。画面の骨格は #933(Shell/)、メニューとキーボードは #934(MacCommands)。
/// Firebase は #930、権限の説明は #931 で足す。
@main
struct MetamonMacApp: App {
    /// ⌘Q の前に保存していない入力を確かめる(#943)
    @NSApplicationDelegateAdaptor(MacAppDelegate.self) private var appDelegate
    private let apps = SampleAppCatalog.load()
    /// 声で頼む(#966)。マイクは 1 つなのでウインドウをまたいで 1 つだけ持つ
    @State private var voice = MacVoiceInput()
    /// 作ったアプリの一覧(#969)。端末に残すのでウインドウをまたいで 1 つだけ持つ
    @State private var library = MacAppLibrary()

    init() {
        MacNotificationDelegate.install() // 前面でも通知を出す・押されたらそのアプリを開く(#965)
        // UI テスト・手元の確認用(DEBUG のみ・iOS と同じ環境変数): METAMON_AI_CONSENT / METAMON_TUTORIAL / METAMON_BETA_WELCOME(#971)
        AIConsentDebug.applyLaunchOverride()
        FirstRunTutorialStore.shared.applyLaunchOverride()
        #if !APP_STORE_REVIEW
        BetaWelcomeStore.shared.applyLaunchOverride()
        #endif
        // 記録の一時フォルダ(MacAppRuntime.mockRecordSync)を起動時(前回の残り)と終了時に片付ける
        MacAppRuntime.removePreviewDirectories()
        NotificationCenter.default.addObserver(forName: NSApplication.willTerminateNotification, object: nil, queue: .main) { _ in
            MainActor.assumeIsolated { MacAppRuntime.removePreviewDirectories() }
        }
        // musicPlayer(#960): 既定は MusicKit。METAMON_MOCK=1(iOS と同じ環境変数)の時だけ音を出さない偽物
        if ProcessInfo.processInfo.environment["METAMON_MOCK"] == "1" { MusicSession.shared.player = FakeMusicPlayback() }
    }

    var body: some Scene {
        WindowGroup(id: MacCommands.mainWindowId) {
            // 設定の「言語」で即時切替(#973)・初回の案内(#971)・声で頼む(#966)・tailorit:// と共有のリンク(#975)
            MacLanguageRoot {
                MacRootView(apps: apps)
                    .macOnboarding()
                    .environment(voice)
                    .environment(library)
                    #if !APP_STORE_REVIEW
                    .modifier(MacLinkReceiver())
                    #endif
            }
        }
        .defaultSize(width: 1100, height: 720)
        .windowResizability(.contentMinSize)
        .commands { MacCommands() }

        Settings {
            MacLanguageRoot { MacSettingsView() } // ⌘,(#973)
        }

        #if !APP_STORE_REVIEW // 審査用ビルドはフィードバックを外す(#319)
        // ヘルプのメニューと設定の「詳細」から開く(#983)
        Window("フィードバック", id: MacFeedbackCenter.windowId) {
            MacLanguageRoot { MacFeedbackWindow() }
        }
        .windowResizability(.contentSize)
        .defaultPosition(.center)
        #endif
    }
}
