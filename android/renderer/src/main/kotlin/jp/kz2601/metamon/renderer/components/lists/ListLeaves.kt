package jp.kz2601.metamon.renderer.components.lists

import jp.kz2601.metamon.dsl.ComponentType
import jp.kz2601.metamon.renderer.registry.ComponentRegistry

/** 一覧・集計・グラフの Leaf 10 種(#859)。1 行 1 部品・名前順。 */
fun ComponentRegistry.Builder.registerListLeaves() {
    register(ComponentType.CALENDAR) { c, ctx, m -> CalendarLeaf(c, ctx, m) }
    register(ComponentType.CHART) { c, ctx, m -> ChartLeaf(c, ctx, m) }
    register(ComponentType.GAUGE) { c, ctx, m -> GaugeLeaf(c, ctx, m) }
    register(ComponentType.GRID_TABLE) { c, ctx, m -> GridTableLeaf(c, ctx, m) }
    register(ComponentType.LIST) { c, ctx, m -> PublicListNotice(ctx.runtime.entityModel(c.entity ?: "")?.scope, m) { mod -> ListLeaf(c, ctx, mod) } } // #253 公開の表示
    register(ComponentType.PROGRESS_BAR) { c, ctx, m -> ProgressBarLeaf(c, ctx, m) }
    register(ComponentType.SEARCH_BAR) { c, ctx, m -> SearchBarLeaf(c, ctx, m) }
    register(ComponentType.STAT) { c, ctx, m -> StatLeaf(c, ctx, m) }
    register(ComponentType.TABLE) { c, ctx, m -> TableLeaf(c, ctx, m) }
    register(ComponentType.TIMELINE) { c, ctx, m -> TimelineLeaf(c, ctx, m) }
}
