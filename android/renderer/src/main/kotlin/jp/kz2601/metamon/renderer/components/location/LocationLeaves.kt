package jp.kz2601.metamon.renderer.components.location

import jp.kz2601.metamon.dsl.ComponentType
import jp.kz2601.metamon.renderer.registry.ComponentRegistry

/** 位置の Leaf(#867・#869)。地図を使わないものだけ(地図は components/map/MapLeaves.kt)。1 行 1 部品・名前順。 */
fun ComponentRegistry.Builder.registerLocationLeaves() {
    register(ComponentType.LOCATION_BUTTON) { c, ctx, m -> LocationButtonLeaf(c, ctx, m) }
    register(ComponentType.LOCATION_TRACKER) { c, ctx, m -> LocationTrackerLeaf(c, ctx, m) } // #869
}
