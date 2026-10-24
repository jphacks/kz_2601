import SwiftUI
import MetamonDSL

/// `card`: 角丸背景 + 影の 1 枚カード。任意の `title` を上に、子を縦に積む。
struct CardContainer: View {
    let c: Component
    let ctx: RenderContext

    var body: some View {
        VStack(alignment: .leading, spacing: c.containerSpacing) {
            if let title = c.title, !title.isEmpty {
                Text(title).font(RenderTheme.Fonts.cardTitle)
            }
            ForEach(c.children) { DSLView(component: $0, ctx: ctx) }
        }
        .renderCard()
    }
}
