package jp.kz2601.metamon.renderer.components.media

import jp.kz2601.metamon.dsl.ComponentType
import jp.kz2601.metamon.renderer.registry.ComponentRegistry

/** 写真と手書きの Leaf(#862)。1 行 1 部品・名前順。 */
fun ComponentRegistry.Builder.registerMediaLeaves() {
    register(ComponentType.DRAWING_FIELD) { c, ctx, m -> DrawingFieldLeaf(c, ctx, m) }
    register(ComponentType.PHOTO_FIELD) { c, ctx, m -> PhotoFieldLeaf(c, ctx, m) }
}
