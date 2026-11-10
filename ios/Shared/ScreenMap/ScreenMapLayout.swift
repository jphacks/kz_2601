import SwiftUI
import MetamonRenderer

/// 画面マップ(#116)の UIKit / AppKit に依存しない部分。iOS の `ScreenMapSheet` と Mac の `MacScreenMapView`(#970)が共有する:
/// 「この画面について話す」の文脈・文言・カードの並べ方・矢印の形。

/// 「この画面について話す」で修正の会話に渡す画面。
struct ScreenContext: Equatable {
    var id: String
    var title: String
    /// 部品から開いたシートの AI ボタンで押した時の部品(#629)
    var component: AIButtonComponent?
}

enum ScreenMapCopy {
    static func contextPrefix(_ s: ScreenContext) -> String {
        if let c = s.component { return L10n.tr("画面「\(s.title)」(id: \(s.id))の部品「\(c.label)」(id: \(c.id))で: ") }
        return L10n.tr("画面「\(s.title)」(id: \(s.id))で: ")
    }

    /// 会話のチップの文言。
    static func chipText(_ s: ScreenContext) -> String {
        if let c = s.component { return L10n.tr("画面「\(s.title)」の「\(c.label)」について話す") }
        return L10n.tr("画面「\(s.title)」について話す")
    }

    static var title: String { L10n.tr("画面マップ") }

    /// 遷移 1 本を言葉にする(詳細の「ここから行ける」「ここへ来られる」)。
    static func describe(_ e: ScreenGraph.Edge) -> String {
        let how: String = switch e.trigger {
        case .button(let label): L10n.tr("ボタン「\(label)」")
        case .toolbar(let label): L10n.tr("上部の「\(label)」")
        case .itemTap: L10n.tr("一覧の行")
        case .dateTap: L10n.tr("日付のタップ")
        case .afterSave: L10n.tr("保存したあと")
        case .other: L10n.tr("部品のタップ")
        }
        switch e.presentation {
        case .push: return how
        case .sheet: return how + L10n.tr("(シート)")
        case .fullScreen: return how + L10n.tr("(全画面)")
        }
    }
}

/// カードの置き場所。カードは段ごとに中央揃えで並べる(段は `ScreenGraph.rows`)。
/// 幅に収まるならカードを広げ、収まらなければ最小幅で横スクロール。
struct ScreenMapLayout {
    let graph: ScreenGraph
    let availableWidth: CGFloat
    var cardHeight: CGFloat = 66
    var columnGap: CGFloat = 18
    var rowGap: CGFloat = 64
    var inset: CGFloat = Theme.Spacing.lg
    var minCardWidth: CGFloat = 104
    var maxCardWidth: CGFloat = 150
    /// 到達できない段の上に置く見出しのぶん
    var unreachableLabel: CGFloat = 26

    var maxColumns: Int { max(1, graph.rows.map(\.count).max() ?? 1) }
    var hasUnreachable: Bool { !graph.unreachable.isEmpty }

    var cardWidth: CGFloat {
        let n = CGFloat(maxColumns)
        let fit = (availableWidth - inset * 2 - columnGap * (n - 1)) / n
        return min(maxCardWidth, max(minCardWidth, fit))
    }

    var contentWidth: CGFloat {
        max(availableWidth, inset * 2 + CGFloat(maxColumns) * cardWidth + CGFloat(maxColumns - 1) * columnGap)
    }

    var contentHeight: CGFloat {
        let rows = CGFloat(graph.rows.count)
        return inset * 2 + rows * cardHeight + max(0, rows - 1) * rowGap + (hasUnreachable ? unreachableLabel : 0)
    }

    func rowTop(_ row: Int) -> CGFloat {
        let extra = hasUnreachable && row == graph.rows.count - 1 ? unreachableLabel : 0
        return inset + CGFloat(row) * (cardHeight + rowGap) + extra
    }

    func frame(of node: ScreenGraph.Node) -> CGRect {
        let count = CGFloat(graph.rows[node.row].count)
        let rowWidth = count * cardWidth + (count - 1) * columnGap
        let x = (contentWidth - rowWidth) / 2 + CGFloat(node.column) * (cardWidth + columnGap)
        return CGRect(x: x, y: rowTop(node.row), width: cardWidth, height: cardHeight)
    }

