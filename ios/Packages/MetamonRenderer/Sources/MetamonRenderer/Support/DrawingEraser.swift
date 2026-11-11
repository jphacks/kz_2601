import Foundation
import PencilKit

/// 線ごと消す消しゴム(iOS の `PKEraserTool(.vector)` と同じ動き)。PKCanvasView が無い Mac で使う。
/// 消しゴムが通った線分(前の点 → 今の点)に触れた線を 1 本ごと取り除く。両 OS で試せる純粋な関数。
enum DrawingEraser {
    /// 消しゴムの当たりの半径(線の太さの半分に足す)
    static let radius: CGFloat = 6

    /// `from` → `to` を通った消しゴムに触れた線を除いた絵。触れた線が無ければ同じ線のまま返す。
    static func erase(_ drawing: PKDrawing, from: CGPoint, to: CGPoint, radius: CGFloat = radius) -> PKDrawing {
        let kept = drawing.strokes.filter { !hits($0, from: from, to: to, radius: radius) }
        guard kept.count != drawing.strokes.count else { return drawing }
        return PKDrawing(strokes: kept)
    }

    /// 線が消しゴムの線分に `radius` + 線の太さの半分 より近ければ当たり。
    static func hits(_ stroke: PKStroke, from: CGPoint, to: CGPoint, radius: CGFloat) -> Bool {
        // 線の描かれる範囲(太さ込み)から遠ければ点を見ない
        let eraser = CGRect(x: min(from.x, to.x), y: min(from.y, to.y), width: abs(to.x - from.x), height: abs(to.y - from.y))
        guard stroke.renderBounds.insetBy(dx: -radius - 1, dy: -radius - 1).intersects(eraser.insetBy(dx: -1, dy: -1)) else { return false }
        let points = stroke.path.interpolatedPoints(by: .distance(3)).map {
            (location: $0.location.applying(stroke.transform), half: max($0.size.width, $0.size.height) / 2)
        }
        guard let first = points.first else { return false }
        if points.count == 1 {
            return distance(first.location, toSegment: from, to) <= radius + first.half
        }
        for (a, b) in zip(points, points.dropFirst())
        where segmentDistance(a.location, b.location, from, to) <= radius + max(a.half, b.half) {
            return true
        }
        return false
    }

    /// 点 p と線分 ab の距離。
    static func distance(_ p: CGPoint, toSegment a: CGPoint, _ b: CGPoint) -> CGFloat {
        let dx = b.x - a.x, dy = b.y - a.y
        let len2 = dx * dx + dy * dy
        let t = len2 == 0 ? 0 : min(1, max(0, ((p.x - a.x) * dx + (p.y - a.y) * dy) / len2))
        return hypot(p.x - (a.x + t * dx), p.y - (a.y + t * dy))
    }

    /// 線分 ab と線分 cd の距離(交われば 0)。
    static func segmentDistance(_ a: CGPoint, _ b: CGPoint, _ c: CGPoint, _ d: CGPoint) -> CGFloat {
        if intersects(a, b, c, d) { return 0 }
        return min(distance(a, toSegment: c, d), distance(b, toSegment: c, d),
                   distance(c, toSegment: a, b), distance(d, toSegment: a, b))
    }

    private static func intersects(_ a: CGPoint, _ b: CGPoint, _ c: CGPoint, _ d: CGPoint) -> Bool {
        func cross(_ o: CGPoint, _ p: CGPoint, _ q: CGPoint) -> CGFloat { (p.x - o.x) * (q.y - o.y) - (p.y - o.y) * (q.x - o.x) }
        let d1 = cross(c, d, a), d2 = cross(c, d, b), d3 = cross(a, b, c), d4 = cross(a, b, d)
        return ((d1 > 0 && d2 < 0) || (d1 < 0 && d2 > 0)) && ((d3 > 0 && d4 < 0) || (d3 < 0 && d4 > 0))
    }
}
