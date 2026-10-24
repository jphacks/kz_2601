import SwiftUI
import MetamonDSL
import MetamonRenderer
import MetamonServices

@main
struct MetamonApp: App {
    /// 起動時に1回だけ組み立てる(DI の入口は BuildConfig)。
    private let services: Services
    @State private var runtime: AppRuntime
    @State private var session: AuthSession

    /// Renderer 同梱のコンポーネント + アプリ本体が提供する `voiceInput`(WhisperKit 依存なので Renderer には置かない)。
    static let componentRegistry = ComponentRegistry.standard
        .registering(.voiceInput) { c, _ in VoiceInputView(prompt: c.prompt) }

    init() {
        // Firebase は Services 組み立てより前に初期化(plist が無ければ警告してモック認証に落ちる)
        FirebaseBootstrap.configureIfAvailable()
        let services = BuildConfig.make()
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
        let session = AuthSession(service: services.auth)
        runtime.onSessionExpired = { session.expire() } // 401 が続いたらサインイン画面へ(#60)
        IntentHost.shared.register(runtime) // Siri / ショートカット(#80)は本体プロセス内でこの runtime を使う
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
            .task { await session.observe() }
        }
    }
}
