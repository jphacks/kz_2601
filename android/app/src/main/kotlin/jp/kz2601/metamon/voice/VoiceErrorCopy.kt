package jp.kz2601.metamon.voice

import androidx.annotation.StringRes
import jp.kz2601.metamon.R
import jp.kz2601.metamon.purchase.PaywallReason
import jp.kz2601.metamon.services.TranscriberFailure
import jp.kz2601.metamon.services.http.FailureKind
import jp.kz2601.metamon.services.http.PlanLimit
import jp.kz2601.metamon.services.http.RateLimitInfo
import java.time.Instant

/** 状態カードの中身。1 文で「何が起きたか + 次の一手」(iOS の `ErrorCopy`)。秒数・dB・サーバーの文は出さない(H9)。 */
data class ErrorCardCopy(
    @StringRes val title: Int,
    @StringRes val body: Int,
    /** 「もう一度」を出すか */
    val retry: Boolean = false,
    /** 「設定を開く」を出すか(マイクの権限) */
    val openSettings: Boolean = false,
    /** body の %1$s(今日の上限が戻る時刻)。null は「明日」 */
    val resetAt: Instant? = null,
    /** 「Pro にする」で開く課金画面(iOS の `Primary.upgradePro`)。null = 出さない */
    val upgrade: PaywallReason? = null,
)

object VoiceErrorCopy {
    fun of(error: VoiceError): ErrorCardCopy = when (error) {
        is VoiceError.Voice -> transcriber(error.failure)
        is VoiceError.Request -> request(error.kind, error.origin)
    }

    /** iOS の `ErrorCopy.transcriberTitle` / `transcriberFailure` と同じ分け方。 */
    fun transcriber(f: TranscriberFailure): ErrorCardCopy = when (f) {
        TranscriberFailure.NotReady -> ErrorCardCopy(R.string.voice_t_not_ready, R.string.voice_b_not_ready)
        TranscriberFailure.MicrophoneDenied -> ErrorCardCopy(R.string.voice_t_mic_denied, R.string.voice_b_mic_denied, openSettings = true)
        TranscriberFailure.NoInputAvailable, is TranscriberFailure.Other ->
            ErrorCardCopy(R.string.voice_t_record_failed, R.string.voice_b_record_failed)
        is TranscriberFailure.TooShort -> ErrorCardCopy(R.string.voice_t_not_heard, R.string.voice_b_too_short)
        is TranscriberFailure.Silent -> ErrorCardCopy(R.string.voice_t_not_heard, R.string.voice_b_silent)
        TranscriberFailure.NotHeard -> ErrorCardCopy(R.string.voice_t_not_heard, R.string.voice_b_not_heard)
        TranscriberFailure.OnDeviceRecognitionUnavailable -> ErrorCardCopy(R.string.voice_t_unavailable, R.string.voice_b_unavailable)
    }

    fun request(kind: FailureKind, origin: VoiceError.Origin): ErrorCardCopy {
        val failed = if (origin == VoiceError.Origin.CHAT) R.string.voice_chat_failed_title else R.string.voice_failed_title
        return when (kind) {
            FailureKind.Offline -> ErrorCardCopy(R.string.voice_offline_title, R.string.voice_offline_body, retry = true)
            FailureKind.Timeout -> ErrorCardCopy(R.string.voice_timeout_title, R.string.voice_timeout_body, retry = true)
            is FailureKind.Busy -> ErrorCardCopy(R.string.voice_busy_title, R.string.voice_busy_body, retry = true)
            FailureKind.SessionExpired -> ErrorCardCopy(R.string.voice_session_title, R.string.voice_session_body)
            FailureKind.AppCheckFailed -> ErrorCardCopy(failed, R.string.voice_app_check_body, retry = true)
            FailureKind.AIPaused -> ErrorCardCopy(R.string.voice_ai_paused_title, R.string.voice_ai_paused_body)
            is FailureKind.FreePlanPaused -> ErrorCardCopy(R.string.voice_free_paused_title, R.string.voice_free_paused_body)
            is FailureKind.ProRequiredKind -> ErrorCardCopy(R.string.voice_plan_limit_title, R.string.voice_pro_required_body)
            is FailureKind.PlanLimitReached -> if (showsProPath(kind.info)) {
                // #317: 今日の上限ではなくプランの上限なので、設定と同じ見出しで開く
                ErrorCardCopy(R.string.voice_plan_limit_title, R.string.voice_plan_limit_body_pro, upgrade = PaywallReason.SETTINGS)
            } else {
                ErrorCardCopy(R.string.voice_plan_limit_title, R.string.voice_plan_limit_body)
            }
            FailureKind.AppStopped -> ErrorCardCopy(R.string.voice_app_stopped_title, R.string.voice_app_stopped_body)
            is FailureKind.RateLimited -> ErrorCardCopy(
                R.string.voice_rate_title,
                R.string.voice_rate_body,
                resetAt = kind.info.resetAt,
                upgrade = PaywallReason.LIMIT_REACHED.takeIf { rateLimitUpgrades(kind.info) },
            )
            is FailureKind.Conflict -> ErrorCardCopy(failed, R.string.voice_conflict_body, retry = true)
            is FailureKind.Rejected -> ErrorCardCopy(R.string.voice_rejected_title, R.string.voice_rejected_body)
            FailureKind.Cancelled, is FailureKind.Other -> ErrorCardCopy(failed, R.string.voice_other_body, retry = true)
        }
    }

    /** プランの上限で「Pro にする」を出すか。作成者本人だけ(参加者は払っても直らない)。iOS の `ErrorCopy.showsProPath` */
    fun showsProPath(limit: PlanLimit): Boolean = limit.upgrade && !limit.byCreator

    /** 今日の上限で「Pro にする」を出すか(iOS の `rateLimitPrimary`)。サインインした無料の人だけ。全体の停止・IP・匿名・Pro は出さない */
    fun rateLimitUpgrades(info: RateLimitInfo): Boolean =
        info.scope == RateLimitInfo.Scope.USER && !info.isAnonymous && info.tier != "pro"
}
