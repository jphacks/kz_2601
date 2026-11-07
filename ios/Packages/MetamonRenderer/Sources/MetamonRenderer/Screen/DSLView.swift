import SwiftUI
import MetamonDSL

/// Component → SwiftUI View。`ComponentRegistry` を `type` 文字列で引くだけ(switch を持たない)。
/// 2段固定(Container > Leaf)なので再帰は Container 内の1段だけ。backend を通らない DSL でも
/// `Component.maxNestingDepth` 段より深い所は描かない(空の枠)ので、深い入れ子でスタックを使い切らない。
public struct DSLView: View {
    public static let maxNestingDepth = Component.maxNestingDepth

    @Environment(\.componentRegistry) private var registry
    @Environment(\.dslRuntime) private var rt
    @Environment(\.dslNestingDepth) private var depth
    let component: Component
    let ctx: RenderContext

    public init(component: Component, ctx: RenderContext) {
        self.component = component
        self.ctx = ctx
    }

    public var body: some View {
        if depth >= Self.maxNestingDepth {
            Color.clear
        } else {
            content
                .modifier(RequiredFieldMark(component: component, ctx: ctx))
                .environment(\.dslNestingDepth, depth + 1)
        }
    }

    @ViewBuilder private var content: some View {
        if let form = ctx.form, form.hides(component, currentUid: rt.currentUserId) {
            // 回答者に見せない項目(#262): 作者以外のフォームでは入力ごと出さない(値はドラフトに残り、比較の式は読める)
            EmptyView()
        } else if FormFieldOverride.applies(to: component, ctx: ctx) {
            FormFieldOverride.view(for: component, ctx: ctx)
        } else if let build = registry.builder(for: component.rawType) {
            build(component, ctx)
        } else {
            // 新しい版の部品・この版で外した部品(#536)。リリースでも枠と理由を出す
            UnavailableComponent(component: component)
        }
    }
}

private struct DSLNestingDepthKey: EnvironmentKey {
    static let defaultValue = 0
}

extension EnvironmentValues {
    /// 外側にある `DSLView` の数(入れ子の段数)。
    var dslNestingDepth: Int {
        get { self[DSLNestingDepthKey.self] }
        set { self[DSLNestingDepthKey.self] = newValue }
    }
}
