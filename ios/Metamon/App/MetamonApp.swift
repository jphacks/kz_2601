import SwiftUI
import MetamonDSL
import MetamonRenderer
import MetamonServices

@main
struct MetamonApp: App {
    /// 起動時に1回だけ組み立てる(DI の入口は BuildConfig)。
    private let services: Services
    @State private var runtime: AppRuntime

    /// Renderer 同梱のコンポーネント + アプリ本体が提供する `voiceInput`(WhisperKit 依存なので Renderer には置かない)。
    static let componentRegistry = ComponentRegistry.standard
        .registering(.voiceInput) { c, _ in VoiceInputView(prompt: c.prompt) }

    init() {
        let services = BuildConfig.make()
        let document = (try? DSLExamples.document(DSLExamples.initial)) ?? .fallback
        self.services = services
        _runtime = State(initialValue: AppRuntime(document: document, services: services))
    }

    var body: some Scene {
        WindowGroup {
            RootView()
                .environment(runtime)                       // アプリ本体の View(RootView / VoiceInputView)向け
                .environment(\.dslRuntime, runtime)         // Renderer 向け(DSLRuntime protocol)
                .environment(\.services, services)          // protocol 越しのサービス
                .environment(\.componentRegistry, Self.componentRegistry)
                .task { await runtime.bootstrap() }
        }
    }
}
