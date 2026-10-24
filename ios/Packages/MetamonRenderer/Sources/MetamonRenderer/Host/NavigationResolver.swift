import Foundation
import MetamonDSL

/// ナビゲーションの状態(ホストの `paths` / `selectedTab` / `sheet` / `fullScreen` の写し)。
public struct NavigationState: Equatable, Sendable {
    /// スタック名(main / tab:xxx / sheet / full)→ 根の上に push 済みの Route
    public var paths: [String: [Route]]
    public var selectedTab: String
    public var sheet: Route?
    public var fullScreen: Route?

    public init(paths: [String: [Route]] = [:], selectedTab: String = "", sheet: Route? = nil, fullScreen: Route? = nil) {
        self.paths = paths
        self.selectedTab = selectedTab
        self.sheet = sheet
        self.fullScreen = fullScreen
    }
}

/// `navigate` の行き先を「今どこにいるか」で解決する純粋関数(#113)。DSL を変えずに次の規則で動く:
/// 1. 行き先が今のスタック(根を含む)にある → そこまで pop(積み直さない)
/// 2. 行き先がタブの根 → そのタブへ切り替え、そのタブのスタックを根まで戻す
/// 3. それ以外 → 今のスタックへ push
/// モーダル(sheet / full)の中では、まずモーダル内のスタックで 1 を見て、無ければ下の画面(選択中タブ / main)で 1〜2 を見る。
/// 当たればモーダルを閉じてから適用、どれにも当たらなければ従来どおりモーダル内に push。
/// `presentation` が sheet / fullScreen なら従来どおりモーダルで開く(規則は使わない)。
/// recordId: 根は recordId を持たないので画面 id だけで一致。push 済みの画面は screen と recordId が両方一致した時だけ同じとみなす
/// (別レコードの編集画面は別物として push する)。
public enum NavigationResolver {
    static let modalStacks: Set<String> = ["sheet", "full"]

    public static func navigate(_ state: NavigationState, to screen: String, presentation: Presentation?,
                                recordId: String?, from route: Route, dsl: AppDSL) -> NavigationState {
        var s = state
        switch presentation ?? .push {
        case .sheet:
            s.paths["sheet"] = nil // 新しいモーダルは根から始める
            s.sheet = Route(screen: screen, recordId: recordId, stack: "sheet")
            return s
        case .fullScreen:
            s.paths["full"] = nil
            s.fullScreen = Route(screen: screen, recordId: recordId, stack: "full")
            return s
        case .push:
            break
        }

        if modalStacks.contains(route.stack) {
            let modalRoot = route.stack == "sheet" ? s.sheet : s.fullScreen
            if popTo(screen, recordId: recordId, root: modalRoot, in: &s) { return s }
            var under = dismissModals(s)
            if popTo(screen, recordId: recordId, root: mainRoot(state: under, dsl: dsl), in: &under)
                || switchTab(to: screen, in: &under, dsl: dsl) {
                return under
            }
        } else {
            let root = rootRoute(of: route.stack, dsl: dsl)
            if popTo(screen, recordId: recordId, root: root, in: &s) || switchTab(to: screen, in: &s, dsl: dsl) { return s }
        }
        s.paths[route.stack, default: []].append(Route(screen: screen, recordId: recordId, stack: route.stack))
        return s
    }

    /// back: スタックに積まれていれば pop、モーダルの最下層なら dismiss(タブ / main の根は閉じない)。
    public static func close(_ state: NavigationState, _ route: Route) -> NavigationState {
        var s = state
        if !(s.paths[route.stack] ?? []).isEmpty {
            s.paths[route.stack]?.removeLast()
            return s
        }
        switch route.stack {
        case "sheet": s.sheet = nil; s.paths["sheet"] = nil
        case "full": s.fullScreen = nil; s.paths["full"] = nil
        default: break
        }
        return s
    }

    static func dismissModals(_ state: NavigationState) -> NavigationState {
        var s = state
        s.sheet = nil
        s.fullScreen = nil
        s.paths["sheet"] = nil
        s.paths["full"] = nil
        return s
    }

    /// モーダルの下に見えている画面(タブがあれば選択中タブ、無ければ main)の根。
    static func mainRoot(state: NavigationState, dsl: AppDSL) -> Route {
        if dsl.app.hasTabs {
            let tab = state.selectedTab.isEmpty ? dsl.initialTab : state.selectedTab
            return Route(screen: tab, stack: Route.tabStack(tab))
        }
        return Route(screen: dsl.initialScreen)
    }

    static func rootRoute(of stack: String, dsl: AppDSL) -> Route? {
        if stack.hasPrefix("tab:") { return Route(screen: String(stack.dropFirst(4)), stack: stack) }
        if stack == "main" { return Route(screen: dsl.initialScreen) }
        return nil
    }

    /// `root` を根とするスタックに行き先があれば、そこまで pop して true。
    private static func popTo(_ screen: String, recordId: String?, root: Route?, in s: inout NavigationState) -> Bool {
        guard let root else { return false }
        if root.screen == screen {
            s.paths[root.stack] = []
            return true
        }
        let path = s.paths[root.stack] ?? []
        guard let i = path.lastIndex(where: { $0.screen == screen && $0.recordId == recordId }) else { return false }
        s.paths[root.stack] = Array(path[...i])
        return true
    }

    /// 行き先がタブの根なら、そのタブを選び、そのスタックを根まで戻して true。
    private static func switchTab(to screen: String, in s: inout NavigationState, dsl: AppDSL) -> Bool {
        guard dsl.app.hasTabs, dsl.app.tabs.contains(where: { $0.screen == screen }) else { return false }
        s.selectedTab = screen
        s.paths[Route.tabStack(screen)] = []
        return true
    }
}
