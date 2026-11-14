package jp.kz2601.metamon.services.feeds

import jp.kz2601.metamon.dsl.feed.FeedQuery
import jp.kz2601.metamon.dsl.feed.FeedResult

/** 外部の公開データ(#348・`POST /v1/feeds/{provider}`・iOS の `FeedClient`)。端末は appconfig にだけ聞き、外部へ直接つながらない。 */
fun interface FeedClient {
    suspend fun feed(query: FeedQuery): FeedResult
}
