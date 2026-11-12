package jp.kz2601.metamon.desktop

import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.ComponentType
import jp.kz2601.metamon.dsl.Screen

// 画面グリッドの規則(Compose に依存しない)。Android の renderer/layout/GridArea.kt の写し(iOS の GridArea.swift・FormFlowLayout と同じ)。
// :renderer-core(#1739)ができたらそちらを使い、この写しは消す。

/** `area`("A1:F6")。列は 0 始まり(A=0)、行は 1 始まり、両端含む。 */
data class GridArea(val col1: Int, val row1: Int, val col2: Int, val row2: Int) {
    val colSpan: Int get() = col2 - col1 + 1
    val rowSpan: Int get() = row2 - row1 + 1

    companion object {
        const val COLUMNS = 6
        const val VISIBLE_ROWS = 12

        /** 行の上限(backend `dsl.MaxGridRows` と同じ)。 */
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

data class Placed(val component: Component, val area: GridArea)

/** 画面直下の部品: area 付き(絶対配置)と area 無し(下に全幅で積む)。 */
data class GridPlacement(val placed: List<Placed>, val stacked: List<Component>) {
    val lastRow: Int get() = placed.maxOfOrNull { it.area.row2 } ?: 0

    /** (行, 列, DSL の順)。描画の木の順。 */
    val placedInReadingOrder: List<Placed>
        get() = placed.withIndex()
            .sortedWith(compareBy({ it.value.area.row1 }, { it.value.area.col1 }, { it.index }))
            .map { it.value }
}

fun gridPlacement(components: List<Component>): GridPlacement =
    GridPlacement(
        placed = components.mapNotNull { c -> GridArea.parse(c.area)?.let { Placed(c, it) } },
        stacked = components.filter { GridArea.parse(it.area) == null },
    )

val Screen.gridPlacement: GridPlacement get() = gridPlacement(components)

val Screen.isVoiceScreen: Boolean
    get() = components.any { c -> c.type == ComponentType.VOICE_INPUT || c.children.any { it.type == ComponentType.VOICE_INPUT } }

/** フォーム画面(entity 付きで area のある部品がある)は area を並び順と幅だけに使う。 */
val Screen.isFormLayout: Boolean get() = entity != null && gridPlacement.placed.isNotEmpty()

/** 適合テストの `layout`。 */
val Screen.layoutKind: String get() = if (isVoiceScreen) "voice" else if (isFormLayout) "form" else "grid"

private val LOCAL_GRID_TYPES = setOf(ComponentType.CARD, ComponentType.SECTION, ComponentType.VSTACK)

/** Container 内グリッド。子が全員 area を持つ card / section / vstack だけ(A1 = Container の左上)。 */
val Component.localGridPlacement: List<Placed>?
    get() {
        if (type !in LOCAL_GRID_TYPES || children.isEmpty()) return null
        val placed = children.mapNotNull { c -> GridArea.parse(c.area)?.let { Placed(c, it) } }
        return if (placed.size == children.size) placed else null
    }

/** フォーム画面の段。行範囲が重なる部品を 1 段にまとめる。 */
data class Band(val items: List<Placed>, val row1: Int, val row2: Int) {
    /** 前の段から 3 行以上空いた段(最下段の削除ボタン等)は少し間を空ける。 */
    fun isDetached(previous: Band?): Boolean = previous != null && row1 - previous.row2 > 2
}

fun formBands(placed: List<Placed>): List<Band> {
    val order = placed.indices.sortedWith(compareBy({ placed[it].area.row1 }, { placed[it].area.col1 }))
    val out = mutableListOf<MutableList<Placed>>()
    var bandEnd = 0
    for (i in order) {
        val a = placed[i].area
        if (out.isNotEmpty() && a.row1 <= bandEnd) {
            out.last() += placed[i]
            bandEnd = maxOf(bandEnd, a.row2)
        } else {
            out += mutableListOf(placed[i])
            bandEnd = a.row2
        }
    }
    return out.map { items -> Band(items, items.first().area.row1, items.maxOf { it.area.row2 }) }
}

/**
 * デスクトップのグリッドの寸法(dp の数)。iOS / Android の縦向きは「幅 ÷ 6 × 高さ ÷ 12」。
 * デスクトップは横長・大きいウィンドウがあるので、幅は [MAX_WIDTH] で止めて中央に寄せ(Mac の mac-016 と同じ考え)、
 * 行の高さは [MIN_CELL_HEIGHT]〜[MAX_CELL_HEIGHT] に収める(低いウィンドウでは画面ごとスクロール)。
 */
data class DesktopGridMetrics(val frameWidth: Float, val cellW: Float, val cellH: Float) {
    companion object {
        const val MAX_WIDTH = 720f
        const val MIN_CELL_HEIGHT = 44f
        const val MAX_CELL_HEIGHT = 96f

        fun resolve(width: Float, height: Float, inset: Float): DesktopGridMetrics {
            val frame = minOf(width, MAX_WIDTH)
            val cellW = maxOf(0f, frame - inset * 2) / GridArea.COLUMNS
            val cellH = (maxOf(0f, height - inset * 2) / GridArea.VISIBLE_ROWS).coerceIn(MIN_CELL_HEIGHT, MAX_CELL_HEIGHT)
            return DesktopGridMetrics(frame, cellW, cellH)
        }
    }
}
