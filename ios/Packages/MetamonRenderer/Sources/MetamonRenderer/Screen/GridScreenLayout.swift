import SwiftUI
import MetamonDSL

/// 画面グリッド配置。
/// - 横 6 列(A-F)= 可視幅 ÷ 6、縦 1 行 = 可視高さ ÷ 12。`area` を持つ直下の子は矩形に絶対配置(セル内でスクロール)。
/// - `area` の無い子は「area で使われた最終行の次の行」から全幅で上から積む(フォールバック)。
/// - 13 行目以降 or フォールバックがはみ出せば画面全体が縦スクロール。voiceInput 画面(初期状態)はスクロールしない。
/// - 見た目: セル間ギャップ / 画面端余白は `RenderTheme.Spacing`、出現は index × stagger の遅延付きフェード(変身の受け側)。
/// - 最下段(行 11〜12)の左端(A〜B 列)/ 右端(E〜F 列)に部品があれば、その上端(global 座標)を
///   `GridBottomObstructionKey` / `GridBottomTrailingObstructionKey` で上げる(ホストがマイク FAB を上に逃がすため・#83 / #52)。
/// - 逆に背景扱いの背の高い部品(list 等)は FAB が上に浮くので、ホストが渡す `fabFrame` と重なる分だけ中身の末尾に余白を足す(#157)。
public struct GridScreenLayout: View {
    let screen: Screen
    let ctx: RenderContext

    private let gap = RenderTheme.Spacing.gridGap
    private let inset = RenderTheme.Spacing.screenInset
    /// キーボードの領域を含めた高さ(#143)。キーボードで可視領域が縮んでも行の高さはこれで決める
    @State private var stableHeight: CGFloat = 0
    /// フォーカスを得た入力欄(`keyboardAnchor` の id)。tick は同じ欄への再フォーカスでもスクロールさせるため
    @State private var focusRequest: (key: String, tick: Int)?
    @Environment(\.fabFrame) private var fabFrame

    public init(screen: Screen, ctx: RenderContext) {
        self.screen = screen
        self.ctx = ctx
    }

