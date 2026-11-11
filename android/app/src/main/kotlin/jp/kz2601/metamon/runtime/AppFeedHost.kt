package jp.kz2601.metamon.runtime

import jp.kz2601.metamon.dsl.feed.FeedQuery
import jp.kz2601.metamon.dsl.feed.FeedResult
import jp.kz2601.metamon.renderer.components.feed.FeedHost
import jp.kz2601.metamon.services.feeds.FeedCache
import jp.kz2601.metamon.services.feeds.FeedClient

/** feed のホスト側(iOS の `AppRuntime+Feeds`)。appconfig の feeds API を読み、[FeedCache] が 10 分覚える(端末にも置く)。 */
class AppFeedHost(private val client: FeedClient, private val cache: FeedCache) : FeedHost {
    override suspend fun feed(query: FeedQuery, force: Boolean): FeedResult = cache.load(query, force, client)

    override suspend fun cachedFeed(query: FeedQuery): FeedResult? = cache.cached(query)
}
