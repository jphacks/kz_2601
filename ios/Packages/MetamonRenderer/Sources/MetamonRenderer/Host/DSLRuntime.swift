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

public extension EnvironmentValues {
    /// Renderer の全 View が参照するランタイム。未注入なら `MissingDSLRuntime`(空描画・副作用 no-op)。
    /// getter で precondition してはいけない: `.environment(\.dslRuntime, rt)` の書き込みは WritableKeyPath 経由で
    /// **先に getter を呼ぶ**ため、注入そのものが起動時クラッシュになる(SIGTRAP in EnvironmentValues.dslRuntime.getter)。
    var dslRuntime: any DSLRuntime {
        get { self[DSLRuntimeKey.self] ?? MissingDSLRuntime.shared }
        set { self[DSLRuntimeKey.self] = newValue }
    }
}

/// DI 漏れ時のフォールバック。何も描けず何もしない(DEBUG では初回アクセスをログに出す)。
@MainActor @Observable
public final class MissingDSLRuntime: DSLRuntime {
    public static let shared = MissingDSLRuntime()

    public let dsl: AppDSL = .fallback
    public let paths: [String: [Route]] = [:]
    public var selectedTab: String = ""
    public let loading: Set<String> = []

    private init() {}

    public func records(for entity: String) -> [Record] { warn(); return [] }
    public func load(entities: Set<String>, force: Bool) async { warn() }
    public func run(_ action: Action, _ ctx: ActionContext) async { warn() }
    public func close(_ route: Route) { warn() }
    public func pathBinding(_ stack: String) -> Binding<[Route]> { warn(); return .constant([]) }

    private func warn() {
        #if DEBUG
        print("[MetamonRenderer] DSLRuntime が未注入です。.environment(\\.dslRuntime, runtime) を付けてください")
        #endif
    }
}