    public var body: some View {
        GeometryReader { geo in
            let cellW = (geo.size.width - inset * 2) / CGFloat(GridArea.columns)
            let layoutH = GridLayoutHeight.resolve(current: geo.size.height, stable: stableHeight)
            let cellH = (layoutH - inset * 2) / CGFloat(GridArea.visibleRows)
            let placement = screen.gridPlacement
            let lastRow = placement.placed.map(\.area.row2).max() ?? 0
            let gridHeight = CGFloat(lastRow) * cellH

            let content = VStack(alignment: .leading, spacing: gap) {
                if screen.entity != nil, !placement.placed.isEmpty {
                    // フォーム画面は area の行を「並び順と横幅」だけに使い、高さは中身なりにする(#131)。
                    // AI が section を A1:F4 のように小さく取ると入力欄がセルで切れていたため
                    FormFlowLayout(placed: placement.placed, cellW: cellW, cellH: cellH, ctx: ctx)
                } else if !placement.placed.isEmpty {
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
            // 入力欄の外(部品の無い所)をタップしたらキーボードを閉じる
            .background(Color.clear.contentShape(Rectangle()).onTapGesture { KeyboardState.dismiss() })

            Group {
                if screen.isVoiceScreen {
                    content
                } else {
                    ScrollViewReader { proxy in
                        ScrollView {
                            // area 無しで積んだ部品(全幅の list 等)は画面のスクロールの末尾に FAB ぶんの余白(#157)。
                            // area だけの画面は高さぴったりなので足さない(足すと常に少しスクロールしてしまう)
                            content
                                .padding(.bottom, placement.stacked.isEmpty ? 0 : FabClearance.contentMargin(viewport: geo.frame(in: .global), fab: fabFrame))
                        }
                        .scrollDismissesKeyboard(.interactively)
                        .scrollBounceBehavior(.basedOnSize)
                        // キーボードが上がり切ってから、フォーカスした入力欄を可視領域(保存バーとキーボードの上)の中央へ
                        .task(id: focusRequest?.tick) {
                            guard let key = focusRequest?.key else { return }
                            // 2 回目: キーボードのツールバー・保存バーの持ち上げが後から入って可視領域が変わるので、落ち着いた位置で合わせ直す
                            for delay in [350, 350] {
                                try? await Task.sleep(for: .milliseconds(delay))
                                guard !Task.isCancelled, KeyboardState.shared.isVisible else { return }
                                withAnimation(.easeOut(duration: 0.25)) { proxy.scrollTo(key, anchor: .center) }
                            }
                        }
                    }
                    .environment(\.focusedInputReporter) { key in
                        focusRequest = (key, (focusRequest?.tick ?? 0) + 1)
                    }
                }
            }
            .preference(key: GridBottomObstructionKey.self, value: bottomObstruction(geo: geo, cellH: cellH, placed: placement.placed.map(\.area)))
            .preference(key: GridBottomTrailingObstructionKey.self, value: bottomObstruction(geo: geo, cellH: cellH, placed: placement.placed.map(\.area), side: .trailing))
        }
        .background {
            GeometryReader { full in
                Color.clear
                    .onAppear { stableHeight = full.size.height }
                    .onChange(of: full.size.height) { _, h in stableHeight = h }
            }
            .ignoresSafeArea(.keyboard)
        }
    }

    /// FAB 帯(行 11〜12・`side` 側の 2 列)に掛かる部品の上端 y(global)。無ければ空。route.id をキーにしてホストが「今見えている画面」の値だけ使う。
    private func bottomObstruction(geo: GeometryProxy, cellH: CGFloat, placed: [GridArea], side: FabSide = .leading) -> [String: CGFloat] {
        let rows = FabClearance.obstructedRows(placed, side: side)
        guard rows > 0 else { return [:] }
        let bottom = geo.frame(in: .global).maxY - inset
        return [ctx.route.id: bottom - CGFloat(rows) * cellH + gap / 2]
    }
}

/// フォーム画面の配置(#131)。行が重なる部品を 1 段にまとめ、段ごとに上から積む。
/// 横位置・幅は area のまま(列 × cellW)、高さは中身なり。面を自前で埋める型(chart / map 等)だけ area の高さを保つ。
/// 前の段から 3 行以上空いて下に置かれた部品(最下段の削除ボタン等)は、少し間を空けて続ける。
struct FormFlowLayout: View {
    let placed: [(component: Component, area: GridArea)]
    let cellW: CGFloat
    let cellH: CGFloat
    let ctx: RenderContext

    private let gap = RenderTheme.Spacing.gridGap

    struct Band: Identifiable {
        var id: String { items.map(\.component.id).joined(separator: "|") }
        var items: [(component: Component, area: GridArea)]
        var row1: Int
        var row2: Int
    }

    /// row1 → col1 の順に並べ、行範囲が重なるものを同じ段にまとめる。
    static func bands(_ placed: [(component: Component, area: GridArea)]) -> [Band] {
        let sorted = placed.sorted { ($0.area.row1, $0.area.col1) < ($1.area.row1, $1.area.col1) }
        var out: [Band] = []
        for item in sorted {
            if var last = out.last, item.area.row1 <= last.row2 {
                last.items.append(item)
                last.row2 = max(last.row2, item.area.row2)
                out[out.count - 1] = last
            } else {
                out.append(Band(items: [item], row1: item.area.row1, row2: item.area.row2))
            }
        }
        return out
    }

    var body: some View {
        let bands = Self.bands(placed)
        VStack(alignment: .leading, spacing: gap) {
            ForEach(Array(bands.enumerated()), id: \.element.id) { index, band in
                let detached = index > 0 && band.row1 - bands[index - 1].row2 > 2
                HStack(alignment: .top, spacing: 0) {
                    ForEach(Array(band.items.enumerated()), id: \.element.component.id) { i, item in
                        let prevEnd = i == 0 ? 0 : band.items[i - 1].area.col2 + 1
                        let lead = CGFloat(max(0, item.area.col1 - prevEnd)) * cellW
                        cell(item.component, area: item.area)
                            .frame(width: CGFloat(item.area.colSpan) * cellW - gap)
                            .padding(.leading, lead + (i == 0 ? gap / 2 : gap))
                    }
                }
                .padding(.top, detached ? RenderTheme.Spacing.md : 0)
                .staggeredAppear(index: index)
            }
        }
    }

    @ViewBuilder
    private func cell(_ c: Component, area: GridArea) -> some View {
        switch c.type {
        case .image, .mapView, .webView, .chart, .gauge, .calendar, .qrScanner, .list, .table, .timeline:
            DSLView(component: c, ctx: ctx).frame(height: max(cellH * 2, CGFloat(area.rowSpan) * cellH - gap))
        default:
            DSLView(component: c, ctx: ctx).fixedSize(horizontal: false, vertical: true)
        }
    }
}

/// マイク FAB が吸着している辺(#52: ドラッグで左右どちらにも置ける)。
public enum FabSide: String, Sendable, CaseIterable {
    case leading, trailing
}

/// マイク FAB(ホストが左下 or 右下に置く)と DSL 部品の衝突回避(#83 / #52)。
public enum FabClearance {
    /// FAB が座る帯 = 最下段 2 行。
    public static let bandRows = 2
    /// FAB が重なる列 = 吸着している側の端 2 列(左: A〜B / 右: E〜F)。
    public static let bandColumns = 2
    /// 「最下段に寄せた操作部品」とみなす行の下限(これより上から始まる背の高い部品は、FAB が浮く背景として扱う)。
    public static let anchoredRowMin = GridArea.visibleRows - 3

    /// 帯に掛かる部品のうち、最も上から始まるものが占める「下からの行数」(0 = 障害なし)。
    /// 例(左): `A11:F12` → 2 / `A12:F12` → 1 / `D11:F12`(右下の追加ボタン)→ 0 / `A1:F12`(全面 list)→ 0。
    /// 例(右): `D11:F12` → 2 / `A11:D12` → 0。
    public static func obstructedRows(_ areas: [GridArea], side: FabSide = .leading) -> Int {
        areas
            .filter { a in
                let inBand = switch side {
                case .leading: a.col1 < bandColumns
                case .trailing: a.col2 >= GridArea.columns - bandColumns
                }
                return inBand && a.row2 > GridArea.visibleRows - bandRows && a.row1 >= anchoredRowMin
            }
            .map { GridArea.visibleRows - $0.row1 + 1 }
            .max() ?? 0
    }
}

/// `GridScreenLayout` が上げる「FAB 帯(左端)に掛かる部品の上端 y(global)」。キーは `Route.id`(タブ分だけ同時に存在しうる)。
public struct GridBottomObstructionKey: PreferenceKey {
    public static let defaultValue: [String: CGFloat] = [:]
    public static func reduce(value: inout [String: CGFloat], nextValue: () -> [String: CGFloat]) {
        value.merge(nextValue()) { _, new in new }
    }
}

/// 同じく右端(E〜F 列)版。FAB を右に吸着させた時に使う(#52)。
public struct GridBottomTrailingObstructionKey: PreferenceKey {
    public static let defaultValue: [String: CGFloat] = [:]
    public static func reduce(value: inout [String: CGFloat], nextValue: () -> [String: CGFloat]) {
        value.merge(nextValue()) { _, new in new }
    }
}

/// セル = 長方形の器。中身がはみ出す型(list / section / stack / text)は器の内側でスクロールし、
/// 上下にまだ続きがある時だけ端をフェードさせる(「切れている」ではなく「続きがある」と読める)。
/// Container 内グリッド(#81)の子のセルにも再利用する(`LocalGrid`)。
struct GridCell: View {
    let component: Component
    let ctx: RenderContext
    @Environment(\.dynamicTypeSize) private var typeSize

    /// 影がセルの外へ滲む幅(カード / ブランドボタンの影半径 + y)。ギャップ(10)を少し超えるが隣の面より下に描かれるので問題ない
    static let shadowBleed: CGFloat = 16

    /// 通常サイズでは「中身が伸びる型」だけ内側スクロール。Dynamic Type のアクセシビリティサイズでは
    /// stat / button / 入力なども固定高のセルからはみ出すので、面を自前で埋める型以外は全部スクロールに逃がす。
    private var scrollsInside: Bool {
        // Container 内グリッド(#81)は子のセルがそれぞれ器になるので、Container 自体は器いっぱいに固定する
        if hasLocalGrid { return false }
        return switch component.type {
        // hstack は通常サイズでは器に収める(stat を横に並べる定番。スクロールにすると高さが無制限になり、stat の数値がセル高さで縮まず下が切れる・#89)
        case .list, .section, .vstack, .text, .table, .timeline, .markdown, .card, .disclosure, .grid, .calendar, .settlement: true
        case .image, .mapView, .webView, .chart, .gauge, .spacer, .divider, .qrScanner, .voiceInput: false
        default: typeSize.isAccessibilitySize
        }
    }

    private var hasLocalGrid: Bool { component.localGridPlacement != nil }

    @Environment(\.dslRuntime) private var rt
    @Environment(\.fabFrame) private var fabFrame
    /// 器の矩形(global)。マイク FAB と重なる分だけ中身の末尾に余白を足す(#157)
    @State private var frame: CGRect = .zero

    /// list(または list だけの section)に行がある時は、カードを行の高さで止める(#131: 4 行なのにセルいっぱいの白い面が残っていた)。
    /// 空の時は従来どおり器の高さを渡し、空状態をセル全体に出す(#84)。
    private var hugsContent: Bool {
        let lists = component.type == .list ? [component]
            : component.type == .section && !component.children.isEmpty && component.children.allSatisfy({ $0.type == .list }) ? component.children : []
        return !lists.isEmpty && lists.allSatisfy { !rt.filteredRecords($0).isEmpty }
    }

    var body: some View {
        if scrollsInside {
            FadingScrollView(fillsViewport: !hugsContent, trailingMargin: FabClearance.contentMargin(viewport: frame, fab: fabFrame)) {
                DSLView(component: component, ctx: ctx)
            }
            .onGeometryChange(for: CGRect.self) { $0.frame(in: .global) } action: { frame = $0 }
        } else {
            DSLView(component: component, ctx: ctx)
                .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .top)
                // 器の矩形で切るとカード/ボタンの影まで四角く切れて「薄い箱」が見える(#131)。影の分だけ外側まで描かせる
                .mask(Rectangle().padding(-GridCell.shadowBleed))
                .environment(\.localGridFills, hasLocalGrid)
        }
    }
}

/// 縦 ScrollView + 端フェード。iOS 17 で動くよう PreferenceKey で内容のフレームを拾い、はみ出している側だけマスクを掛ける。
/// Reduce Motion ではフェードの切り替えをアニメーションせず即時に差し替える(マスク自体は「続きがある」合図なので残す)。
/// 中身が器より低い時は器の高さを提案する(#84: 空の list / 行の少ない list がセル全体を使える)。
private struct FadingScrollView<Content: View>: View {
    var fillsViewport = true
    /// 中身の末尾に足す余白(マイク FAB の下に隠れる最後の行を、スクロールで FAB の上へ出す・#157)
    var trailingMargin: CGFloat = 0
    @ViewBuilder var content: Content
    @State private var contentFrame: CGRect = .zero
    @State private var viewportHeight: CGFloat = 0
    @Environment(\.accessibilityReduceMotion) private var reduceMotion

