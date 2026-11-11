package jp.kz2601.metamon.renderer.screen

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.BoxWithConstraints
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.offset
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.compositionLocalOf
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.layout.Layout
import androidx.compose.ui.layout.Placeable
import androidx.compose.ui.unit.Constraints
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.ComponentType
import jp.kz2601.metamon.dsl.Screen
import jp.kz2601.metamon.renderer.components.DSLComponent
import jp.kz2601.metamon.renderer.layout.GridArea
import jp.kz2601.metamon.renderer.layout.GridMetrics
import jp.kz2601.metamon.renderer.layout.Placed
import jp.kz2601.metamon.renderer.layout.bandIndices
import jp.kz2601.metamon.renderer.layout.formBands
import jp.kz2601.metamon.renderer.layout.gridPlacement
import jp.kz2601.metamon.renderer.layout.isDetached
import jp.kz2601.metamon.renderer.layout.isFormLayout
import jp.kz2601.metamon.renderer.layout.isVoiceScreen
import jp.kz2601.metamon.renderer.layout.localGridPlacement
import jp.kz2601.metamon.renderer.registry.RenderContext
import jp.kz2601.metamon.renderer.theme.RenderTheme

// 画面グリッド(iOS の Screen/GridScreenLayout.swift・Containers/LocalGrid.swift)。
// - 横 6 列 = 可視幅 ÷ 6、縦 1 行 = 可視高さ ÷ 12。area を持つ直下の子は矩形に絶対配置。
// - area の無い子は、グリッドの下に全幅で上から積む。
// - 13 行目以降・積んだ分がはみ出せば画面ごと縦スクロール。voiceInput の画面はスクロールしない。
// - フォーム画面(screen.entity)は area を「並び順と横幅」だけに使い、段ごとに中身の高さで積む(FormFlowLayout)。

/** 高さが compact(横向き)とみなす高さ。 */
private val COMPACT_HEIGHT = 480.dp

@Composable
fun GridScreenLayout(screen: Screen, ctx: RenderContext, modifier: Modifier = Modifier) {
    val gap = RenderTheme.Spacing.gridGap
    val inset = RenderTheme.Spacing.screenInset
    BoxWithConstraints(modifier.fillMaxSize()) {
        val metrics = GridMetrics.resolve(
            width = maxWidth.value,
            height = maxHeight.value,
            inset = inset.value,
            compactHeight = maxHeight < COMPACT_HEIGHT,
        )
        val cellW = metrics.cellW.dp
        val cellH = metrics.cellH.dp
        val placement = screen.gridPlacement
        val scrolls = !screen.isVoiceScreen || metrics.scrollsVertically
        val scroll = if (scrolls) Modifier.verticalScroll(rememberScrollState()) else Modifier

        Box(Modifier.fillMaxSize().then(scroll), contentAlignment = Alignment.TopCenter) {
            Column(
                Modifier.width(metrics.frameWidth.dp).padding(inset),
                verticalArrangement = Arrangement.spacedBy(gap),
            ) {
                if (screen.isFormLayout) {
                    FormFlowLayout(placement.placed, cellW, cellH, ctx)
                } else if (placement.placed.isNotEmpty()) {
                    Box(Modifier.fillMaxWidth().height(cellH * placement.lastRow)) {
                        placement.placed.forEach { (c, a) ->
                            GridCell(
                                c, ctx,
                                Modifier
                                    .offset(x = cellW * a.col1 + gap / 2, y = cellH * (a.row1 - 1) + gap / 2)
                                    .size(width = cellW * a.colSpan - gap, height = cellH * a.rowSpan - gap)
                                    .changeMark(screen.id, c, screen.components.indexOf(c)),
                            )
                        }
                    }
                }
                // 変わった部品の印(#894)
                placement.stacked.forEach { DSLComponent(it, ctx, Modifier.changeMark(screen.id, it, screen.components.indexOf(it))) }
            }
        }
    }
}

/** 面を自前で埋める型(フォームの段・流し込みのローカルグリッドで最低限の高さを保つ)。 */
private val FILLS_SURFACE = setOf(
    ComponentType.IMAGE, ComponentType.MAP_VIEW, ComponentType.WEB_VIEW, ComponentType.CHART, ComponentType.GAUGE,
    ComponentType.CALENDAR, ComponentType.GRID_TABLE, ComponentType.QR_SCANNER, ComponentType.QR_CODE,
    ComponentType.LIST, ComponentType.TABLE, ComponentType.TIMELINE,
)

