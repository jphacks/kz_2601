import SwiftUI
import AppKit
import MetamonDSL
import MetamonRenderer
import MetamonServices

/// Mac の殻(#929)。画面の骨格は #933(Shell/)、メニューとキーボードは #934(MacCommands)。
/// Firebase(ログイン・App Check・クラッシュ報告)は #930(`MacServices`・`MacAccount`)、権限の説明は #931。
@main
struct MetamonMacApp: App {
    /// ⌘Q の前に保存していない入力を確かめる(#943)
    @NSApplicationDelegateAdaptor(MacAppDelegate.self) private var appDelegate
    private let apps = SampleAppCatalog.load()
    /// 声で頼む(#966)。マイクは 1 つなのでウインドウをまたいで 1 つだけ持つ
    @State private var voice = MacVoiceInput()
    /// 作ったアプリの一覧(#969)。端末に残すのでウインドウをまたいで 1 つだけ持つ
    /// サインインしたらサーバーの自分のアプリも混ぜる(#1057)。前回のサーバーの一覧(`AppListCache`)を起動直後に出す
    @State private var library: MacAppLibrary

    init() {
        let library = MacAppLibrary(defaults: .standard, server: MacServices.shared.appConfig, serverCache: .shared)
        _library = State(initialValue: library)
        // Firebase は接続(MacServices)を組み立てる前に初期化(plist が無ければ何もせず、ログインと AI はモック・#930)
        MacFirebase.bootstrap()
        Task { await MacAccount.shared.start() } // 未ログインなら匿名でログインし、状態の変化を追う
        MacNotificationDelegate.install() // 前面でも通知を出す・押されたらそのアプリを開く(#965)
        MacRecordResend.shared.install() // 記録の送信待ちを前面・スリープ明け・回線の復帰・一定間隔で送り直す(#976)
        // ショートカット・Siri・Spotlight(App Intents・#1034)。記録の追加はウインドウを開かずに来るので、一覧は起動の最初に渡す
        IntentHost.shared.register(library: library, bundled: apps)
        // UI テスト・手元の確認用(DEBUG のみ・iOS と同じ環境変数): METAMON_AI_CONSENT / METAMON_TUTORIAL / METAMON_BETA_WELCOME(#971)
        AIConsentDebug.applyLaunchOverride()
        FirstRunTutorialStore.shared.applyLaunchOverride()
        #if !APP_STORE_REVIEW
        BetaWelcomeStore.shared.applyLaunchOverride()
        MacAIButtonMetrics.install() // 前の日までの AI ボタンの集計を、起動・前面に戻った時に送る(#446・#1327)
        MacPushHost.shared.install(services: MacServices.shared) // プッシュの端末の登録(#982)
        #endif
        // 記録の一時フォルダ(MacAppRuntime.mockRecordSync)と共有に渡した書き出しの写し(#946)を起動時(前回の残り)と終了時に片付ける
        MacAppRuntime.removePreviewDirectories()
        ExportFiles.removeAll()
        MacCameraFiles.removeAll() // 録画の一時ファイル(前回の残り)
        MacCameraFiles.preferredMicrophoneUID = { MacAudioInput.selectedDevice()?.uid } // 録画も設定で選んだマイクで(#1221)
        NotificationCenter.default.addObserver(forName: NSApplication.willTerminateNotification, object: nil, queue: .main) { _ in
            MainActor.assumeIsolated {
                MacAppRuntime.removePreviewDirectories()
                ExportFiles.removeAll()
            }
        }
        // musicPlayer(#960): 既定は MusicKit。METAMON_MOCK=1(iOS と同じ環境変数)の時だけ音を出さない偽物
        if ProcessInfo.processInfo.environment["METAMON_MOCK"] == "1" { MusicSession.shared.player = FakeMusicPlayback() }
    }

    var body: some Scene {
        WindowGroup(id: MacCommands.mainWindowId) { root(window: nil) }
            .defaultSize(width: 1100, height: 720)
            .windowResizability(.contentMinSize)
            .commands { MacCommands() }

        // 生成アプリごとのウインドウ(#1853)。サイドバーの右クリック・⌥クリックで開く。同じアプリなら開いているものが前に出る
        WindowGroup("アプリ", id: MacAppWindows.windowId, for: AppID.self) { $app in root(window: app) }
            .defaultSize(width: 900, height: 680)
            .windowResizability(.contentMinSize)

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

    /// どのウインドウも同じ殻(`MacRootView`)。アプリのウインドウ(#1853)は開くアプリを渡す
    private func root(window: AppID?) -> some View {
        // 設定の「言語」で即時切替(#973)・初回の案内(#971)・声で頼む(#966)・tailorit:// と共有のリンク(#975)
        MacLanguageRoot {
            MacRootView(apps: apps, window: window)
                .macOnboarding()
                .environment(voice)
                .environment(library)
                #if !APP_STORE_REVIEW
                .modifier(MacLinkReceiver())
                .modifier(MacPushRegistrationObserver()) // プッシュの端末の登録(#982)
                #else
                .modifier(MacWidgetLinkReceiver()) // 審査用の版は共有のリンクを外すので、ウィジェットの URL(#984)だけ受ける
                #endif
                .modifier(MacCrashReportUser())
                .modifier(MacStatusItemInstaller()) // メニューバーの常駐(#1848・設定で ON の時だけ。1 つだけ作る)
                .task { MacAccount.shared.wipe.library = library } // サインアウト・削除で一覧も空にする(#972)
                .task {
                    // マイクの抜き差し(#1221): 録音中に使っている装置が外れたら止めて知らせる
                    MacInputDevices.shared.onChange = { [voice] in voice.inputDevicesChanged(available: $0) }
                    voice.attachAudioFocus() // 録音と再生は同時に 1 つだけ(#952)
                    MacInputDevices.shared.startObserving()
                }
        }
    }
}
