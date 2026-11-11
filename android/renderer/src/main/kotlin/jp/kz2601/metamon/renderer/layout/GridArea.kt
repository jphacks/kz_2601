package jp.kz2601.metamon.renderer.layout

import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.ComponentType
import jp.kz2601.metamon.dsl.Screen

// 画面グリッドの計算(Compose に依存しない)。iOS の MetamonDSL/Layout/GridArea.swift と
// MetamonRenderer の FormFlowLayout.bands・LocalGridLayout と同じ規則。

/**
 * `area`("A1:F6")の解析結果。列は 0 始まり(A=0)、行は 1 始まり、両端含む。
 * 画面は横 6 列 × 縦 N 行(1 行 = 可視高さ ÷ 12。13 行目以降は画面ごとスクロール)。
 */
data class GridArea(val col1: Int, val row1: Int, val col2: Int, val row2: Int) {
    val colSpan: Int get() = col2 - col1 + 1
    val rowSpan: Int get() = row2 - row1 + 1

    fun overlaps(o: GridArea): Boolean = col1 <= o.col2 && o.col1 <= col2 && row1 <= o.row2 && o.row1 <= row2

    companion object {
        const val COLUMNS = 6
        const val VISIBLE_ROWS = 12

        /** 行の上限(backend `dsl.MaxGridRows` と同じ)。超える行はここに丸める。 */
        const val MAX_ROWS = 48

        /** 解析できなければ null(その部品は下に積む)。 */
        fun parse(s: String?): GridArea? {
            val parts = s?.split(":") ?: return null
            if (parts.size != 2) return null
            val a = cell(parts[0]) ?: return null
            val b = cell(parts[1]) ?: return null
            if (a.first > b.first || a.second > b.second) return null
            return GridArea(a.first, a.second, b.first, b.second)
        }

        private fun cell(s: String): Pair<Int, Int>? {
            val first = s.firstOrNull() ?: return null
            if (first !in 'A'..'F') return null
            val digits = s.drop(1)
            if (digits.isEmpty() || !digits.all { it in '0'..'9' }) return null
            val row = digits.toIntOrNull() ?: return null
            if (row < 1) return null
            return (first - 'A') to minOf(row, MAX_ROWS)
        }
    }
}

/** area の付いた部品。 */
data class Placed(val component: Component, val area: GridArea)

/** 画面直下の部品の配置: area 付き(絶対配置)と、area 無し(下に全幅で積む)。 */
data class GridPlacement(val placed: List<Placed>, val stacked: List<Component>) {
    /** area で使われた最終行(1 始まり)。無ければ 0。 */
    val lastRow: Int get() = placed.maxOfOrNull { it.area.row2 } ?: 0

    /** (行, 列, DSL の順)で並べた配置(描画の木の順)。 */
    val placedInReadingOrder: List<Placed>
        get() = placed.withIndex()
            .sortedWith(compareBy({ it.value.area.row1 }, { it.value.area.col1 }, { it.index }))
            .map { it.value }
}

val Screen.gridPlacement: GridPlacement
    get() {
        val placed = components.mapNotNull { c -> GridArea.parse(c.area)?.let { Placed(c, it) } }
        val stacked = components.filter { GridArea.parse(it.area) == null }
        return GridPlacement(placed, stacked)
    }

/** voiceInput を含む画面はスクロールさせず全面に出す(iOS の `isVoiceScreen`)。 */
val Screen.isVoiceScreen: Boolean
    get() = components.any { c -> c.type == ComponentType.VOICE_INPUT || c.children.any { it.type == ComponentType.VOICE_INPUT } }

/** フォーム画面(entity 付きで area のある部品がある)は、area を並び順と幅だけに使う。 */
val Screen.isFormLayout: Boolean get() = entity != null && gridPlacement.placed.isNotEmpty()

/** 子に area を付けてローカルグリッドに並べられる Container(#81)。 */
val LOCAL_GRID_TYPES: Set<ComponentType> = setOf(ComponentType.CARD, ComponentType.SECTION, ComponentType.VSTACK)

