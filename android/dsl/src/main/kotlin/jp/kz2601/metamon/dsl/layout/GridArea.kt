package jp.kz2601.metamon.dsl.layout

import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.ComponentType
import jp.kz2601.metamon.dsl.Screen

/**
 * `area`("A1:F6")の解析結果(iOS の MetamonDSL/Layout/GridArea.swift)。
 * 列は 0 始まり(A=0)、行は 1 始まり、両端含む。
 * 画面は横 6 列(A-F)× 縦 N 行(1 行 = 可視高さ ÷ 12。13 行目以降は画面ごとスクロール)。
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

/** 部品の `area`。不正・未指定なら null。 */
val Component.gridArea: GridArea? get() = GridArea.parse(area)

/** 画面直下の部品を、絶対配置と積むものに分ける。 */
val Screen.gridPlacement: GridPlacement
    get() = GridPlacement(
        placed = components.mapNotNull { c -> c.gridArea?.let { Placed(c, it) } },
        stacked = components.filter { it.gridArea == null },
    )

/** 兄弟同士で `area` が重なっている組(backend も検証するが、クライアントでも検出できるように)。 */
val Screen.overlappingAreas: List<Pair<Component, Component>>
    get() {
        val placed = gridPlacement.placed
        return buildList {
            for (i in placed.indices) {
                for (j in i + 1 until placed.size) {
                    if (placed[i].area.overlaps(placed[j].area)) add(placed[i].component to placed[j].component)
                }
            }
        }
    }

/** 子に area を付けてローカルグリッドに並べられる Container(#81)。 */
val LOCAL_GRID_TYPES: Set<ComponentType> = setOf(ComponentType.CARD, ComponentType.SECTION, ComponentType.VSTACK)

/**
 * Container 内グリッド(#81)。card / section / vstack の子が**全員**有効な area を持つ時だけ配置を返す
 * (A1 = Container の左上)。一部だけ・不正・対象外の型なら null(従来どおり積む)。
 */
val Component.localGridPlacement: List<Placed>?
    get() {
        if (type !in LOCAL_GRID_TYPES || children.isEmpty()) return null
        val placed = children.mapNotNull { c -> c.gridArea?.let { Placed(c, it) } }
        return if (placed.size == children.size) placed else null
    }
