import SwiftUI
import MetamonDSL

/// `hstack`: 子を横に等幅で並べる。
struct HStackContainer: View {
    let c: Component
    let ctx: RenderContext

    var body: some View {
        HStack(alignment: .top, spacing: c.containerSpacing) {
            ForEach(c.children) { DSLView(component: $0, ctx: ctx).frame(maxWidth: .infinity) }
        }
    }
}
