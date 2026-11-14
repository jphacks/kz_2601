package jp.kz2601.metamon.renderer.components.ai

import jp.kz2601.metamon.dsl.ComponentType
import jp.kz2601.metamon.renderer.registry.ComponentRegistry

/** AI の Leaf(#873)。1 行 1 部品・名前順。 */
fun ComponentRegistry.Builder.registerAiLeaves() {
    register(ComponentType.AI_BUTTON) { c, ctx, m -> AiButtonLeaf(c, ctx, m) }
}
