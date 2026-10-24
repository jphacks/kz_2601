import SwiftUI
import MetamonDSL

/// `vstack`: 子を縦に積む。
struct VStackContainer: View {
    let c: Component
    let ctx: RenderContext

    var body: some View {
        VStack(alignment: .leading, spacing: c.containerSpacing) {
            ForEach(c.children) { DSLView(component: $0, ctx: ctx) }
        }
    }
}
