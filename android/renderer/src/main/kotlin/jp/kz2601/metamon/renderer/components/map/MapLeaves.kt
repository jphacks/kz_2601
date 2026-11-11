package jp.kz2601.metamon.renderer.components.map

import jp.kz2601.metamon.dsl.ComponentType
import jp.kz2601.metamon.renderer.registry.ComponentRegistry

/** 地図の Leaf(MapLibre + OpenFreeMap・#867)。1 行 1 部品・名前順。placeSearch は場所の検索の API が無いのでまだ入れない。 */
fun ComponentRegistry.Builder.registerMapLeaves() {
    register(ComponentType.LOCATION_PICKER) { c, ctx, m -> LocationPickerLeaf(c, ctx, m) }
    register(ComponentType.MAP_VIEW) { c, ctx, m -> MapViewLeaf(c, ctx, m) }
}
