import SwiftUI
import MetamonDSL

/// `disclosure`: 折りたたみ。`title` がトグル行、`expanded` が初期状態(既定 false)。
struct DisclosureContainer: View {
    let c: Component
    let ctx: RenderContext
    @State private var isExpanded: Bool

    init(c: Component, ctx: RenderContext) {
        self.c = c
        self.ctx = ctx
        _isExpanded = State(initialValue: c.expanded ?? false)
    }

    var body: some View {
        DisclosureGroup(isExpanded: $isExpanded) {
            VStack(alignment: .leading, spacing: c.containerSpacing) {
                ForEach(c.children) { DSLView(component: $0, ctx: ctx) }
            }
            .padding(.top, RenderTheme.Spacing.sm)
        } label: {
            Text(c.title ?? "詳細").font(RenderTheme.Fonts.cardTitle)
        }
        .renderCard()
    }
}
