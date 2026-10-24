import SwiftUI
import MetamonDSL

/// 画面グリッド配置。
/// - 横 6 列(A-F)= 可視幅 ÷ 6、縦 1 行 = 可視高さ ÷ 12。`area` を持つ直下の子は矩形に絶対配置(セル内でスクロール)。
/// - `area` の無い子は「area で使われた最終行の次の行」から全幅で上から積む(フォールバック)。
/// - 13 行目以降 or フォールバックがはみ出せば画面全体が縦スクロール。voiceInput 画面(初期状態)はスクロールしない。
/// - 見た目: セル間ギャップ / 画面端余白は `RenderTheme.Spacing`、出現は index × stagger の遅延付きフェード(変身の受け側)。
public struct GridScreenLayout: View {
    let screen: Screen
    let ctx: RenderContext

    private let gap = RenderTheme.Spacing.gridGap
    private let inset = RenderTheme.Spacing.screenInset

    public init(screen: Screen, ctx: RenderContext) {
        self.screen = screen
        self.ctx = ctx
    }

    public var body: some View {
        GeometryReader { geo in
            let cellW = (geo.size.width - inset * 2) / CGFloat(GridArea.columns)
            let cellH = (geo.size.height - inset * 2) / CGFloat(GridArea.visibleRows)
            let placement = screen.gridPlacement
            let lastRow = placement.placed.map(\.area.row2).max() ?? 0
            let gridHeight = CGFloat(lastRow) * cellH

            let content = VStack(alignment: .leading, spacing: gap) {
                if !placement.placed.isEmpty {
                    ZStack(alignment: .topLeading) {
                        Color.clear.frame(width: geo.size.width - inset * 2, height: gridHeight)
                        ForEach(Array(placement.placed.enumerated()), id: \.element.component.id) { index, item in
                            let (c, a) = item
                            GridCell(component: c, ctx: ctx)
                                .frame(width: CGFloat(a.colSpan) * cellW - gap, height: CGFloat(a.rowSpan) * cellH - gap)
                                .staggeredAppear(index: index)
                                .offset(x: CGFloat(a.col1) * cellW + gap / 2, y: CGFloat(a.row1 - 1) * cellH + gap / 2)
                        }
                    }
                }
                ForEach(Array(placement.stacked.enumerated()), id: \.element.id) { index, c in
                    DSLView(component: c, ctx: ctx)
                        .staggeredAppear(index: placement.placed.count + index)
                }
            }
            .padding(inset)
            .frame(width: geo.size.width, alignment: .topLeading)

            if screen.isVoiceScreen {
                content
            } else {
                ScrollView {
                    content
                }
                .scrollDismissesKeyboard(.interactively)
                .scrollBounceBehavior(.basedOnSize)
            }
        }
    }
}

/// セル = 長方形の器。中身がはみ出す型(list / section / stack / text)は器の内側でスクロールし、
/// 上下にまだ続きがある時だけ端をフェードさせる(「切れている」ではなく「続きがある」と読める)。
private struct GridCell: View {
    let component: Component
    let ctx: RenderContext

    private var scrollsInside: Bool {
        switch component.type {
        case .list, .section, .vstack, .hstack, .text, .table, .timeline, .markdown, .card, .disclosure, .grid, .calendar: true
        default: false
        }
    }

    var body: some View {
        if scrollsInside {
            FadingScrollView {
                DSLView(component: component, ctx: ctx)
            }
        } else {
            DSLView(component: component, ctx: ctx)
                .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .top)
                .clipped()
        }
    }
}

/// 縦 ScrollView + 端フェード。iOS 17 で動くよう PreferenceKey で内容のフレームを拾い、はみ出している側だけマスクを掛ける。
private struct FadingScrollView<Content: View>: View {
    @ViewBuilder var content: Content
    @State private var contentFrame: CGRect = .zero
    @State private var viewportHeight: CGFloat = 0

    private let fade: CGFloat = 18
    private let slack: CGFloat = 2

    private var fadesTop: Bool { contentFrame.minY < -slack }
    private var fadesBottom: Bool { viewportHeight > 0 && contentFrame.maxY > viewportHeight + slack }

    var body: some View {
        ScrollView(.vertical) {
            content
                // 影が器の外にはみ出さないよう、カードの影ぶんだけ内側に余白
                .padding(.bottom, RenderTheme.Shadow.cardY + 2)
                .background(
                    GeometryReader { g in
                        Color.clear.preference(key: ContentFrameKey.self, value: g.frame(in: .named("fadingScroll")))
                    }
                )
        }
        .coordinateSpace(name: "fadingScroll")
        .scrollBounceBehavior(.basedOnSize)
        .onPreferenceChange(ContentFrameKey.self) { contentFrame = $0 }
        .background(
            GeometryReader { g in
                Color.clear.onAppear { viewportHeight = g.size.height }
                    .onChange(of: g.size.height) { _, h in viewportHeight = h }
            }
        )
        .mask(
            VStack(spacing: 0) {
                LinearGradient(colors: [fadesTop ? .clear : .black, .black], startPoint: .top, endPoint: .bottom)
                    .frame(height: fade)
                Color.black
                LinearGradient(colors: [.black, fadesBottom ? .clear : .black], startPoint: .top, endPoint: .bottom)
                    .frame(height: fade)
            }
            .animation(.easeOut(duration: 0.18), value: fadesTop)
            .animation(.easeOut(duration: 0.18), value: fadesBottom)
        )
        .clipShape(RoundedRectangle(cornerRadius: RenderTheme.Radius.inner, style: .continuous))
    }
}

private struct ContentFrameKey: PreferenceKey {
    static let defaultValue: CGRect = .zero
    static func reduce(value: inout CGRect, nextValue: () -> CGRect) { value = nextValue() }
}

// MARK: - 出現アニメ(変身の受け側)

/// 画面が差し替わった直後、各セルが index × stagger の遅延で薄く小さい状態から出現する。
/// RootView 側が `.id(document.version)` でツリーを作り直すので、変身のたびに onAppear が走る。Reduce Motion ではスキップ。
private struct StaggeredAppear: ViewModifier {
    let index: Int
    @Environment(\.accessibilityReduceMotion) private var reduceMotion
    @State private var shown = false

    func body(content: Content) -> some View {
        content
            .opacity(shown || reduceMotion ? 1 : 0)
            .scaleEffect(shown || reduceMotion ? 1 : RenderTheme.Motion.appearScale)
            .onAppear {
                guard !shown, !reduceMotion else { return }
                withAnimation(RenderTheme.Motion.appear.delay(Double(min(index, 12)) * RenderTheme.Motion.stagger)) {
                    shown = true
                }
            }
    }
}

private extension View {
    func staggeredAppear(index: Int) -> some View { modifier(StaggeredAppear(index: index)) }
}
