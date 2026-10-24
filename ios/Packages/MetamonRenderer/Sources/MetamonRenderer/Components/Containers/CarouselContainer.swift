import SwiftUI
import MetamonDSL

/// `carousel`: 横スクロールの帯。各子は `itemWidth`(small=40% / medium=70% / large=90%)の等幅カード。
struct CarouselContainer: View {
    let c: Component
    let ctx: RenderContext

    private var widthRatio: CGFloat {
        switch c.itemWidth { case "small": 0.4; case "large": 0.9; default: 0.7 }
    }

    var body: some View {
        GeometryReader { geo in
            ScrollView(.horizontal, showsIndicators: false) {
                LazyHStack(spacing: c.containerSpacing) {
                    ForEach(c.children) { child in
                        DSLView(component: child, ctx: ctx)
                            .padding(16)
                            .frame(width: geo.size.width * widthRatio, alignment: .leading)
                            .frame(maxHeight: .infinity, alignment: .top)
                            .background(Color(.secondarySystemGroupedBackground), in: RoundedRectangle(cornerRadius: 16))
                    }
                }
                .scrollTargetLayout()
            }
            .scrollTargetBehavior(.viewAligned)
            .scrollBounceBehavior(.basedOnSize, axes: .horizontal)
        }
        .frame(minHeight: 96)
    }
}
