package jp.kz2601.metamon.renderer.components.dashcam

import jp.kz2601.metamon.dsl.ComponentType
import jp.kz2601.metamon.renderer.registry.ComponentRegistry

/** ドラレコの Leaf(#872)。1 行 1 部品・名前順。 */
fun ComponentRegistry.Builder.registerDashcamLeaves() {
    register(ComponentType.DASHCAM) { c, ctx, m -> DashcamLeaf(c, ctx, m) }
}
