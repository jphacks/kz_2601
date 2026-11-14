package jp.kz2601.metamon.services.http

import kotlinx.coroutines.CancellationException
import java.io.IOException
import java.io.InterruptedIOException
import java.time.Instant

/**
 * 失敗の種類(iOS の `FailureKind`・#60)。画面はこの分類だけを見て文言と主ボタンを決める。
 * iOS に無い [ProRequiredKind] は 402 `pro_required` を「その他の 4xx」に落とさないために足した。
 */
sealed interface FailureKind {
    /** 圏外・接続断 */
    data object Offline : FailureKind

    /** 応答が返らなかった(読み込みの時間切れ / 504) */
    data object Timeout : FailureKind

    /** 401(トークンを取り直しても 401)。サインインし直す */
    data object SessionExpired : FailureKind

    /** 401 `app_check_failed`。サインインでは直らない */
    data object AppCheckFailed : FailureKind

    /** 503 `ai_paused` */
    data object AIPaused : FailureKind

    /** 503 `free_plan_paused`。もう一度では解けない */
    data class FreePlanPaused(val resumeAt: Instant?) : FailureKind

    /** 402 `pro_required` */
    data class ProRequiredKind(val info: ProRequired) : FailureKind

    /** 403 `plan_limit_reached`(アプリの数・記録・写真の上限) */
    data class PlanLimitReached(val info: PlanLimit) : FailureKind

    /** 403 `app_stopped` */
    data object AppStopped : FailureKind

    /** 混雑・一時障害(429 / 5xx) */
    data class Busy(val retryAfterSeconds: Double?) : FailureKind

    /** 今日の上限(429 `rate_limited` + scope)。再試行させない */
    data class RateLimited(val info: RateLimitInfo) : FailureKind

    /** 409 の版の競合。最新を取り直して再適用する */
    data class Conflict(val currentVersion: Int?) : FailureKind

    /** 利用者が止めた */
    data object Cancelled : FailureKind

    /** その他の 4xx(入力不備など)。message は人向け */
    data class Rejected(val message: String) : FailureKind

    /** 分類できない(読めない応答など) */
    data class Other(val message: String) : FailureKind

    /** 「もう一度」で同じ操作をやり直す意味があるか */
    val isRetryable: Boolean get() = this is Offline || this is Timeout || this is Busy

    companion object {
        fun classify(error: Throwable, now: Instant = Instant.now()): FailureKind {
            if (error is CancellationException) return Cancelled
            if (error is InterruptedIOException) return Timeout // SocketTimeoutException もここ
            if (error is IOException) return Offline
            val api = error as? ApiError ?: return Other(error.message ?: error.toString())
            api.rateLimit(now)?.let { return RateLimited(it) }
            if (api.isAppCheckFailure) return AppCheckFailed
            if (api.isAIPaused) return AIPaused
            if (api.isFreePlanPaused) return FreePlanPaused(api.resumeAt(now))
            api.proRequired?.let { return ProRequiredKind(it) }
            api.planLimit?.let { return PlanLimitReached(it) }
            if (api.isAppStopped) return AppStopped
            return when (val s = api.status) {
                401 -> SessionExpired
                409 -> Conflict(api.currentVersion)
                429, 500, 502, 503 -> Busy(api.retryAfterSeconds)
                504 -> Timeout
                in 400..499 -> Rejected(api.message ?: "HTTP $s")
                in 500..599 -> Busy(api.retryAfterSeconds)
                else -> Other(api.message ?: "HTTP $s")
            }
        }
    }
}
