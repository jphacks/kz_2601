package jp.kz2601.metamon.renderer.components.ai

import androidx.annotation.StringRes
import jp.kz2601.metamon.dsl.Task
import jp.kz2601.metamon.dsl.ai.AiTextError
import jp.kz2601.metamon.renderer.R

/** 画面に出す 1 文(文言の id と差し込む値)。[raw] はサーバーの文をそのまま出す時。 */
data class AiMessage(@StringRes val res: Int, val arg: String? = null, val raw: String? = null)

/** aiButton の文言の選び方(strings_ai.xml)。 */
object AiCopy {
    fun error(e: Throwable): AiMessage = when (e) {
        is AiBuildError -> AiMessage(
            when (e.kind) {
                AiBuildError.Kind.UNKNOWN_TASK, AiBuildError.Kind.NO_INSTRUCTION -> R.string.ai_error_bad_config
                AiBuildError.Kind.EMPTY_INPUT -> R.string.ai_error_empty_input
                AiBuildError.Kind.NO_OPTIONS -> R.string.ai_error_no_options
                AiBuildError.Kind.NO_RECORDS -> R.string.ai_error_no_records
                AiBuildError.Kind.NO_OWN_RECORDS -> R.string.ai_error_no_own_records
            },
        )
        AiTextError.Unsupported -> AiMessage(R.string.ai_unavailable_title)
        AiTextError.NeedsConsent -> AiMessage(R.string.ai_error_needs_consent)
        AiTextError.NeedsPro -> AiMessage(R.string.ai_error_needs_pro)
        AiTextError.CreatorNeedsPro -> AiMessage(R.string.ai_error_creator_needs_pro)
        AiTextError.Refused -> AiMessage(R.string.ai_error_refused)
        AiTextError.BadOutput -> AiMessage(R.string.ai_error_bad_output)
        AiTextError.Paused -> AiMessage(R.string.ai_error_paused)
        AiTextError.TooLarge -> AiMessage(R.string.ai_error_too_large)
        // 利用上限などはサーバーの文(日本語)をそのまま出す
        is AiTextError.Failed -> AiMessage(R.string.ai_error_failed, raw = e.detail)
        else -> AiMessage(R.string.ai_error_failed, arg = e.message ?: e.javaClass.simpleName)
    }

    @StringRes
    fun reason(r: AiRoute.UnavailableReason): Int = when (r) {
        AiRoute.UnavailableReason.LongOnlyOnline -> R.string.ai_reason_long_only_online
        AiRoute.UnavailableReason.PhotoOnlyOnline -> R.string.ai_reason_photo_only_online
        is AiRoute.UnavailableReason.Device -> when (r.reason) {
            OnDeviceAvailability.Reason.DEVICE_NOT_ELIGIBLE -> R.string.ai_reason_device_not_eligible
            OnDeviceAvailability.Reason.NOT_ENABLED -> R.string.ai_reason_not_enabled
            OnDeviceAvailability.Reason.MODEL_NOT_READY -> R.string.ai_reason_model_not_ready
            OnDeviceAvailability.Reason.OS_TOO_OLD -> R.string.ai_reason_os_too_old
            OnDeviceAvailability.Reason.OTHER -> R.string.ai_reason_other
        }
    }

    @StringRes
    fun label(task: Task?): Int = when (task) {
        Task.SUMMARIZE -> R.string.ai_label_summarize
        Task.CLASSIFY -> R.string.ai_label_classify
        Task.REWRITE -> R.string.ai_label_rewrite
        Task.EXTRACT -> R.string.ai_label_extract
        Task.SUGGEST -> R.string.ai_label_suggest
        Task.GENERATE -> R.string.ai_generate_label
        else -> R.string.ai_label_default
    }
}
