package jp.kz2601.metamon.renderer.navigation

import kotlin.math.abs
import kotlin.math.hypot
import kotlin.math.max
import kotlin.math.min

/** 画面マップの座標(単位は dp)。Compose に依存しない。 */
data class MapPoint(val x: Float, val y: Float)

data class MapRect(val x: Float, val y: Float, val width: Float, val height: Float) {
    val midX: Float get() = x + width / 2
    val midY: Float get() = y + height / 2
    val maxY: Float get() = y + height
}

/**
 * カードの置き場所(iOS / Mac 共有の `ScreenMapLayout`)。カードは段ごとに中央揃え(段は [ScreenGraph.rows])。
 * 幅に収まるならカードを広げ、収まらなければ最小幅で横スクロール。
 */
data class ScreenMapLayout(
    val graph: ScreenGraph,
    val availableWidth: Float,
    val cardHeight: Float = 66f,
    val columnGap: Float = 18f,
    val rowGap: Float = 64f,
    val inset: Float = 16f,
    val minCardWidth: Float = 104f,
    val maxCardWidth: Float = 150f,
    /** 到達できない段の上に置く見出しのぶん */
    val unreachableLabel: Float = 26f,
) {
    val maxColumns: Int get() = max(1, graph.rows.maxOfOrNull { it.size } ?: 1)
    val hasUnreachable: Boolean get() = graph.unreachable.isNotEmpty()

    val cardWidth: Float
        get() {
            val n = maxColumns.toFloat()
            val fit = (availableWidth - inset * 2 - columnGap * (n - 1)) / n
            return min(maxCardWidth, max(minCardWidth, fit))
        }

    val contentWidth: Float
        get() = max(availableWidth, inset * 2 + maxColumns * cardWidth + (maxColumns - 1) * columnGap)

    val contentHeight: Float
        get() {
            val rows = graph.rows.size.toFloat()
            return inset * 2 + rows * cardHeight + max(0f, rows - 1) * rowGap + if (hasUnreachable) unreachableLabel else 0f
        }

    fun rowTop(row: Int): Float {
        val extra = if (hasUnreachable && row == graph.rows.size - 1) unreachableLabel else 0f
        return inset + row * (cardHeight + rowGap) + extra
    }

    fun frame(node: ScreenGraph.Node): MapRect {
        val count = graph.rows[node.row].size
        val rowWidth = count * cardWidth + (count - 1) * columnGap
        val x = (contentWidth - rowWidth) / 2 + node.column * (cardWidth + columnGap)
        return MapRect(x, rowTop(node.row), cardWidth, cardHeight)
    }

    /** 描く矢印 1 本。[active] は選択中の画面の出入り(未選択なら全部)、[faded] は薄く描く。 */
    data class Arrow(val link: ScreenGraph.Link, val path: ArrowGeometry.ArrowPath, val active: Boolean, val faded: Boolean)

    /**
     * 描く矢印の一覧。上り・同じ段・往復は曲げて、逆向きの線やカードと重ならないようにする(自分への矢印は描かない)。
     * 到達できない画面から出る矢印と、選択中の画面に関係ない矢印は薄く。
     */
    fun arrows(selected: String?): List<Arrow> {
        val frames = graph.nodes.associate { it.id to frame(it) }
        val pairs = graph.links.map { it.id }.toSet()
        return graph.links.filter { it.from != it.to }.mapNotNull { link ->
            val a = frames[link.from] ?: return@mapNotNull null
            val b = frames[link.to] ?: return@mapNotNull null
            val from = graph.node(link.from)
            val to = graph.node(link.to)
            val reverse = "${link.to}->${link.from}" in pairs
            val curved = reverse || (to?.row ?: 0) <= (from?.row ?: 0)
            val sameRow = to?.row == from?.row
            val active = selected == null || selected == link.from || selected == link.to
            Arrow(link, ArrowGeometry.path(a, b, curved, below = sameRow && !reverse), active, faded = !active || from?.reachable != true)
        }
    }
}

/** 矢印の線と矢じり。線はカードの中心どうしを結びカードの縁で切る。曲げる時は進行方向の左へ膨らませる(往復は左右に分かれる)。 */
object ArrowGeometry {
    const val BEND = 34f
    const val HEAD_LENGTH = 9f

    /** [control] があれば 2 次ベジェ、無ければ直線。[head] は矢じりの 3 点(先端・左右)。 */
    data class ArrowPath(val start: MapPoint, val end: MapPoint, val control: MapPoint?, val head: List<MapPoint>)

    /** [below]: 同じ段どうしはカードの下を回る(段の間の線と重ねない) */
    fun path(a: MapRect, b: MapRect, curved: Boolean, below: Boolean = false): ArrowPath {
        val ca = MapPoint(a.midX, a.midY)
        val cb = MapPoint(b.midX, b.midY)
        val dx = cb.x - ca.x
        val dy = cb.y - ca.y
        val len = max(1f, hypot(dx, dy))
        val mid = MapPoint((ca.x + cb.x) / 2, (ca.y + cb.y) / 2)
        val control = when {
            below -> MapPoint(mid.x, max(a.maxY, b.maxY) + BEND)
            curved -> MapPoint(mid.x + dy / len * BEND, mid.y - dx / len * BEND)
            else -> mid
        }
        val start = edgePoint(a, control)
        val end = edgePoint(b, control)
        val ux = end.x - control.x
        val uy = end.y - control.y
        val ul = max(1f, hypot(ux, uy))
        val nx = ux / ul
        val ny = uy / ul
        val base = MapPoint(end.x - nx * HEAD_LENGTH, end.y - ny * HEAD_LENGTH)
        val w = HEAD_LENGTH * 0.55f
        val head = listOf(end, MapPoint(base.x - ny * w, base.y + nx * w), MapPoint(base.x + ny * w, base.y - nx * w))
        return ArrowPath(start, end, if (curved) control else null, head)
    }

    /** 矩形の中心から [p] へ向かう半直線と、矩形の縁(少し外側)との交点。 */
    fun edgePoint(r: MapRect, p: MapPoint): MapPoint {
        val cx = r.midX
        val cy = r.midY
        val dx = p.x - cx
        val dy = p.y - cy
        if (dx == 0f && dy == 0f) return MapPoint(cx, cy)
        val hw = r.width / 2 + 3
        val hh = r.height / 2 + 3
        val t = min(if (dx == 0f) Float.POSITIVE_INFINITY else hw / abs(dx), if (dy == 0f) Float.POSITIVE_INFINITY else hh / abs(dy))
        return MapPoint(cx + dx * t, cy + dy * t)
    }
}