/** フォーム画面の配置。行が重なる部品を 1 段にまとめ、段ごとに上から積む。横位置・幅は area のまま。 */
@Composable
fun FormFlowLayout(placed: List<Placed>, cellW: Dp, cellH: Dp, ctx: RenderContext) {
    val gap = RenderTheme.Spacing.gridGap
    val bands = formBands(placed)
    Column(verticalArrangement = Arrangement.spacedBy(gap)) {
        bands.forEachIndexed { index, band ->
            val detached = band.isDetached(bands.getOrNull(index - 1))
            Box(Modifier.fillMaxWidth().padding(top = if (detached) RenderTheme.Spacing.md else 0.dp)) {
                band.items.forEach { (c, a) ->
                    val m = Modifier
                        .offset(x = cellW * a.col1 + gap / 2)
                        .width(cellW * a.colSpan - gap)
                    if (c.type in FILLS_SURFACE) {
                        DSLComponent(c, ctx, m.height(maxOf(cellH * 2, cellH * a.rowSpan - gap)))
                    } else {
                        DSLComponent(c, ctx, m)
                    }
                }
            }
        }
    }
}

/** 中身が伸びるのでセルの内側でスクロールさせる型。 */
private val SCROLLS_INSIDE = setOf(
    ComponentType.LIST, ComponentType.SECTION, ComponentType.VSTACK, ComponentType.TEXT, ComponentType.TABLE,
    ComponentType.TIMELINE, ComponentType.MARKDOWN, ComponentType.CARD, ComponentType.DISCLOSURE, ComponentType.GRID,
    ComponentType.CALENDAR, ComponentType.GRID_TABLE, ComponentType.SETTLEMENT, ComponentType.MATCH_LIST, ComponentType.FEED,
)

/** Container 内グリッドを器いっぱいに置くか(GridCell が area 付き Container を描く時だけ true)。 */
internal val LocalGridFills = compositionLocalOf { false }

/** セル = 長方形の器。中身がはみ出す型は器の内側でスクロールする。 */
@Composable
fun GridCell(component: Component, ctx: RenderContext, modifier: Modifier = Modifier) {
    val hasLocalGrid = component.localGridPlacement != null
    val scrollsInside = !hasLocalGrid && component.type in SCROLLS_INSIDE
    if (scrollsInside) {
        Box(modifier.verticalScroll(rememberScrollState())) { DSLComponent(component, ctx) }
    } else {
        Box(modifier) {
            CompositionLocalProvider(LocalGridFills provides hasLocalGrid) {
                DSLComponent(component, ctx, if (hasLocalGrid) Modifier.fillMaxSize() else Modifier)
            }
        }
    }
}

/**
 * Container 内グリッド(#81)。card / section / vstack の子が全員 area を持つ時、中身の矩形を横 6 列のローカルグリッドにする。
 * - 器いっぱい(`LocalGridFills`): 1 行 = 中身の高さ ÷ 子が使う最終行。子は GridCell で描く。
 * - 流し込み(既定): 横位置と幅だけ効かせ、行が重なる子を 1 段にまとめて上から積む。
 */
@Composable
fun LocalGrid(placed: List<Placed>, ctx: RenderContext, modifier: Modifier = Modifier) {
    val fills = LocalGridFills.current
    val gap = RenderTheme.Spacing.gridGap
    Layout(
        content = {
            placed.forEach { (c, a) ->
                when {
                    fills -> GridCell(c, ctx)
                    c.type in FILLS_SURFACE -> DSLComponent(c, ctx, Modifier.heightIn(min = maxOf(120.dp, 60.dp * a.rowSpan)))
                    else -> DSLComponent(c, ctx)
                }
            }
        },
        modifier = if (fills) modifier.fillMaxSize() else modifier.fillMaxWidth(),
    ) { measurables, constraints ->
        val gapPx = gap.roundToPx().toFloat()
        val total = constraints.maxWidth.toFloat()
        val colW = (total + gapPx) / GridArea.COLUMNS
        fun width(a: GridArea) = maxOf(0, (a.colSpan * colW - gapPx).toInt())
        val areas = placed.map { it.area }

        if (fills && constraints.hasBoundedHeight) {
            val lastRow = maxOf(1, areas.maxOf { it.row2 })
            val rowH = (constraints.maxHeight + gapPx) / lastRow
            val placeables = measurables.mapIndexed { i, m ->
                val a = areas[i]
                val h = maxOf(0, (a.rowSpan * rowH - gapPx).toInt())
                m.measure(Constraints.fixed(width(a), h))
            }
            layout(constraints.maxWidth, constraints.maxHeight) {
                placeables.forEachIndexed { i, p ->
                    p.place((areas[i].col1 * colW).toInt(), ((areas[i].row1 - 1) * rowH).toInt())
                }
            }
        } else {
            val bands = bandIndices(areas)
            val ys = IntArray(placed.size)
            val placeables = arrayOfNulls<Placeable>(placed.size)
            var y = 0
            bands.forEachIndexed { n, band ->
                var h = 0
                band.forEach { i ->
                    val w = width(areas[i])
                    val p = measurables[i].measure(Constraints(minWidth = w, maxWidth = w))
                    placeables[i] = p
                    ys[i] = y
                    h = maxOf(h, p.height)
                }
                y += h + if (n == bands.lastIndex) 0 else gapPx.toInt()
            }
            layout(constraints.maxWidth, y) {
                placeables.forEachIndexed { i, p -> p?.place((areas[i].col1 * colW).toInt(), ys[i]) }
            }
        }
    }
}
