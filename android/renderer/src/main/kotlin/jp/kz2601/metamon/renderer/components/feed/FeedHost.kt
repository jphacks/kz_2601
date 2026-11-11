package jp.kz2601.metamon.renderer.components.feed

import androidx.compose.runtime.staticCompositionLocalOf
import jp.kz2601.metamon.dsl.feed.FeedQuery
import jp.kz2601.metamon.dsl.feed.FeedResult
import jp.kz2601.metamon.renderer.DSLRuntime

/**
 * 外部の公開データ(#348・iOS の `FeedHost`)。appconfig の `POST /v1/feeds/{provider}` を読む。端末は外部へ直接つながらない。
 * 持たない時(モック・プレビュー・テスト)は部品が「読み込めませんでした」を出す。
 */
interface FeedHost {
    /** 同じ問い合わせはホストがしばらく覚えておく(画面を行き来するたびに取りに行かない)。[force] で取り直す。 */
    suspend fun feed(query: FeedQuery, force: Boolean): FeedResult

    /** 覚えている行(古くても・取りに行かない)。取りに行く前に先に出す。無ければ null。 */
    suspend fun cachedFeed(query: FeedQuery): FeedResult? = null
}

/** アプリが渡すホスト(:app が DSLView の外から入れる)。ランタイム自身が [FeedHost] ならそちらが先。 */
val LocalFeedHost = staticCompositionLocalOf<FeedHost?> { null }

/** ランタイムが [FeedHost] を兼ねていればそれ(iOS の `DSLRuntime.feeds`)。 */
val DSLRuntime.feeds: FeedHost? get() = this as? FeedHost
