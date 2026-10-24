import SwiftUI

@main
struct MorphApp: App {
    @State private var runtime: AppRuntime

    init() {
        let services = BuildConfig.makeServices()
        let document = (try? AppDocument.bundled("initial")) ?? .fallback
        _runtime = State(initialValue: AppRuntime(document: document, services: services))
    }

    var body: some Scene {
        WindowGroup {
            RootView()
                .environment(runtime)
                .task { await runtime.bootstrap() }
        }
    }
}
