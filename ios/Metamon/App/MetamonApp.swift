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
    /// 最低対応版・強制アップデート(#302)。サインイン前にも効く
    @State private var updateGate = UpdateGate()
    @Environment(\.scenePhase) private var scenePhase
    /// 起動のスプラッシュ(コールド起動の 1 回だけ・UI テストのモック起動では出さない)
    private let splashMode = SplashGate.decide()
    #if !APP_STORE_REVIEW
    @UIApplicationDelegateAdaptor(PushAppDelegate.self) private var pushDelegate // 端末トークンを受ける(#269)
    #endif

    /// Renderer 同梱のコンポーネント + アプリ本体が提供する `voiceInput`(WhisperKit 依存なので Renderer には置かない)。
    static let componentRegistry = ComponentRegistry.standard
        .registering(.voiceInput) { c, _ in VoiceInputView(prompt: c.prompt) }

    /// App Check(#318): シミュレータ・Debug 構成はデバッグ用のトークン、実機の Staging / Release / Review は App Attest。
    /// docs/infra/app-check.md
    static var appCheckMode: AppCheckMode {
        #if DEBUG || targetEnvironment(simulator)
        .debug
        #else
        .deviceAttestation
        #endif
    }

    init() {
        // Firebase は Services 組み立てより前に初期化(plist が無ければ警告してモック認証に落ちる)
        FirebaseBootstrap.configureIfAvailable(appCheck: Self.appCheckMode)
        // クラッシュ報告(#296): 設定の「クラッシュ報告を送る」を反映(Firebase が無ければ何もしない)
        CrashReportingPreference.shared.activate(reporter: FirebaseCrashReporter() ?? NoopCrashReporter())
        UNUserNotificationCenter.current().delegate = ForegroundNotificationPresenter.shared // 前面でも通知を出す(#228)
        var services = BuildConfig.make()
        // #246(5.1.2(i)): 同意が無ければ ai へ一切送らない。どの入口(Siri 等)から呼ばれても効くよう Services の ai を包む
        AIConsentDebug.applyLaunchOverride()
        FirstRunTutorialStore.shared.applyLaunchOverride() // #274(DEBUG のみ)
        CoachMarkStore.shared.applyLaunchOverride() // #1441(DEBUG のみ)
        #if !APP_STORE_REVIEW
        BetaWelcomeStore.shared.applyLaunchOverride() // #313(DEBUG のみ)
        #endif
        #if METAMON_STG
        DebugClock.applyLaunch() // 開発用メニューの日付ずらし(#2082)
        #endif
        _ = ExpensiveNetworkProbe.shared // 音声モデルの確認(#275)より前に回線の種類の監視を始める
        services.ai = ConsentGatedAIClient(base: services.ai) { AIConsentStore.isGranted(in: .standard) }
        if BuildConfig.forceMock { MusicSession.shared.player = FakeMusicPlayback() } // musicPlayer(#412)は音を出さない偽物
        if BuildConfig.forceMock { DeviceCalendar.source = FakeDeviceCalendar.sample } // 端末のカレンダー・リマインダー(#1987)は固定の中身
        if BuildConfig.forceMock { RecentlyPlayed.source = FakeRecentlyPlayed() } // 最近聞いた曲(#1988)は固定の履歴
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
        runtime.migrateTimerKeysIfNeeded() // タイマーのキーをアプリ名 → appId に(1 回だけ・部品が描かれる前)
        runtime.aiConsent = .shared
        runtime.tutorial = .shared
        runtime.coachMarks = .shared // #1441
        runtime.spotlight = SpotlightAppIndexer(index: CoreSpotlightIndex()) // #1996
        #if !APP_STORE_REVIEW
        runtime.betaWelcome = .shared // #313
        #endif
        let session = AuthSession(service: services.auth)
        runtime.onSessionExpired = { session.expire() } // 401 が続いたらサインイン画面へ(#60)
        runtime.isAnonymousUser = { session.user?.isAnonymous == true } // 引き継ぎの案内(#276)
        IntentHost.shared.register(runtime) // Siri / ショートカット(#80)は本体プロセス内でこの runtime を使う
        #if !APP_STORE_REVIEW
        PassbyController.install(runtime: runtime, mock: BuildConfig.forceMock) // すれ違い通信(#425)
        // 課金(#314): 検証済みの取引をサーバーに送り、購入の appAccountToken に uid を入れる
        ProGate.shared.store.server = services.entitlements
        ProGate.shared.store.accountUID = { [auth = services.auth] in auth.currentUser?.uid }
        PushHost.shared.install(services: services) // 端末トークンの登録(#269)
        #endif
        _runtime = State(initialValue: runtime)
        _session = State(initialValue: session)
        if splashMode != .hidden { SplashPresence.shared.begin() } // 起動時のシート・リンク・吹き出しはスプラッシュの後に
        runtime.beginLaunchConsentHoldIfNeeded() // 起動時の同意 → シェアリンク・Intent の順(シートを同時に出さない)
    }

    var body: some Scene {
        WindowGroup {
            Group {
                if session.user == nil {
                    SignInView()
                        .transition(.opacity)
                } else {
                    RootView()
                        .appLimitSheets()                           // 持てるアプリの数の置き換え(#630・下の environment を受ける)
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
                            LaunchConsentHold.shared.finishLaunchIfIdle() // 待たせる段が無ければ、以後のリンクは順番待ちにしない
                            #if !APP_STORE_REVIEW
                            SharingUI.shared.isReady = true // 保留中のシェアリンクを開く(#278)
                            #endif
                            await runtime.flushAIButtonMetrics() // #446: 前の日までの集計を送る
                        }
                        .onChange(of: scenePhase) { _, phase in
                            if phase == .active { Task { await runtime.flushAIButtonMetrics() } }
                        }
                        #if !APP_STORE_REVIEW
                        .onDisappear { SharingUI.shared.isReady = false } // サインアウト: 次のサインインの bootstrap まで待つ
                        #endif
                }
            }
            .animation(.easeInOut(duration: 0.3), value: session.user == nil)
            .modifier(SplashOverlay(mode: splashMode)) // 下の起動処理は待たずに進む
            .modifier(UpdateGateModifier(gate: updateGate))
            #if METAMON_STG
            .modifier(DebugClockBanner()) // 日付をずらしている間の印(#2082)
            #endif
            .modifier(SpotlightOpenReceiver()) // Spotlight の結果のタップ(#1996)。サインイン前でも受けて保留
            #if !APP_STORE_REVIEW
            .modifier(ShareLinkReceiver()) // シェアリンクはサインイン前でも受けて保留(#278)
            .modifier(PushRegistrationObserver(uid: session.user?.uid)) // #269
            #endif
            .environment(session)
            .modifier(AssistiveAccessReader())              // アシスティブアクセスの中か(#1997・判定はここだけ)
            .environment(\.services, services)              // protocol 越しのサービス
            // 表示言語(#44): 設定の「言語」で即時切替。`Text("…")` の解決と日付・数値の書式がこの Locale に従う
            .environment(\.locale, LanguagePreference.shared.locale)
            .task { await session.observe() }
            // クラッシュ報告に付ける本人の区別は uid だけ(名前・内容は付けない・#296)
            .onChange(of: session.user == nil) { _, signedOut in
                if signedOut { runtime.launchPresentationsAbandoned() } // サインアウト: 起動時の順番待ちを全部放す
            }
            .onChange(of: session.user?.uid, initial: true) { _, uid in CrashReportingPreference.shared.updateUser(uid: uid) }
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