    private let fade: CGFloat = 18
    private let slack: CGFloat = 2
    private let bottomPad = RenderTheme.Shadow.cardY + 2

    private var fadesTop: Bool { contentFrame.minY < -slack }
    private var fadesBottom: Bool { viewportHeight > 0 && contentFrame.maxY > viewportHeight + slack }

    var body: some View {
        ScrollView(.vertical) {
            content
                .frame(minHeight: fillsViewport ? max(0, viewportHeight - bottomPad - trailingMargin) : nil, alignment: .top)
                // 影が器の外にはみ出さないよう、カードの影ぶんだけ内側に余白
                .padding(.bottom, bottomPad)
                .background(
                    GeometryReader { g in
                        Color.clear.preference(key: ContentFrameKey.self, value: g.frame(in: .named("fadingScroll")))
                    }
                )
                // FAB ぶんの余白は中身の末尾に持つ(フェード判定には含めない)。contentMargins では末尾までスクロールしても最後の行が FAB の下に残った
                .padding(.bottom, trailingMargin)
        }
        #if DEBUG
        // 撮影用(#157): `METAMON_SCROLL_CELLS_TO_END=1` でセル内スクロールを末尾から表示(FAB の上に最後の行が出るかを確認)
        .defaultScrollAnchor(ProcessInfo.processInfo.environment["METAMON_SCROLL_CELLS_TO_END"] == "1" ? .bottom : nil)
        #endif
        .coordinateSpace(name: "fadingScroll")
        .scrollBounceBehavior(.basedOnSize)
        .onPreferenceChange(ContentFrameKey.self) { contentFrame = $0 }
        .background(
            GeometryReader { g in
                Color.clear.onAppear { viewportHeight = g.size.height }
                    .onChange(of: g.size.height) { _, h in viewportHeight = h }
            }
        )
        // 横は影の分だけ外に描かせる(角丸 clip で list カードの影が四角く切れていた・#131)。縦はフェードの位置を器の端に合わせる
        .scrollClipDisabled()
        .mask(
            VStack(spacing: 0) {
                LinearGradient(colors: [fadesTop ? .clear : .black, .black], startPoint: .top, endPoint: .bottom)
                    .frame(height: fade)
                Color.black
                LinearGradient(colors: [.black, fadesBottom ? .clear : .black], startPoint: .top, endPoint: .bottom)
                    .frame(height: fade)
            }
            .padding(.horizontal, -GridCell.shadowBleed)
            .animation(reduceMotion ? nil : .easeOut(duration: 0.18), value: fadesTop)
            .animation(reduceMotion ? nil : .easeOut(duration: 0.18), value: fadesBottom)
        )
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
