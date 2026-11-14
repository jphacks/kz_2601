package jp.kz2601.metamon.renderer.components.feed

import jp.kz2601.metamon.dsl.ComponentType
import jp.kz2601.metamon.renderer.registry.ComponentRegistry

/** 外部の公開データの部品(#876)。1 行 1 部品・名前順。データは [FeedHost](アプリが appconfig から取る)。 */
fun ComponentRegistry.Builder.registerFeedLeaves() {
    register(ComponentType.FEED) { c, ctx, m -> FeedLeaf(c, ctx, m) }
}
