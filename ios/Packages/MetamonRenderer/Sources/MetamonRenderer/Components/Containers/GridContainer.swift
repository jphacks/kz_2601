import SwiftUI
import MetamonDSL

/// `grid`: 等幅 `columns`(2..4・既定 2)列のタイル。stat / button / image などの小部品向け。
struct GridContainer: View {
    let c: Component
    let ctx: RenderContext

    private var columns: [GridItem] {
        let n = min(max(c.columnCount ?? 2, 2), 4)
        return Array(repeating: GridItem(.flexible(), spacing: c.containerSpacing, alignment: .top), count: n)
    }

    var body: some View {
        LazyVGrid(columns: columns, alignment: .leading, spacing: c.containerSpacing) {
            ForEach(c.children) { DSLView(component: $0, ctx: ctx) }
        }
    }
}
