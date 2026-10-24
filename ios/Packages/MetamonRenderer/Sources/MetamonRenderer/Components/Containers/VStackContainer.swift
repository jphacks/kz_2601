import SwiftUI
import MetamonDSL

/// `vstack`: 子を縦に積む。
struct VStackContainer: View {
    let c: Component
    let ctx: RenderContext

    var body: some View {
        VStack(alignment: .leading, spacing: c.containerSpacing) {
            if let placed = c.localGridPlacement { LocalGrid(placed: placed, ctx: ctx) } else { ForEach(c.children) { DSLView(component: $0, ctx: ctx) } }
        }
    }
}
