package jp.kz2601.metamon.renderer.components.health

import jp.kz2601.metamon.dsl.ComponentType
import jp.kz2601.metamon.renderer.registry.ComponentRegistry

/** ヘルスの Leaf(#870)。1 行 1 部品・名前順。 */
fun ComponentRegistry.Builder.registerHealthLeaves() {
    register(ComponentType.HEALTH_STAT) { c, ctx, m -> HealthStatLeaf(c, ctx, m) }
}
