package jp.kz2601.metamon.runtime

import jp.kz2601.metamon.R
import jp.kz2601.metamon.dsl.ActionType

/** AppRuntime が出す文言(iOS の L10n キーと同じ)。文は strings_runtime.xml、画面で解決する。 */
object RuntimeCopy {
    val NO_FORM = UiText.Res(R.string.runtime_no_form)
    val READ_ONLY = UiText.Res(R.string.runtime_read_only)
    val SAVE_FAILED = UiText.Res(R.string.runtime_save_failed)
    val OPEN_URL_REJECTED = UiText.Res(R.string.runtime_open_url_rejected)
    val SAVE_PARENT_FIRST = UiText.Res(R.string.runtime_save_parent_first)
    val REJECTED_GENERIC = UiText.Res(R.string.runtime_rejected_generic)
    val CONFLICT = UiText.Res(R.string.runtime_conflict)
    val NOT_MEMBER = UiText.Res(R.string.runtime_not_member)
    val APP_STOPPED = UiText.Res(R.string.runtime_app_stopped)
    val TEXT_NOT_ALLOWED = UiText.Res(R.string.runtime_text_not_allowed)
    val ADMINS_ONLY = UiText.Res(R.string.runtime_admins_only)
    val ONE_PER_PERSON = UiText.Res(R.string.runtime_one_per_person)
    val DUPLICATE = UiText.Res(R.string.runtime_duplicate)
    val NOTIFICATION_FAILED = UiText.Res(R.string.runtime_notification_failed)
    val ACTIVITY_NOTIFICATIONS_OFF = UiText.Res(R.string.runtime_activity_notifications_off)
    val FAILED = UiText.Res(R.string.runtime_failed)
    val UNAVAILABLE_ON_ANDROID = UiText.Res(R.string.runtime_unavailable_on_android)
    const val NAME_SEPARATOR = "・"
    val HOT_RELOADED = UiText.Res(R.string.runtime_hot_reloaded)
    val OFFLINE_RESTORED = UiText.Res(R.string.runtime_offline_restored)
    val APPS_OFFLINE = UiText.Res(R.string.runtime_apps_offline)
    val APPS_FAILED = UiText.Res(R.string.runtime_apps_failed)
    val SWITCH_FAILED = UiText.Res(R.string.runtime_switch_failed)

    /** 共有の投稿の表示名(field id が authorName の時の名前)。 */
    val AUTHOR_NAME = UiText.Res(R.string.runtime_author_name)

    fun switched(name: String, offline: Boolean) =
        UiText.Res(if (offline) R.string.runtime_switched_offline else R.string.runtime_switched, name)

    fun missingRequired(names: List<String>) = UiText.Res(R.string.runtime_missing_required, names.joinToString(NAME_SEPARATOR))

    fun saveParentFirst(name: String) = if (name.isEmpty()) SAVE_PARENT_FIRST else UiText.Res(R.string.runtime_save_parent_first_named, name)

    fun rejected(fieldName: String) = UiText.Res(R.string.runtime_rejected_field, fieldName)

    fun paused(pending: Int) = UiText.Res(R.string.runtime_paused, pending)

    fun notificationScheduled(whenText: String) = UiText.Res(R.string.runtime_notification_scheduled, whenText)

    fun unknownAction(rawType: String) = UiText.Res(R.string.runtime_unknown_action, rawType)

    fun adminsOnly(fieldName: String) = UiText.Res(R.string.runtime_admins_only_field, fieldName)

    fun duplicate(names: List<String>) = UiText.Res(R.string.runtime_duplicate_fields, names.joinToString(NAME_SEPARATOR))

    fun textNotAllowed(fieldName: Any) = UiText.Res(R.string.runtime_text_not_allowed_field, fieldName)

    /** 確認ダイアログの主ボタン(iOS の `Confirm.button(for:)`)。 */
    fun confirmButton(type: ActionType): Pair<Int, Boolean> = when (type) {
        ActionType.DELETE_RECORD -> R.string.runtime_confirm_delete to true
        ActionType.RESET_FORM -> R.string.runtime_confirm_reset to true
        ActionType.SAVE_RECORD -> R.string.runtime_confirm_save to false
        ActionType.EXPORT -> R.string.runtime_confirm_export to false
        ActionType.NAVIGATE, ActionType.OPEN_URL -> R.string.runtime_confirm_open to false
        else -> R.string.runtime_confirm_continue to false
    }
}
