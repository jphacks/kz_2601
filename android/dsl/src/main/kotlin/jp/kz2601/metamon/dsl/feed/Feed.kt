package jp.kz2601.metamon.dsl.feed

import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.DslJson
import jp.kz2601.metamon.dsl.LossyListSerializer
import jp.kz2601.metamon.dsl.Provider
import kotlinx.serialization.KSerializer
import kotlinx.serialization.Serializable
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.put
import java.net.URI

/**
 * `feed` が読むもの(`POST /v1/feeds/{provider}`・#348・iOS の `FeedQuery`)。URL は持たない: provider と、その型の決まったパラメータだけ。
 */
data class FeedQuery(
    val provider: String,
    val region: String? = null,
    val feedId: String? = null,
    /** diet: 国会会議録で探す語(部品の `query`)。 */
    val q: String? = null,
    val limit: Int = 5,
) {
    /** `POST v1/feeds/{provider}` の宛先(区切りごと)。パラメータは本文で送り、URL に載せない。 */
    val path: List<String> get() = listOf("v1", "feeds", provider)

    /** POST の本文 `{"region","feedId","q","limit"}`(無いものは送らない)。 */
    fun body(): String = buildJsonObject {
        region?.let { put("region", it) }
        feedId?.let { put("feedId", it) }
        q?.let { put("q", it) }
        put("limit", limit)
    }.toString()

    /** 端末のキャッシュの鍵。 */
    val cacheKey: String get() = listOf(provider, region.orEmpty(), feedId.orEmpty(), q.orEmpty(), limit.toString()).joinToString("|")

    companion object {
        val providers: Set<String> = setOf("weather", "holidays", "rss", "warnings", "pressure", "arxiv", "diet")

        /** `feedId`(許可した id)を読む provider。 */
        val feedIdProviders: Set<String> = setOf("rss", "arxiv")

        /** `region`(府県予報区のコード)を読む provider。 */
        val regionProviders: Set<String> = setOf("weather", "warnings", "pressure")

        /** 部品から。provider が知らない値・diet で語が空なら null(描かずに「使えません」の枠)。 */
        fun of(c: Component): FeedQuery? {
            val p = c.provider?.takeIf { it != Provider.UNKNOWN }?.wire ?: return null
            if (p !in providers) return null
            if (p == "diet" && c.query.orEmpty().trim().isEmpty()) return null
            return FeedQuery(
                provider = p,
                region = if (p in regionProviders) c.region else null,
                feedId = if (p in feedIdProviders) c.feedId else null,
                q = if (p == "diet") c.query else null,
                limit = (c.limit ?: 5).coerceIn(1, 20),
            )
        }
    }
}

/** サーバーの答え。知らないキーは無視し、壊れた行だけ落とす(iOS の `FeedResult`)。 */
@Serializable
data class FeedResult(
    val provider: String,
    val title: String = "",
    @Serializable(with = FeedItemListSerializer::class)
    val items: List<FeedItem> = emptyList(),
    val source: FeedSource,
    val stale: Boolean = false,
) {
    companion object {
        /** 応答の JSON を読む。provider・source が無い・JSON でなければ投げる。 */
        fun decode(json: String): FeedResult = DslJson.decodeFromString(serializer(), json)

        fun encode(result: FeedResult): String = DslJson.encodeToString(serializer(), result)
    }
}

internal object FeedItemListSerializer : KSerializer<List<FeedItem>> by LossyListSerializer(FeedItem.serializer())

@Serializable
data class FeedItem(
    val date: String? = null,
    val title: String,
    val subtitle: String? = null,
    val icon: String? = null,
    val url: String? = null,
    val max: Double? = null,
    val min: Double? = null,
    val pop: Int? = null,
    /** pressure: 24 時間以内に 6 hPa 以上下がる。 */
    val drop: Boolean? = null,
    /** pressure: 行の中の 1 時間ごとの気圧(hPa)。 */
    val series: List<Double>? = null,
) {
    val id: String get() = "${date.orEmpty()}|$title|${url.orEmpty()}"

    /** 開ける記事のリンク(https でホストのあるものだけ)。 */
    val link: String?
        get() {
            val u = url ?: return null
            val parsed = runCatching { URI(u) }.getOrNull() ?: return null
            return if (parsed.scheme == "https" && !parsed.host.isNullOrEmpty()) u else null
        }
}

/** 出典(公共データ利用規約の出典の記載)。一覧の下に必ず出す。 */
@Serializable
data class FeedSource(val name: String, val url: String, val license: String)
