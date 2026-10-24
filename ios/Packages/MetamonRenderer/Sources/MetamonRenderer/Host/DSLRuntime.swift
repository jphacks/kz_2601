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
    /// サーバーから取れずローカルにも無い entity id(#85)。空状態(まだありません)と区別して「読み込めませんでした」を描く
    var failed: Set<String> { get }

    func records(for entity: String) -> [Record]
    func load(entities: Set<String>, force: Bool) async
    func run(_ action: Action, _ ctx: ActionContext) async
    /// back: スタックに積まれていれば pop、モーダルの最下層なら dismiss
    func close(_ route: Route)
    func pathBinding(_ stack: String) -> Binding<[Route]>
    /// `nearbyShare`(#36): 近くの端末を探すシートを開き、選んだ相手に `records`(entity の保存フィールドのみ)を送る。
    func shareNearby(entity: String, records: [[String: JSONValue]])
}

public extension DSLRuntime {
    func load(entities: Set<String>) async { await load(entities: entities, force: false) }
    var failed: Set<String> { [] }
    /// 既定は何もしない(プレビュー・テスト用ランタイム)。
    func shareNearby(entity: String, records: [[String: JSONValue]]) {}

    /// 部品が共通で使う空状態の判定: 読み込み中 / 取得失敗 / 本当に空。
    func emptyState(for entity: String) -> EmptyState {
        if loading.contains(entity) { return .loading }
        if failed.contains(entity) { return .unavailable }
        return .empty
    }
}

/// レコードが 0 件の時の理由。`EmptyPlaceholder` の文言・再試行ボタンを決める。
public enum EmptyState: Equatable, Sendable {
    case loading, unavailable, empty
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
