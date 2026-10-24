import SwiftUI
import Observation
import MetamonDSL

/// Renderer がホスト(アプリ本体の `AppRuntime`)に要求する能力。Renderer は Services を知らず、この protocol 越しにだけ副作用を起こす。
/// `@Observable` な実体を `.environment(\.dslRuntime, runtime)` で注入する(プロパティアクセスは SwiftUI が追跡する)。
@MainActor
public protocol DSLRuntime: AnyObject, Observable {
    var dsl: AppDSL { get }
    /// スタック名(main / tab:xxx / sheet / full)→ push 済み Route
    var paths: [String: [Route]] { get }
    /// タブ表示時に選択中のタブ(= tab.screen)
    var selectedTab: String { get set }
    /// 読み込み中の entity id
    var loading: Set<String> { get }

    func records(for entity: String) -> [Record]
    func load(entities: Set<String>, force: Bool) async
    func run(_ action: Action, _ ctx: ActionContext) async
    /// back: スタックに積まれていれば pop、モーダルの最下層なら dismiss
    func close(_ route: Route)
    func pathBinding(_ stack: String) -> Binding<[Route]>
}

public extension DSLRuntime {
    func load(entities: Set<String>) async { await load(entities: entities, force: false) }
}

private struct DSLRuntimeKey: EnvironmentKey {
    static let defaultValue: (any DSLRuntime)? = nil
}

/// 未注入時のフォールバック(何もしない・`AppDSL.fallback` を描く)。
/// `.environment(\.dslRuntime, x)` は WritableKeyPath 経由で getter を呼んでから setter を呼ぶため、getter で落とすと注入自体がクラッシュする。
@MainActor
@Observable
final class UnboundDSLRuntime: DSLRuntime {
    static let shared = UnboundDSLRuntime()

    var dsl: AppDSL = .fallback
    var paths: [String: [Route]] = [:]
    var selectedTab = ""
    var loading: Set<String> = []

    func records(for entity: String) -> [Record] { [] }
    func load(entities: Set<String>, force: Bool) async {}
    func run(_ action: Action, _ ctx: ActionContext) async {}
    func close(_ route: Route) {}
    func pathBinding(_ stack: String) -> Binding<[Route]> { .constant([]) }
}

public extension EnvironmentValues {
    /// Renderer の全 View が参照するランタイム。アプリ本体が `.environment(\.dslRuntime, runtime)` で注入する。
    var dslRuntime: any DSLRuntime {
        get { self[DSLRuntimeKey.self] ?? MainActor.assumeIsolated { UnboundDSLRuntime.shared } }
        set { self[DSLRuntimeKey.self] = newValue }
    }
}
