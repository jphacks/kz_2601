package jp.kz2601.metamon.services

import jp.kz2601.metamon.services.http.ApiJson
import kotlinx.serialization.Serializable
import java.time.Instant
import java.time.ZoneId

/**
 * `GET /v1/me/usage`(iOS の `MyUsage`・openapi-appconfig.yaml の `MyUsage`)。音声の画面の「今日の残り」に使う分だけ読む。
 */
@Serializable
data class MyUsage(
    /** free | pro */
    val plan: String = "free",
    val tier: String? = null,
    /** 次の 0:00 JST(ISO 8601) */
    val resetAt: String? = null,
    val ai: Ai? = null,
) {
    @Serializable
    data class Ai(val used: Int = 0, val limit: Int? = null, val remaining: Int? = null)

    val isPro: Boolean get() = plan == "pro"

    /** 「今日の残り」。数えない・読めない時は null。[observedAt] はサーバーから読んだ時刻 */
    fun remainingToday(observedAt: Instant): RemainingToday? {
        val remaining = ai?.remaining ?: return null
        val reset = resetAt?.let { runCatching { Instant.parse(it) }.getOrNull() }
        return RemainingToday(remaining, ai.limit, reset, observedAt)
    }

    companion object {
        fun decode(json: String): MyUsage = ApiJson.decodeFromString(serializer(), json)
    }
}

/**
 * ai の「今日の残り」(iOS の `RemainingToday`・#630)。単位はサーバーの重み(新規 3・修正 2・チャット 1)なので「回」と言わない。
 */
data class RemainingToday(
    val remaining: Int,
    val limit: Int? = null,
    /** 0:00 JST */
    val resetAt: Instant? = null,
    val observedAt: Instant,
) {
    /** サーバーの重みの既定(rate-limit.md)。説明の文言だけに使う */
    object Weight {
        const val CREATE = 3
        const val EDIT = 2
        const val CHAT = 1
    }

    /** 戻る時刻を過ぎた(無ければ JST の日付が変わった)ら古い。古い値は出さない */
    fun isStale(now: Instant): Boolean {
        resetAt?.let { return !now.isBefore(it) }
        return observedAt.atZone(JST).toLocalDate() != now.atZone(JST).toLocalDate()
    }

    private companion object {
        val JST: ZoneId = ZoneId.of("Asia/Tokyo")
    }
}

/** 使用量を読む口(iOS の `PlanUsageSource`・[jp.kz2601.metamon.services.live.HttpAppConfigClient] が実装)。null = サーバーに無い(404) */
interface PlanUsageSource {
    suspend fun myUsage(): MyUsage?
}
