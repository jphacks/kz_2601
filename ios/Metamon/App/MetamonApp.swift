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
        var document = (try? DSLExamples.document(DSLExamples.initial)) ?? .fallback
        document.appId = DeviceAppID.current
        self.services = services
        _runtime = State(initialValue: AppRuntime(document: document, services: services))
        _session = State(initialValue: AuthSession(service: services.auth))
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
                        .task { await runtime.bootstrap() }
                }
            }
            .animation(.easeInOut(duration: 0.3), value: session.user == nil)
            .environment(session)
            .environment(\.services, services)              // protocol 越しのサービス
            .task { await session.observe() }
        }
    }
}
