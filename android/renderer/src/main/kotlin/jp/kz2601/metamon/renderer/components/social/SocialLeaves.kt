package jp.kz2601.metamon.renderer.components.social

import jp.kz2601.metamon.dsl.ComponentType
import jp.kz2601.metamon.renderer.registry.ComponentRegistry

/** みんなで使う部品(#875・#877)。1 行 1 部品・名前順。 */
fun ComponentRegistry.Builder.registerSocialLeaves() {
    register(ComponentType.CHAT_THREAD) { c, ctx, m -> ChatThreadLeaf(c, ctx, m) }
    register(ComponentType.MATCH_LIST) { c, ctx, m -> MatchListLeaf(c, ctx, m) }
}
