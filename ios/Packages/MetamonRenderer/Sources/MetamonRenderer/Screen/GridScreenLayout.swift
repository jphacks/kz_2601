import SwiftUI
import MetamonDSL

/// 画面グリッド配置。
/// - 横 6 列(A-F)= 可視幅 ÷ 6、縦 1 行 = 可視高さ ÷ 12。`area` を持つ直下の子は矩形に絶対配置(セル内でスクロール)。
/// - `area` の無い子は「area で使われた最終行の次の行」から全幅で上から積む(フォールバック)。
/// - 13 行目以降 or フォールバックがはみ出せば画面全体が縦スクロール。voiceInput 画面(初期状態)はスクロールしない。
/// - 見た目: セル間ギャップ / 画面端余白は `RenderTheme.Spacing`、出現は index × stagger の遅延付きフェード(変身の受け側)。
/// - 最下段(行 11〜12)の左端(A〜B 列)に部品があれば、その上端(global 座標)を `GridBottomObstructionKey` で上げる
///   (ホストがマイク FAB を上に逃がすため・#83)。
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

            Group {
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
            .preference(key: GridBottomObstructionKey.self, value: bottomObstruction(geo: geo, cellH: cellH, placed: placement.placed.map(\.area)))
        }
    }

    /// FAB 帯(行 11〜12・A〜B 列)に掛かる部品の上端 y(global)。無ければ空。route.id をキーにしてホストが「今見えている画面」の値だけ使う。
    private func bottomObstruction(geo: GeometryProxy, cellH: CGFloat, placed: [GridArea]) -> [String: CGFloat] {
        let rows = FabClearance.obstructedRows(placed)
        guard rows > 0 else { return [:] }
        let bottom = geo.frame(in: .global).maxY - inset
        return [ctx.route.id: bottom - CGFloat(rows) * cellH + gap / 2]
    }
}

/// マイク FAB(ホストが左下に置く)と DSL 部品の衝突回避(#83)。
public enum FabClearance {
    /// FAB が座る帯 = 最下段 2 行。
    public static let bandRows = 2
    /// FAB が重なる列 = A〜B(左端 2 列)。
    public static let bandColumns = 2
    /// 「最下段に寄せた操作部品」とみなす行の下限(これより上から始まる背の高い部品は、FAB が浮く背景として扱う)。
    public static let anchoredRowMin = GridArea.visibleRows - 3

    /// 帯に掛かる部品のうち、最も上から始まるものが占める「下からの行数」(0 = 障害なし)。
    /// 例: `A11:F12` → 2 / `A12:F12` → 1 / `D11:F12`(右下の追加ボタン)→ 0 / `A1:F12`(全面 list)→ 0。
    public static func obstructedRows(_ areas: [GridArea]) -> Int {
        areas
            .filter { $0.col1 < bandColumns && $0.row2 > GridArea.visibleRows - bandRows && $0.row1 >= anchoredRowMin }
            .map { GridArea.visibleRows - $0.row1 + 1 }
            .max() ?? 0
    }
}

/// `GridScreenLayout` が上げる「FAB 帯に掛かる部品の上端 y(global)」。キーは `Route.id`(タブ分だけ同時に存在しうる)。
public struct GridBottomObstructionKey: PreferenceKey {
    public static let defaultValue: [String: CGFloat] = [:]
    public static func reduce(value: inout [String: CGFloat], nextValue: () -> [String: CGFloat]) {
        value.merge(nextValue()) { _, new in new }
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
/// 中身が器より低い時は器の高さを提案する(#84: 空の list / 行の少ない list がセル全体を使える)。
private struct FadingScrollView<Content: View>: View {
    @ViewBuilder var content: Content
    @State private var contentFrame: CGRect = .zero
    @State private var viewportHeight: CGFloat = 0

    private let fade: CGFloat = 18
    private let slack: CGFloat = 2
    private let bottomPad = RenderTheme.Shadow.cardY + 2

    private var fadesTop: Bool { contentFrame.minY < -slack }
    private var fadesBottom: Bool { viewportHeight > 0 && contentFrame.maxY > viewportHeight + slack }

    var body: some View {
        ScrollView(.vertical) {
            content
                .frame(minHeight: max(0, viewportHeight - bottomPad), alignment: .top)
                // 影が器の外にはみ出さないよう、カードの影ぶんだけ内側に余白
                .padding(.bottom, bottomPad)
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
