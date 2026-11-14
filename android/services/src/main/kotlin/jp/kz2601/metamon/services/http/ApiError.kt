package jp.kz2601.metamon.services.http

import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.booleanOrNull
import kotlinx.serialization.json.contentOrNull
import kotlinx.serialization.json.doubleOrNull
import java.time.Instant

/**
 * backend の失敗応答(iOS の `APIError`)。
 * - [Server]: 共通のエラー封筒 `{error:{code,message,details?}}`
 * - [Http]: 封筒で無い応答(プロキシの 502 など)
 */
sealed class ApiError(message: String) : Exception(message) {
    abstract val status: Int

    /** `Retry-After`(429 / 503)の秒数。無ければ null */
    abstract val retryAfterSeconds: Double?

    /** `error.code`。封筒で無い応答は null */
    abstract val code: String?

    class Server(
        override val status: Int,
        override val code: String,
        val serverMessage: String,
        val details: JsonObject = JsonObject(emptyMap()),
        override val retryAfterSeconds: Double? = null,
    ) : ApiError(serverMessage.ifEmpty { code }) {
        override fun toString(): String = "ApiError.Server($status $code: $serverMessage)"
    }

    class Http(
        override val status: Int,
        val body: String,
        override val retryAfterSeconds: Double? = null,
    ) : ApiError("HTTP $status: ${body.take(200)}") {
        override val code: String? get() = null

        override fun toString(): String = "ApiError.Http($status)"
    }

    private val detailMap: JsonObject get() = (this as? Server)?.details ?: JsonObject(emptyMap())

    fun detailString(key: String): String? = (detailMap[key] as? JsonPrimitive)?.takeIf { it.isString }?.content
    fun detailNumber(key: String): Double? = (detailMap[key] as? JsonPrimitive)?.doubleOrNull
    fun detailBool(key: String): Boolean? = (detailMap[key] as? JsonPrimitive)?.booleanOrNull
    fun detail(key: String): JsonElement? = detailMap[key]

    /** `details.field`(422 `invalid_record` / `text_not_allowed` など。どの項目か) */
    val field: String? get() = detailString("field")

    /** `details.reason`(`creator_needs_pro` / `admins_only` / `inviter_left` など) */
    val reason: String? get() = detailString("reason")

    /** 409 `version_conflict` の `details.currentVersion` */
    val currentVersion: Int? get() = detailNumber("currentVersion")?.toInt()

    /** App Check で弾かれた 401(#318)。ID トークンの失効と区別する */
    val isAppCheckFailure: Boolean get() = status == 401 && code == ErrorCodes.APP_CHECK_FAILED

    /** 運営が AI を一時停止(#511・503 `ai_paused`) */
    val isAIPaused: Boolean get() = status == 503 && code == ErrorCodes.AI_PAUSED

    /** 無料プランの一時停止(#317・503 `free_plan_paused`) */
    val isFreePlanPaused: Boolean get() = status == 503 && code == ErrorCodes.FREE_PLAN_PAUSED

    /** 運営がアプリの自動の動きを止めている(#346・403 `app_stopped`) */
    val isAppStopped: Boolean get() = status == 403 && code == ErrorCodes.APP_STOPPED

    /** 他の人に見える文字に表示できない言葉(#321・422 `text_not_allowed`) */
    val isTextNotAllowed: Boolean get() = status == 422 && code == ErrorCodes.TEXT_NOT_ALLOWED

    /** DSL の版の競合(409)。最新を取り直して再適用する */
    val isVersionConflict: Boolean get() = status == 409 && code != ErrorCodes.DUPLICATE_RECORD

    /** `If-Match` を付け忘れた(412)。クライアントの不具合 */
    val isPreconditionRequired: Boolean get() = status == 412

    /** 無料プランの再開時刻(`details.resumeAt` → `retryAfterSec` → `Retry-After`) */
    fun resumeAt(now: Instant = Instant.now()): Instant? {
        detailString("resumeAt")?.let { raw -> runCatching { Instant.parse(raw) }.getOrNull()?.let { return it } }
        detailNumber("retryAfterSec")?.let { return now.plusMillis((it * 1000).toLong()) }
        return retryAfterSeconds?.let { now.plusMillis((it * 1000).toLong()) }
    }

