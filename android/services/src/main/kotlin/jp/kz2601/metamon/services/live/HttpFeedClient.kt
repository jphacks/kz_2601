package jp.kz2601.metamon.services.live

import jp.kz2601.metamon.dsl.feed.FeedQuery
import jp.kz2601.metamon.dsl.feed.FeedResult
import jp.kz2601.metamon.services.feeds.FeedClient
import jp.kz2601.metamon.services.http.HttpRequest
import jp.kz2601.metamon.services.http.MetamonHttp

/** appconfig の feeds API(#348・iOS の `HTTPFeedClient`)。POST で送り、diet の検索語などを URL に載せない。 */
class HttpFeedClient(private val http: MetamonHttp) : FeedClient {
    override suspend fun feed(query: FeedQuery): FeedResult {
        val body = http.send(HttpRequest("POST", query.path, body = query.body(), timeoutMillis = TIMEOUT_MILLIS))
        return FeedResult.decode(body)
    }

    private companion object {
        /** arxiv / diet はサーバーで順番待ち(10 秒まで)があるので長め(iOS と同じ 20 秒)。 */
        const val TIMEOUT_MILLIS = 20_000L
    }
}
