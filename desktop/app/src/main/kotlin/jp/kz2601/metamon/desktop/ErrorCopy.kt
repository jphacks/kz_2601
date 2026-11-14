package jp.kz2601.metamon.desktop

import jp.kz2601.metamon.desktop.i18n.Strings
import jp.kz2601.metamon.services.http.FailureKind
import jp.kz2601.metamon.services.http.RateLimitInfo
import java.time.Instant
import java.time.ZoneId
import java.time.format.DateTimeFormatter
import java.time.format.FormatStyle

/**
 * 生成・会話の失敗の文言を 1 か所にまとめる(#2984)。iOS の `ErrorCopy`・Android の `voice/VoiceErrorCopy.kt` と同じ分け方。
 * 見出しは「何が起きたか」、本文は「次にすること」の 1 文ずつ。秒数・サーバーの文は出さない。
 * キーは Android の strings の物を使い、デスクトップだけ違う文は `desktop_` のキー。
 */
object ErrorCopy {
    data class Copy(
        val title: String,
        val body: String,
        /** 「もう一度」を出すか */
        val retry: Boolean,
    ) {
        /** 知らせの 1 行(見出し + 本文) */
        val line: String get() = "$title $body"
    }

    /** 会話(質問)の失敗か、生成の失敗か。見出しだけ変わる */
    enum class Origin { GENERATE, CHAT }

    fun of(
        kind: FailureKind,
        origin: Origin = Origin.GENERATE,
        zone: ZoneId = ZoneId.systemDefault(),
    ): Copy {
        val failed = if (origin == Origin.CHAT) "voice_chat_failed_title" else "voice_failed_title"
        fun c(title: String, body: String, retry: Boolean = false) = Copy(Strings[title], Strings[body], retry)
        return when (kind) {
            FailureKind.Offline -> c("voice_offline_title", "voice_offline_body", retry = true)
            FailureKind.Timeout -> c("voice_timeout_title", "voice_timeout_body", retry = true)
            is FailureKind.Busy -> c("voice_busy_title", "voice_busy_body", retry = true)
            FailureKind.SessionExpired -> c("voice_session_title", "voice_session_body")
            // Android の本文は Google Play を指すので、デスクトップ用の文にする
            FailureKind.AppCheckFailed -> c(failed, "desktop_error_app_check", retry = true)
            FailureKind.AIPaused -> c("voice_ai_paused_title", "voice_ai_paused_body")
            is FailureKind.FreePlanPaused -> c("voice_free_paused_title", "voice_free_paused_body")
            is FailureKind.ProRequiredKind -> c("voice_plan_limit_title", "voice_pro_required_body")
            is FailureKind.PlanLimitReached -> c("voice_plan_limit_title", "voice_plan_limit_body")
            FailureKind.AppStopped -> c("voice_app_stopped_title", "voice_app_stopped_body")
            is FailureKind.RateLimited -> Copy(
                Strings["voice_rate_title"],
                Strings.format("voice_rate_body", resetText(kind.info, zone)),
                retry = false,
            )
            is FailureKind.Conflict -> c(failed, "voice_conflict_body", retry = true)
            is FailureKind.Rejected -> c("voice_rejected_title", "voice_rejected_body")
            FailureKind.Cancelled, is FailureKind.Other -> c(failed, "voice_other_body", retry = true)
        }
    }

    /** 例外から。分類は API と同じ [FailureKind.classify]。 */
    fun of(error: Throwable, origin: Origin = Origin.GENERATE, zone: ZoneId = ZoneId.systemDefault()): Copy =
        of(FailureKind.classify(error), origin, zone)

    /** 今日の上限が戻る時刻。分からない時は「明日」。 */
    private fun resetText(info: RateLimitInfo, zone: ZoneId): String {
        val at: Instant = info.resetAt ?: return Strings["voice_tomorrow"]
        return DateTimeFormatter.ofLocalizedTime(FormatStyle.SHORT).withLocale(Strings.locale).withZone(zone).format(at)
    }
}
