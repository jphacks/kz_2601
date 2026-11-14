package jp.kz2601.metamon.renderer.components.nearby

import jp.kz2601.metamon.dsl.ComponentType
import jp.kz2601.metamon.renderer.registry.ComponentRegistry

/** 近くの端末と送り合う Leaf(#1382)。1 行 1 部品・名前順。 */
fun ComponentRegistry.Builder.registerNearbyLeaves() {
    register(ComponentType.NEARBY_SHARE) { c, ctx, m -> NearbyShareLeaf(c, ctx, m) }
}
