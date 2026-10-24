import SwiftUI
import MetamonDSL

/// `section`: 見出し + 角丸カードの中に子を縦に積む。
struct SectionContainer: View {
    let c: Component
    let ctx: RenderContext

    var body: some View {
        VStack(alignment: .leading, spacing: RenderTheme.Spacing.sm) {
            if let title = c.title, !title.isEmpty {
                Text(title)
                    .font(RenderTheme.Fonts.sectionTitle)
                    .kerning(0.3)
                    .foregroundStyle(.secondary)
                    .padding(.horizontal, RenderTheme.Spacing.xs + 2)
            }
            if c.children.allSatisfy(\.rendersOwnSurface) {
                // list / table など自前の面を持つ子だけなら見出し + そのまま(カードの中にカードを作らない)。
                // セルの残り高さを子に渡す(#84: 空の list がセル全体を使えるように)
                VStack(alignment: .leading, spacing: c.containerSpacing) {
                    ForEach(c.children) { DSLView(component: $0, ctx: ctx) }
                }
                .frame(maxHeight: .infinity, alignment: .top)
            } else {
                VStack(alignment: .leading, spacing: c.containerSpacing) {
                    ForEach(c.children) { DSLView(component: $0, ctx: ctx) }
                }
                .renderCard()
            }
        }
    }
}
