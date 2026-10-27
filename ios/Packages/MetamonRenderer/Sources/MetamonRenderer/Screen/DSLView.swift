import SwiftUI
import MetamonDSL

/// Component → SwiftUI View。`ComponentRegistry` を `type` 文字列で引くだけ(switch を持たない)。
/// 2段固定(Container > Leaf)なので再帰は Container 内の1段だけ。
public struct DSLView: View {
    @Environment(\.componentRegistry) private var registry
    @Environment(\.dslRuntime) private var rt
    let component: Component
    let ctx: RenderContext

    public init(component: Component, ctx: RenderContext) {
        self.component = component
        self.ctx = ctx
    }

    public var body: some View {
        if let form = ctx.form, form.hides(component, currentUid: rt.currentUid) {
            // 回答者に見せない項目(#262): 作者以外のフォームでは入力ごと出さない(値はドラフトに残り、比較の式は読める)
            EmptyView()
        } else if FormFieldOverride.applies(to: component, ctx: ctx) {
            FormFieldOverride.view(for: component, ctx: ctx)
        } else if let build = registry.builder(for: component.rawType) {
            build(component, ctx)
        } else {
            UnsupportedComponent(label: L("未対応: \(component.rawType)"))
        }
    }
}