    /** ai の 1 日の上限(#280・429 `rate_limited` + `details.scope`)。混雑の 429 は null(= busy) */
    fun rateLimit(now: Instant = Instant.now()): RateLimitInfo? {
        if (status != 429 || code != ErrorCodes.RATE_LIMITED) return null
        val scope = detailString("scope") ?: return null
        var resetAt = detailString("resetAt")?.let { runCatching { Instant.parse(it) }.getOrNull() }
        if (resetAt == null) {
            val wait = detailNumber("retryAfterSec") ?: retryAfterSeconds
            if (wait != null) resetAt = now.plusMillis((wait * 1000).toLong())
        }
        return RateLimitInfo(
            scope = RateLimitInfo.Scope.from(scope),
            tier = detailString("tier"),
            limit = detailNumber("limit")?.toInt(),
            remaining = detailNumber("remaining")?.toInt(),
            resetAt = resetAt,
        )
    }

    val proRequired: ProRequired? get() = ProRequired.from(this)
    val planLimit: PlanLimit? get() = PlanLimit.from(this)
}

/** サーバーの 1 日の上限に当たった時の中身(#280・openapi-ai.yaml `RateLimitDetails`)。 */
data class RateLimitInfo(
    val scope: Scope,
    /** anonymous | signed_in | pro */
    val tier: String? = null,
    val limit: Int? = null,
    val remaining: Int? = null,
    val resetAt: Instant? = null,
) {
    enum class Scope(val wire: String) {
        USER("user"),
        IP("ip"),
        GLOBAL("global"),
        ;

        companion object {
            fun from(raw: String): Scope = entries.firstOrNull { it.wire == raw } ?: USER
        }
    }

    val isAnonymous: Boolean get() = tier == "anonymous"
}

/** 有料機能 1 つ(#77・openapi `PaidFeature`)。 */
data class PaidFeature(val id: String, val title: String, val reason: String = "")

/** 402 `pro_required`(#316)。`details.reason == creator_needs_pro` = 作成者の Pro が要る(参加者は払っても直らない)。 */
data class ProRequired(val features: List<PaidFeature>, val creatorNeedsPro: Boolean = false) {
    companion object {
        const val CREATOR_NEEDS_PRO = "creator_needs_pro"

        fun from(error: Throwable): ProRequired? {
            val api = error as? ApiError.Server ?: return null
            if (api.status != 402 || api.code != ErrorCodes.PRO_REQUIRED) return null
            val features = (api.detail("features") as? JsonArray).orEmpty().mapNotNull { item ->
                val o = item as? JsonObject ?: return@mapNotNull null
                fun str(key: String) = (o[key] as? JsonPrimitive)?.contentOrNull
                val id = str("id") ?: return@mapNotNull null
                PaidFeature(id = id, title = str("title") ?: id, reason = str("reason") ?: "")
            }
            return ProRequired(features, creatorNeedsPro = api.reason == CREATOR_NEEDS_PRO)
        }
    }
}

/**
 * 403 `plan_limit_reached`(#317)。アプリの数の上限(`resource = apps`)もここ。
 * `details.reason == creator_plan_limit` = 作成者のプランの上限(参加者には課金の導線を出さない)。
 */
data class PlanLimit(
    val resource: Resource,
    /** free | pro */
    val plan: String? = null,
    val limit: Int? = null,
    /** Pro にすると上限が上がる */
    val upgrade: Boolean = false,
    val byCreator: Boolean = false,
) {
    enum class Resource(val wire: String) {
        APPS("apps"),
        RECORDS_PER_APP("recordsPerApp"),
        PHOTOS_PER_APP("photosPerApp"),
        OTHER("other"),
        ;

        companion object {
            fun from(raw: String?): Resource = entries.firstOrNull { it.wire == raw } ?: OTHER
        }
    }

    companion object {
        const val CREATOR_PLAN_LIMIT = "creator_plan_limit"

        fun from(error: Throwable): PlanLimit? {
            val api = error as? ApiError.Server ?: return null
            if (api.status != 403 || api.code != ErrorCodes.PLAN_LIMIT_REACHED) return null
            return PlanLimit(
                resource = Resource.from(api.detailString("resource")),
                plan = api.detailString("plan"),
                limit = api.detailNumber("limit")?.toInt(),
                upgrade = api.detailBool("upgrade") ?: false,
                byCreator = api.reason == CREATOR_PLAN_LIMIT,
            )
        }
    }
}
