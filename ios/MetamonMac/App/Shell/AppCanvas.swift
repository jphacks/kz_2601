import SwiftUI
import MetamonDSL
import MetamonRenderer

/// 中央: 選んだアプリを Renderer で描く。タブ / 単一スタックは `DSLNavigationRoot`、sheet / fullScreen はここで重ねる。
/// アプリを切り替えたら呼び出し側の `.id(app.id)` でランタイムごと作り直す。
struct AppCanvas: View {
    @State private var runtime: MacPreviewRuntime?
    @State private var loadFailed = false
    let app: SampleApp

    var body: some View {
        Group {
            if let runtime {
                AppCanvasContent(runtime: runtime)
            } else if loadFailed {
                ContentUnavailableView("アプリを開けませんでした", systemImage: "exclamationmark.triangle")
            } else {
                ProgressView()
            }
        }
        .navigationTitle(app.name)
        .task {
            do {
                runtime = MacPreviewRuntime(appId: app.id, dsl: try app.loadDSL())
            } catch {
                loadFailed = true
            }
        }
    }
}

private struct AppCanvasContent: View {
    @Bindable var runtime: MacPreviewRuntime

    var body: some View {
        DSLNavigationRoot()
            // Mac の fullScreenCover は無いので sheet で出す(大きめの最小サイズ)
            .sheet(item: $runtime.sheet) { route in
                modal(route).frame(minWidth: 420, minHeight: 480)
            }
            .sheet(item: $runtime.fullScreen) { route in
                modal(route).frame(minWidth: 640, minHeight: 560)
            }
            .environment(\.dslRuntime, runtime)
            .environment(\.componentRegistry, ComponentRegistry.standard)
    }

    private func modal(_ route: Route) -> some View {
        ScreenStack(root: route)
            .environment(\.dslRuntime, runtime)
            .environment(\.componentRegistry, ComponentRegistry.standard)
    }
}
