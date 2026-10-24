import SwiftUI
import MetamonDSL

/// `section`: 見出し + 角丸カードの中に子を縦に積む。
struct SectionContainer: View {
    let c: Component
    let ctx: RenderContext

    var body: some View {
        VStack(alignment: .leading, spacing: c.containerSpacing) {
            if let title = c.title, !title.isEmpty {
                Text(title).font(.caption).foregroundStyle(.secondary).padding(.horizontal, 4)
            }
            VStack(alignment: .leading, spacing: c.containerSpacing) {
                ForEach(c.children) { DSLView(component: $0, ctx: ctx) }
            }
            .padding(16)
            .frame(maxWidth: .infinity, alignment: .leading)
            .background(Color(.secondarySystemGroupedBackground), in: RoundedRectangle(cornerRadius: 16))
        }
    }
}
