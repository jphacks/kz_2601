import SwiftUI
import MetamonDSL

/// 画面グリッド配置。
/// - 横 6 列(A-F)= 可視幅 ÷ 6、縦 1 行 = 可視高さ ÷ 12。`area` を持つ直下の子は矩形に絶対配置(セル内でスクロール)。
/// - `area` の無い子は「area で使われた最終行の次の行」から全幅で上から積む(フォールバック)。
/// - 13 行目以降 or フォールバックがはみ出せば画面全体が縦スクロール。voiceInput 画面(初期状態)はスクロールしない。
public struct GridScreenLayout: View {
    let screen: Screen
    let ctx: RenderContext

    private let gap: CGFloat = 8
    private let inset: CGFloat = 12

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
                        ForEach(placement.placed, id: \.component.id) { c, a in
                            GridCell(component: c, ctx: ctx)
                                .frame(width: CGFloat(a.colSpan) * cellW - gap, height: CGFloat(a.rowSpan) * cellH - gap)
                                .offset(x: CGFloat(a.col1) * cellW + gap / 2, y: CGFloat(a.row1 - 1) * cellH + gap / 2)
                        }
                    }
                }
                ForEach(placement.stacked) { DSLView(component: $0, ctx: ctx) }
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

/// セル = 長方形の器。中身がはみ出す型(list / section / stack / text)は器の内側でスクロールする。
private struct GridCell: View {
    let component: Component
    let ctx: RenderContext

    private var scrollsInside: Bool {
        switch component.type {
        case .list, .section, .vstack, .hstack, .text, .table, .timeline, .markdown: true
        default: false
        }
    }

    var body: some View {
        if scrollsInside {
            ScrollView(.vertical) {
                DSLView(component: component, ctx: ctx)
            }
            .scrollBounceBehavior(.basedOnSize)
            .clipShape(RoundedRectangle(cornerRadius: 12))
        } else {
            DSLView(component: component, ctx: ctx)
                .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .top)
                .clipped()
        }
    }
}
