import SwiftUI
import UserNotifications
import MetamonDSL
import MetamonRenderer
import MetamonServices

@main
struct MetamonApp: App {
    /// 起動時に1回だけ組み立てる(DI の入口は BuildConfig)。
    private let services: Services
    @State private var runtime: AppRuntime
    @State private var session: AuthSession
    @Environment(\.scenePhase) private var scenePhase

    /// Renderer 同梱のコンポーネント + アプリ本体が提供する `voiceInput`(WhisperKit 依存なので Renderer には置かない)。
    static let componentRegistry = ComponentRegistry.standard
        .registering(.voiceInput) { c, _ in VoiceInputView(prompt: c.prompt) }

    init() {
        // Firebase は Services 組み立てより前に初期化(plist が無ければ警告してモック認証に落ちる)
        FirebaseBootstrap.configureIfAvailable()
        UNUserNotificationCenter.current().delegate = ForegroundNotificationPresenter.shared // 前面でも通知を出す(#228)
        var services = BuildConfig.make()
        // #246(5.1.2(i)): 同意が無ければ ai へ一切送らない。どの入口(Siri 等)から呼ばれても効くよう Services の ai を包む
        AIConsentDebug.applyLaunchOverride()
        FirstRunTutorialStore.shared.applyLaunchOverride() // #274(DEBUG のみ)
        _ = ExpensiveNetworkProbe.shared // 音声モデルの確認(#275)より前に回線の種類の監視を始める
        services.ai = ConsentGatedAIClient(base: services.ai) { AIConsentStore.isGranted(in: .standard) }
        if BuildConfig.forceMock { MusicSession.shared.player = FakeMusicPlayback() } // musicPlayer(#412)は音を出さない偽物
        #if !APP_STORE_REVIEW
        if BuildConfig.forceMock { HealthData.source = MockHealthData() } // healthStat(#167)は固定値
        if BuildConfig.forceMock { SensorData.source = MockSensorData() } // sensorValue(#419)は決まった波形
        #else
        _ = ReviewBuildPackageMarker.self // パッケージにも APP_STORE_REVIEW が渡っていることをコンパイル時に保証(#245)
        #endif
        let appId = DeviceAppID.current
        var document: AppDocument
        if services.isLive, let cached = DocumentCache.shared.load(appId: appId) {
            // 最後に取得した DSL を即描画(G4: 再起動しても作ったアプリが残る)。サーバーの最新は bootstrap で取り直す
            document = cached
        } else {
            document = (try? DSLExamples.document(DSLExamples.initial, version: 0)) ?? .fallback
        }
        document.appId = appId
        #if DEBUG
        if let override = DebugDSLOverride.document { document = override } // 見た目監査(#131)
        #endif
        self.services = services
        let runtime = AppRuntime(document: document, services: services)
        runtime.aiConsent = .shared
        runtime.tutorial = .shared
        let session = AuthSession(service: services.auth)
        runtime.onSessionExpired = { session.expire() } // 401 が続いたらサインイン画面へ(#60)
        runtime.isAnonymousUser = { session.user?.isAnonymous == true } // 引き継ぎの案内(#276)
        IntentHost.shared.register(runtime) // Siri / ショートカット(#80)は本体プロセス内でこの runtime を使う
        #if !APP_STORE_REVIEW
        PassbyController.install(runtime: runtime, mock: BuildConfig.forceMock) // すれ違い通信(#425)
        #endif
        _runtime = State(initialValue: runtime)
        _session = State(initialValue: session)
    }

    var body: some Scene {
        WindowGroup {
            Group {
                if session.user == nil {
                    SignInView()
                        .transition(.opacity)
                } else {
                    RootView()
                        .environment(runtime)                       // アプリ本体の View(RootView / VoiceInputView)向け
                        .environment(\.dslRuntime, runtime)         // Renderer 向け(DSLRuntime protocol)
                        .environment(\.componentRegistry, Self.componentRegistry)
                        #if !APP_STORE_REVIEW // 審査用ビルドは App 内課金を持たない(#450 G1)
                        .modifier(PaywallHost())                    // 課金(#77): 上限シート + StoreKit の監視開始
                        #endif
                        .transition(.opacity)
                        .task {
                            await runtime.bootstrap()
                            IntentHost.shared.markReady() // 保留中の「開く/話しかける」を実行(#80)
                        }
                }
            }
            .animation(.easeInOut(duration: 0.3), value: session.user == nil)
            .environment(session)
            .environment(\.services, services)              // protocol 越しのサービス
            // 表示言語(#44): 設定の「言語」で即時切替。`Text("…")` の解決と日付・数値の書式がこの Locale に従う
            .environment(\.locale, LanguagePreference.shared.locale)
            .task { await session.observe() }
            .onChange(of: scenePhase) { _, phase in
                if phase == .background { BackgroundRefresh.schedule() } // #38: 裏の定期同期を予約
            }
        }
        .backgroundTask(.appRefresh(BackgroundRefresh.identifier)) { [runtime] in
            await MainActor.run { BackgroundRefresh.schedule() } // 次回分を先に積む(連鎖させて定期化)
            await runtime.backgroundRefresh()
        }
    }
}
