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
                    .kerning(0.2)
                    .foregroundStyle(.secondary)
                    .padding(.horizontal, RenderTheme.Spacing.xs)
            }
            VStack(alignment: .leading, spacing: c.containerSpacing) {
                ForEach(c.children) { DSLView(component: $0, ctx: ctx) }
            }
            .renderCard()
        }
    }
}
