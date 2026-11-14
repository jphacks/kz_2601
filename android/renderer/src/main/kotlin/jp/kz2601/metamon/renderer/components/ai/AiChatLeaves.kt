package jp.kz2601.metamon.renderer.components.ai

import jp.kz2601.metamon.dsl.ComponentType
import jp.kz2601.metamon.renderer.registry.ComponentRegistry

/** AI と話す Leaf(#874)。1 行 1 部品・名前順。 */
fun ComponentRegistry.Builder.registerAiChatLeaves() {
    register(ComponentType.AI_CHAT) { c, ctx, m -> AiChatLeaf(c, ctx, m) }
}
