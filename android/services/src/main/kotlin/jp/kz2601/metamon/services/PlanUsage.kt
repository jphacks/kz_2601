package jp.kz2601.metamon.services

import jp.kz2601.metamon.services.http.ApiJson
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.serialization.Serializable
import kotlinx.serialization.json.contentOrNull
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import java.time.Instant
import java.time.ZoneId

/** 使っている量と上限(#317・openapi-appconfig.yaml の `UsageMeter`)。[limit] が null = 数えない */
@Serializable
data class UsageMeter(val used: Int = 0, val limit: Int? = null, val remaining: Int? = null) {
    /** 上限に対する割合(数えない・上限 0 は null) */
    val fraction: Double? get() = limit?.takeIf { it > 0 }?.let { used.toDouble() / it }

    /** 80% 以上(iOS の `UsageNoticeRules.isNear`) */
    val isNear: Boolean get() = (fraction ?: 0.0) >= NEAR_THRESHOLD

    companion object {
        const val NEAR_THRESHOLD = 0.8
    }
}

/**
 * `GET /v1/me/usage`(iOS の `MyUsage`・openapi-appconfig.yaml の `MyUsage`)。
 * 音声の画面の「今日の残り」と、設定の「使っている量」(作ったアプリの数)に使う分だけ読む。
 */
@Serializable
data class MyUsage(
    /** free | pro */
    val plan: String = "free",
    val tier: String? = null,
    /** 次の 0:00 JST(ISO 8601) */
    val resetAt: String? = null,
    val ai: Ai? = null,
    /** 作ったアプリの数。数えられなかった時は null */
    val apps: UsageMeter? = null,
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

    companion object {
        private val JST: ZoneId = ZoneId.of("Asia/Tokyo")

        /**
         * 応答の `X-RateLimit-Remaining` / `-Limit` / `-Reset`(docs/backend/rate-limit.md)から読む(iOS の `RemainingToday.parse`)。
         * 残りが無い・壊れている(整数でない・負・上限より多い)時は null。ヘッダー名の大文字小文字は [header] が問わない。
         * 429 は利用者の上限(`details.scope == "user"`)の時だけ使う(IP・全体の上限の数は本人の残りではない)。
         */
        fun parse(header: (String) -> String?, status: Int, body: String, now: Instant): RemainingToday? {
            fun int(name: String): Int? = header(name)?.trim()?.toIntOrNull()?.takeIf { it >= 0 }
            val remaining = int("X-RateLimit-Remaining") ?: return null
            val limit = int("X-RateLimit-Limit")
            if (limit != null && remaining > limit) return null
            if (status == 429 && userScope(body) != true) return null
            val resetAt = int("X-RateLimit-Reset")?.takeIf { it > 0 }?.let { Instant.ofEpochSecond(it.toLong()) }
            return RemainingToday(remaining, limit, resetAt, now)
        }

        private fun userScope(body: String): Boolean? = runCatching {
            val details = ApiJson.parseToJsonElement(body).jsonObject["error"]?.jsonObject?.get("details")?.jsonObject
            details?.get("scope")?.jsonPrimitive?.contentOrNull == "user"
        }.getOrNull()
    }
}

/**
 * 最後に分かった「今日の残り」(iOS の `RemainingTodayStore`)。ai の応答のヘッダー([RemainingToday.parse])と
 * `GET /v1/me/usage` から入り、音声の画面と設定が読む。端末には残さない(起動し直したら次の依頼か usage で分かる)。
 */
class RemainingTodayStore {
    private val _value = MutableStateFlow<RemainingToday?>(null)
    val value: StateFlow<RemainingToday?> = _value.asStateFlow()

    /** 古い応答(後から届いた前の依頼の分)では上書きしない。 */
    fun record(value: RemainingToday) {
        _value.update { last -> if (last != null && last.observedAt.isAfter(value.observedAt)) last else value }
    }

    /** 今使える値(古ければ null)。 */
    fun current(now: Instant): RemainingToday? = _value.value?.takeUnless { it.isStale(now) }

    /** サインアウト・アカウント削除で消す(別のアカウントの残りを出さない)。 */
    fun clear() {
        _value.value = null
    }
}

/** `GET /v1/apps/{appId}/usage`(iOS の `AppUsage`)。払う人のプランで数えた記録・写真 */
@Serializable
data class AppUsage(
    val appId: String = "",
    /** 払う人のプラン(free | pro) */
    val plan: String = "free",
    /** self = 自分のアプリ・creator = 他の人が作ったアプリ */
    val payer: String = "creator",
    /** Pro にすると記録か写真の上限が上がる */
    val upgrade: Boolean = false,
    val records: UsageMeter? = null,
    val photos: UsageMeter? = null,
) {
    /** 自分のアプリ(作成者)。参加者には量も Pro も出さない */
    val isMine: Boolean get() = payer == "self"

    companion object {
        fun decode(json: String): AppUsage = ApiJson.decodeFromString(serializer(), json)
    }
}

/** 使用量を読む口(iOS の `PlanUsageSource`・[jp.kz2601.metamon.services.live.HttpAppConfigClient] が実装)。null = サーバーに無い(404) */
interface PlanUsageSource {
    suspend fun myUsage(): MyUsage?

    /** 今のアプリの記録・写真。既定は読まない(null) */
    suspend fun appUsage(appId: String): AppUsage? = null
}