/**
 * Container 内グリッド。card / section / vstack の子が**全員**有効な area を持つ時だけ配置を返す
 * (A1 = Container の左上)。一部だけ・不正・対象外の型なら null(従来どおり積む)。
 */
val Component.localGridPlacement: List<Placed>?
    get() {
        if (type !in LOCAL_GRID_TYPES || children.isEmpty()) return null
        val placed = children.mapNotNull { c -> GridArea.parse(c.area)?.let { Placed(c, it) } }
        return if (placed.size == children.size) placed else null
    }

/** フォーム画面の段。行範囲が重なる部品を 1 段にまとめる。 */
data class Band(val items: List<Placed>, val row1: Int, val row2: Int)

/** row1 → col1 の順に並べ、行範囲が重なるものを同じ段にまとめる(iOS の `FormFlowLayout.bands`)。 */
fun formBands(placed: List<Placed>): List<Band> =
    bandIndices(placed.map { it.area }).map { idx ->
        val items = idx.map { placed[it] }
        Band(items, items.first().area.row1, items.maxOf { it.area.row2 })
    }

/** [formBands] の添字版(段ごとの、元の並びでの添字)。ローカルグリッドの流し込みにも使う。 */
fun bandIndices(areas: List<GridArea>): List<List<Int>> {
    val order = areas.indices.sortedWith(compareBy({ areas[it].row1 }, { areas[it].col1 }))
    val out = mutableListOf<MutableList<Int>>()
    var bandEnd = 0
    for (i in order) {
        if (out.isNotEmpty() && areas[i].row1 <= bandEnd) {
            out.last() += i
            bandEnd = maxOf(bandEnd, areas[i].row2)
        } else {
            out += mutableListOf(i)
            bandEnd = areas[i].row2
        }
    }
    return out
}

/** 前の段から 3 行以上空いた段(最下段の削除ボタン等)は少し間を空けて続ける。 */
fun Band.isDetached(previous: Band?): Boolean = previous != null && row1 - previous.row2 > 2

/**
 * 画面グリッドの寸法(iOS の `GridMetrics`)。単位は dp の数値。
 * 縦向きは「可視幅 ÷ 6 × 可視高さ ÷ 12」。横向き(高さが compact)は幅を絞って中央に寄せ、行の高さを固定してスクロール。
 */
data class GridMetrics(
    val frameWidth: Float,
    val cellW: Float,
    val cellH: Float,
    val scrollsVertically: Boolean,
) {
    companion object {
        const val MAX_LANDSCAPE_WIDTH = 640f
        const val LANDSCAPE_CELL_HEIGHT = 52f
        const val FAB_GUTTER = 96f
        const val MIN_LANDSCAPE_WIDTH = 420f

        fun resolve(width: Float, height: Float, inset: Float, compactHeight: Boolean): GridMetrics {
            val columns = GridArea.COLUMNS.toFloat()
            if (!compactHeight) {
                return GridMetrics(
                    frameWidth = width,
                    cellW = maxOf(0f, width - inset * 2) / columns,
                    cellH = maxOf(0f, height - inset * 2) / GridArea.VISIBLE_ROWS,
                    scrollsVertically = false,
                )
            }
            val withGutter = minOf(width - FAB_GUTTER * 2, MAX_LANDSCAPE_WIDTH)
            val frameWidth = if (withGutter >= MIN_LANDSCAPE_WIDTH) withGutter else minOf(width, MAX_LANDSCAPE_WIDTH)
            return GridMetrics(
                frameWidth = frameWidth,
                cellW = maxOf(0f, frameWidth - inset * 2) / columns,
                cellH = maxOf(LANDSCAPE_CELL_HEIGHT, maxOf(0f, height - inset * 2) / GridArea.VISIBLE_ROWS),
                scrollsVertically = true,
            )
        }
    }
}
