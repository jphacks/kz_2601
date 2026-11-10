package jp.kz2601.metamon.renderer.components.location

import jp.kz2601.metamon.dsl.ComponentType
import jp.kz2601.metamon.renderer.registry.ComponentRegistry

/** 位置の Leaf(#867)。地図の提供元に依らないものだけ。1 行 1 部品・名前順。 */
fun ComponentRegistry.Builder.registerLocationLeaves() {
    register(ComponentType.LOCATION_BUTTON) { c, ctx, m -> LocationButtonLeaf(c, ctx, m) }
}