    /// 矢印を描く。上り・同じ段・往復の矢印は曲げて、逆向きの線やカードと重ならないようにする。
    /// `selected` の出入りを濃く、それ以外を薄く。到達できない画面から出る矢印は薄く。
    func drawArrows(in ctx: GraphicsContext, selected: String?, accent: Color) {
        let frames = Dictionary(uniqueKeysWithValues: graph.nodes.map { ($0.id, frame(of: $0)) })
        let pairs = Set(graph.links.map(\.id))
        for link in graph.links where link.from != link.to {
            guard let a = frames[link.from], let b = frames[link.to] else { continue }
            let from = graph.node(link.from), to = graph.node(link.to)
            let reverse = pairs.contains("\(link.to)->\(link.from)")
            let curved = reverse || (to?.row ?? 0) <= (from?.row ?? 0)
            let sameRow = to?.row == from?.row
            let path = ArrowGeometry.path(from: a, to: b, curved: curved, below: sameRow && !reverse)
            let active = selected == nil || selected == link.from || selected == link.to
            let color: Color = active ? accent : .secondary
            let opacity = (from?.reachable ?? false) ? (active ? 0.9 : 0.25) : 0.25
            ctx.stroke(path.line, with: .color(color.opacity(opacity)),
                       style: StrokeStyle(lineWidth: active && selected != nil ? 2.4 : 1.6, lineCap: .round,
                                          dash: link.isModal ? [6, 4] : []))
            ctx.fill(path.head, with: .color(color.opacity(opacity)))
        }
    }
}

/// 矢印の線と矢じり。線はカードの中心どうしを結び、カードの縁で切る。曲げる時は進行方向の左へ膨らませる(往復は自然に左右に分かれる)。
enum ArrowGeometry {
    static let bend: CGFloat = 34
    static let headLength: CGFloat = 9

    /// `below`: 同じ段どうしはカードの下を回る(段の間の線と重ねない)
    static func path(from a: CGRect, to b: CGRect, curved: Bool, below: Bool = false) -> (line: Path, head: Path) {
        let ca = CGPoint(x: a.midX, y: a.midY), cb = CGPoint(x: b.midX, y: b.midY)
        let dx = cb.x - ca.x, dy = cb.y - ca.y
        let len = max(1, hypot(dx, dy))
        let mid = CGPoint(x: (ca.x + cb.x) / 2, y: (ca.y + cb.y) / 2)
        let control = below ? CGPoint(x: mid.x, y: max(a.maxY, b.maxY) + bend)
            : curved ? CGPoint(x: mid.x + dy / len * bend, y: mid.y - dx / len * bend) : mid
        let start = edgePoint(of: a, toward: control)
        let end = edgePoint(of: b, toward: control)
        var line = Path()
        line.move(to: start)
        if curved { line.addQuadCurve(to: end, control: control) } else { line.addLine(to: end) }

        let ux = end.x - control.x, uy = end.y - control.y
        let ul = max(1, hypot(ux, uy))
        let (nx, ny) = (ux / ul, uy / ul)
        let base = CGPoint(x: end.x - nx * headLength, y: end.y - ny * headLength)
        var head = Path()
        head.move(to: end)
        head.addLine(to: CGPoint(x: base.x - ny * headLength * 0.55, y: base.y + nx * headLength * 0.55))
        head.addLine(to: CGPoint(x: base.x + ny * headLength * 0.55, y: base.y - nx * headLength * 0.55))
        head.closeSubpath()
        return (line, head)
    }

    /// 矩形の中心から `p` へ向かう半直線と、矩形の縁(少し外側)との交点。
    static func edgePoint(of r: CGRect, toward p: CGPoint) -> CGPoint {
        let c = CGPoint(x: r.midX, y: r.midY)
        let dx = p.x - c.x, dy = p.y - c.y
        guard dx != 0 || dy != 0 else { return c }
        let hw = r.width / 2 + 3, hh = r.height / 2 + 3
        let t = min(dx == 0 ? .infinity : hw / abs(dx), dy == 0 ? .infinity : hh / abs(dy))
        return CGPoint(x: c.x + dx * t, y: c.y + dy * t)
    }
}
