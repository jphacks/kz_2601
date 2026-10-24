import SwiftUI
import MetamonDSL
import MetamonServices

@main
struct MetamonApp: App {
    /// 起動時に1回だけ組み立てる(DI の入口は BuildConfig)。
    private let services: Services
    @State private var runtime: AppRuntime

    init() {
        let services = BuildConfig.make()
        let document = (try? DSLExamples.document(DSLExamples.initial)) ?? .fallback
        self.services = services
        _runtime = State(initialValue: AppRuntime(document: document, services: services))
    }

    var body: some Scene {
        WindowGroup {
            RootView()
                .environment(runtime)
                .environment(\.services, services)
                .task { await runtime.bootstrap() }
        }
    }
}
