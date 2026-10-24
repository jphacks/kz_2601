import SwiftUI
import MetamonDSL

/// `app.tabs` があれば TabView(タブ毎に独立した NavigationStack)、無ければ `initialScreen` を根にした単一 NavigationStack。
/// モーダル・FAB・オーバーレイはホスト(アプリ本体の RootView)がこの上に重ねる。
public struct DSLNavigationRoot: View {
    @Environment(\.dslRuntime) private var rt

    public init() {}

    public var body: some View {
        if rt.dsl.app.hasTabs {
            TabView(selection: Binding(get: { rt.selectedTab }, set: { rt.selectedTab = $0 })) {
                ForEach(rt.dsl.app.tabs) { tab in
                    ScreenStack(root: Route(screen: tab.screen, stack: Route.tabStack(tab.screen)))
                        .tabItem { Label(tab.title, systemImage: tab.icon ?? "square") }
                        .tag(tab.screen)
                }
            }
        } else {
            ScreenStack(root: Route(screen: rt.dsl.initialScreen))
        }
    }
}
