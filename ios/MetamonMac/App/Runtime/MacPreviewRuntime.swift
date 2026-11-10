import SwiftUI
import Observation
import MetamonDSL
import MetamonRenderer

/// Mac の殻が 1 つのアプリを描くための最小のランタイム(#933)。
/// 画面の移動(navigate / back・sheet / fullScreen)は iOS と同じ `NavigationResolver` で動かす。
/// 記録は持たない(保存・同期・AI・通知はサービスの Mac 対応で iOS の `AppRuntime` 相当に置き換える)。
@MainActor
@Observable
final class MacPreviewRuntime: DSLRuntime {
    let dsl: AppDSL
    let appId: String
    var paths: [String: [Route]] = [:]
    var selectedTab: String
    let loading: Set<String> = []
    var sheet: Route?
    var fullScreen: Route?

    init(appId: String, dsl: AppDSL) {
        self.appId = appId
        self.dsl = dsl
        selectedTab = dsl.app.tabs.first?.screen ?? ""
    }

    func records(for entity: String) -> [Record] { [] }
    func load(entities: Set<String>, force: Bool) async {}

    func run(_ action: Action, _ ctx: ActionContext) async {
        switch action.type {
        case .navigate:
            guard let screen = action.screen else { return }
            // prefillRef(親から子を作る)は子を新規で開くので recordId を渡さない(iOS と同じ)
            let recordId = action.prefillRef == nil ? ctx.recordId : nil
            apply(NavigationResolver.navigate(state, to: screen, presentation: action.presentation,
                                              recordId: recordId, from: ctx.route, dsl: dsl))
        case .back:
            close(ctx.route)
        default:
            break
        }
    }

    func close(_ route: Route) { apply(NavigationResolver.close(state, route)) }

    func pathBinding(_ stack: String) -> Binding<[Route]> {
        Binding(get: { self.paths[stack] ?? [] }, set: { self.paths[stack] = $0 })
    }

    private var state: NavigationState {
        NavigationState(paths: paths, selectedTab: selectedTab, sheet: sheet, fullScreen: fullScreen)
    }

    /// 変わったものだけ書き戻す(iOS の `AppRuntime.setNavigation` と同じ理由)。
    private func apply(_ next: NavigationState) {
        if next.sheet != sheet { sheet = next.sheet }
        if next.fullScreen != fullScreen { fullScreen = next.fullScreen }
        if next.selectedTab != selectedTab { selectedTab = next.selectedTab }
        if next.paths != paths { paths = next.paths }
    }
}
