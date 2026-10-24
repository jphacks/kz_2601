import SwiftUI
import MetamonDSL

/// Container 内グリッド(#81)。card / section / vstack の子が全員 `area` を持つ時、Container の中身の矩形を
/// 横 6 列のローカルグリッドとみなして子を置く(A1 = Container の左上)。
/// - 器いっぱいモード(`localGridFills` = true: 通常画面で area を持つ Container のセル): 1 行 = 中身の高さ ÷ 子が使う最終行。
///   子は `GridCell` で描く(セル内スクロール・影の外側描画・list を行の高さで止める挙動を画面グリッドと共有)。
/// - 流し込みモード(既定: フォーム画面の `FormFlowLayout` / area 無しで積まれた Container): 横位置と幅だけ効かせ、
///   行が重なる子を 1 段にまとめて上から積む。高さは中身なり(#139 の方針)。
struct LocalGrid: View {
    let placed: [(component: Component, area: GridArea)]
    let ctx: RenderContext
    @Environment(\.localGridFills) private var fills

    var body: some View {
        LocalGridLayout(areas: placed.map(\.area), fills: fills) {
            ForEach(placed, id: \.component.id) { item in
                if fills {
                    GridCell(component: item.component, ctx: ctx)
                } else {
                    flowCell(item.component, area: item.area)
                }
            }
        }
    }

    /// FormFlowLayout.cell と同じ扱い: 面を自前で埋める型だけ最低限の高さを保ち、他は中身なり。
    @ViewBuilder
    private func flowCell(_ c: Component, area: GridArea) -> some View {
        switch c.type {
        case .image, .mapView, .webView, .chart, .gauge, .calendar, .qrScanner, .list, .table, .timeline:
            DSLView(component: c, ctx: ctx).frame(height: max(120, CGFloat(area.rowSpan) * 60))
        default:
            DSLView(component: c, ctx: ctx).fixedSize(horizontal: false, vertical: true)
        }
    }
}

/// `LocalGrid` の配置。列幅 = (幅 + gap) ÷ 6 で、外周には余白を作らない(Container の padding に揃える)。
struct LocalGridLayout: Layout {
    let areas: [GridArea]
    let fills: Bool
    var gap: CGFloat = RenderTheme.Spacing.gridGap

    private var lastRow: Int { max(1, areas.map(\.row2).max() ?? 1) }
    private func colW(_ width: CGFloat) -> CGFloat { (width + gap) / CGFloat(GridArea.columns) }
    private func width(_ a: GridArea, in total: CGFloat) -> CGFloat { max(0, CGFloat(a.colSpan) * colW(total) - gap) }

    /// 流し込みの段(row1 → col1 の順・行範囲が重なるものを同じ段に。FormFlowLayout.bands と同じ規則)。
    private var bands: [[Int]] {
        let order = areas.indices.sorted { (areas[$0].row1, areas[$0].col1) < (areas[$1].row1, areas[$1].col1) }
        var out: [[Int]] = []
        var bandEnd = 0
        for i in order {
            if !out.isEmpty, areas[i].row1 <= bandEnd {
                out[out.count - 1].append(i)
                bandEnd = max(bandEnd, areas[i].row2)
            } else {
                out.append([i])
                bandEnd = areas[i].row2
            }
        }
        return out
    }

    /// 流し込みで各段の高さを測り、置く位置(y)を返す。
    private func flow(width total: CGFloat, subviews: Subviews, place: ((Int, CGFloat, ProposedViewSize) -> Void)? = nil) -> CGFloat {
        var y: CGFloat = 0
        for (n, band) in bands.enumerated() {
            var h: CGFloat = 0
            for i in band where i < subviews.count {
                let p = ProposedViewSize(width: width(areas[i], in: total), height: nil)
                h = max(h, subviews[i].sizeThatFits(p).height)
                place?(i, y, p)
            }
            y += h + (n == bands.count - 1 ? 0 : gap)
        }
        return y
    }

    func sizeThatFits(proposal: ProposedViewSize, subviews: Subviews, cache: inout ()) -> CGSize {
        let w = proposal.width ?? 320
        if fills, let h = proposal.height {
            return CGSize(width: w, height: h)
        }
        return CGSize(width: w, height: flow(width: w, subviews: subviews))
    }

    func placeSubviews(in bounds: CGRect, proposal: ProposedViewSize, subviews: Subviews, cache: inout ()) {
        let cw = colW(bounds.width)
        if fills {
            let rowH = (bounds.height + gap) / CGFloat(lastRow)
            for (i, sv) in subviews.enumerated() where i < areas.count {
                let a = areas[i]
                sv.place(
                    at: CGPoint(x: bounds.minX + CGFloat(a.col1) * cw, y: bounds.minY + CGFloat(a.row1 - 1) * rowH),
                    anchor: .topLeading,
                    proposal: ProposedViewSize(width: width(a, in: bounds.width), height: max(0, CGFloat(a.rowSpan) * rowH - gap))
                )
            }
        } else {
            _ = flow(width: bounds.width, subviews: subviews) { i, y, p in
                subviews[i].place(at: CGPoint(x: bounds.minX + CGFloat(areas[i].col1) * cw, y: bounds.minY + y), anchor: .topLeading, proposal: p)
            }
        }
    }
}

private struct LocalGridFillsKey: EnvironmentKey {
    static let defaultValue = false
}

extension EnvironmentValues {
    /// Container 内グリッドを器いっぱいに置くか(`GridCell` が area 付き Container を描く時だけ true)。
    var localGridFills: Bool {
        get { self[LocalGridFillsKey.self] }
        set { self[LocalGridFillsKey.self] = newValue }
    }
}
