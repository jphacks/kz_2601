import SwiftUI
import MetamonDSL

/// `carousel`: 横スクロールの帯。各子は `itemWidth`(small=40% / medium=70% / large=90%)の等幅カード。
struct CarouselContainer: View {
    let c: Component
    let ctx: RenderContext

    private var widthRatio: CGFloat {
        switch c.itemWidth { case "small": 0.4; case "large": 0.9; default: 0.7 }
    }

    #if os(iOS)
    var body: some View {
        GeometryReader { geo in
            ScrollView(.horizontal, showsIndicators: false) {
                LazyHStack(spacing: c.containerSpacing) {
                    ForEach(c.children) { child in
                        DSLView(component: child, ctx: ctx)
                            .frame(maxHeight: .infinity, alignment: .top)
                            .renderCard()
                            .frame(width: geo.size.width * widthRatio, alignment: .leading)
                    }
                }
                .scrollTargetLayout()
            }
            .scrollTargetBehavior(.viewAligned)
            .scrollBounceBehavior(.basedOnSize, axes: .horizontal)
        }
        .frame(minHeight: 96)
    }
    #else
    // Mac(#940): 指でめくれないので、トラックパッドの横スクロールに加えて左右のボタン・ページの点・矢印キーで送る。
    // 「視差効果を減らす」の時は送りのアニメーションを止める(#937)。
    @Environment(\.accessibilityReduceMotion) private var reduceMotion
    @State private var scrolledPage: Int?
    @Environment(\.formTabCoordinator) private var tabs

    private var count: Int { c.children.count }
    private var page: Int { CarouselPaging.step(scrolledPage ?? 0, by: 0, count: count) }

    var body: some View {
        VStack(spacing: RenderTheme.Spacing.sm) {
            GeometryReader { geo in
                ScrollView(.horizontal, showsIndicators: false) {
                    LazyHStack(spacing: c.containerSpacing) {
                        ForEach(c.children.indices, id: \.self) { i in
                            DSLView(component: c.children[i], ctx: ctx)
                                .frame(maxHeight: .infinity, alignment: .top)
                                .renderCard()
                                .frame(width: geo.size.width * widthRatio, alignment: .leading)
                        }
                    }
                    .scrollTargetLayout()
                }
                .scrollTargetBehavior(.viewAligned)
                .scrollBounceBehavior(.basedOnSize, axes: .horizontal)
                .scrollPosition(id: $scrolledPage, anchor: .leading)
            }
            .frame(minHeight: 96)
            if CarouselPaging.showsControls(count: count) {
                pager
            }
        }
        // Tab の渡し先が別のカードの中なら、そのカードへ送る(出ていなければ出てから止まる・#2065)
        .onChange(of: tabs?.request) { _, request in
            guard let request, let i = c.children.firstIndex(where: { FormTabOrder.contains(request.id, in: $0) }), i != page else { return }
            scroll(to: i)
        }
        .onKeyPress(.leftArrow) { go(-1) }
        .onKeyPress(.rightArrow) { go(1) }
        .accessibilityElement(children: .contain)
        .accessibilityLabel(c.title ?? L("カルーセル"))
    }

    /// 前へ・ページの点・次へ。点は Tab で止まり、矢印キーと VoiceOver の増減で送れる。
    private var pager: some View {
        HStack(spacing: RenderTheme.Spacing.md) {
            Button { _ = go(-1) } label: { Image(systemName: "chevron.left") }
                .iconButtonLabel(L("前へ"))
                .help(L("前へ")) // iPad のポインタ用に iOS でも残す
                .disabled(!CarouselPaging.canGoBack(page, count: count))
            HStack(spacing: 6) {
                ForEach(0..<count, id: \.self) { i in
                    Circle()
                        .fill(i == page ? AnyShapeStyle(RenderTheme.Colors.brandA) : AnyShapeStyle(.quaternary))
                        .frame(width: 7, height: 7)
                        .contentShape(Rectangle().inset(by: -4))
                        .onTapGesture { scroll(to: i) }
                        .macHelp(L("\(i + 1) 枚目を見る"))
                }
            }
            .focusable()
            .accessibilityElement(children: .ignore)
            .accessibilityLabel(L("ページ"))
            .accessibilityValue(CarouselPaging.positionText(page, count: count))
            .accessibilityAdjustableAction { direction in
                switch direction {
                case .increment: _ = go(1)
                case .decrement: _ = go(-1)
                @unknown default: break
                }
            }
            Button { _ = go(1) } label: { Image(systemName: "chevron.right") }
                .iconButtonLabel(L("次へ"))
                .help(L("次へ"))
                .disabled(!CarouselPaging.canGoForward(page, count: count))
        }
        .buttonStyle(.borderless)
        .font(.callout.weight(.semibold))
        .frame(maxWidth: .infinity)
    }

    private func go(_ delta: Int) -> KeyPress.Result {
        let next = CarouselPaging.step(page, by: delta, count: count)
        guard next != page else { return .ignored }
        scroll(to: next)
        return .handled
    }

    private func scroll(to i: Int) {
        withAnimation(reduceMotion ? nil : RenderTheme.Motion.select) { scrolledPage = i }
    }
    #endif
}
