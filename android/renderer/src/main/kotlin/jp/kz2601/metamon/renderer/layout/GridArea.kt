package jp.kz2601.metamon.renderer.layout

import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.ComponentType
import jp.kz2601.metamon.dsl.Screen
import jp.kz2601.metamon.dsl.layout.gridPlacement as dslGridPlacement
import jp.kz2601.metamon.dsl.layout.localGridPlacement as dslLocalGridPlacement

// 画面グリッドの計算(Compose に依存しない)。area の解析は :dsl(iOS の MetamonDSL/Layout/GridArea.swift)に
// 1 つだけ置き、ここでは描画側だけの規則(段・寸法)を足す。iOS の FormFlowLayout.bands・LocalGridLayout と同じ。

typealias GridArea = jp.kz2601.metamon.dsl.layout.GridArea
typealias Placed = jp.kz2601.metamon.dsl.layout.Placed
typealias GridPlacement = jp.kz2601.metamon.dsl.layout.GridPlacement

val LOCAL_GRID_TYPES: Set<ComponentType> = jp.kz2601.metamon.dsl.layout.LOCAL_GRID_TYPES

val Screen.gridPlacement: GridPlacement get() = dslGridPlacement

val Component.localGridPlacement: List<Placed>? get() = dslLocalGridPlacement

/** voiceInput を含む画面はスクロールさせず全面に出す(iOS の `isVoiceScreen`)。 */
val Screen.isVoiceScreen: Boolean
    get() = components.any { c -> c.type == ComponentType.VOICE_INPUT || c.children.any { it.type == ComponentType.VOICE_INPUT } }

/** フォーム画面(entity 付きで area のある部品がある)は、area を並び順と幅だけに使う。 */
val Screen.isFormLayout: Boolean get() = entity != null && gridPlacement.placed.isNotEmpty()

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
