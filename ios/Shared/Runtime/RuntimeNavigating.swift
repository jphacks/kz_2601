import SwiftUI
import MetamonDSL
import MetamonRenderer

/// 画面の移動(navigate / back・sheet / fullScreen)。行き先は `NavigationResolver` が今いる場所で解決する(#113)。
/// iOS の `AppRuntime` と Mac の `MacAppRuntime` が同じ実装を使う。
@MainActor
protocol RuntimeNavigating: DSLRuntime {
    var paths: [String: [Route]] { get set }
    var selectedTab: String { get set }
    var sheet: Route? { get set }
    var fullScreen: Route? { get set }
}

extension RuntimeNavigating {
    var navigationState: NavigationState {
        NavigationState(paths: paths, selectedTab: selectedTab, sheet: sheet, fullScreen: fullScreen)
    }

    /// 今のスタックにあれば pop・タブの根ならタブ切替・モーダル内から下の画面へ飛ぶならモーダルを閉じる・それ以外は push。
    func navigate(to screen: String, presentation: Presentation?, recordId: String?, from route: Route) {
        setNavigation(NavigationResolver.navigate(navigationState, to: screen, presentation: presentation,
                                                  recordId: recordId, from: route, dsl: dsl))
    }

    /// back: スタックに積まれていれば pop、モーダルの最下層なら dismiss。
    func close(_ route: Route) {
        setNavigation(NavigationResolver.close(navigationState, route))
    }

    func pathBinding(_ stack: String) -> Binding<[Route]> {
        Binding(get: { self.paths[stack] ?? [] }, set: { self.paths[stack] = $0 })
    }

    /// 変わったものだけ書き戻す(@Observable の無駄な再描画・NavigationStack の再設定を避ける)。
    func setNavigation(_ next: NavigationState) {
        if next.sheet != sheet { sheet = next.sheet }
        if next.fullScreen != fullScreen { fullScreen = next.fullScreen }
        if next.selectedTab != selectedTab { selectedTab = next.selectedTab }
        if next.paths != paths { paths = next.paths }
    }
}
