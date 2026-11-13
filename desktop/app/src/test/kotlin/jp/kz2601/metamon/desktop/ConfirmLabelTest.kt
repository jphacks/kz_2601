package jp.kz2601.metamon.desktop

import jp.kz2601.metamon.desktop.i18n.Lang
import jp.kz2601.metamon.desktop.i18n.Strings
import jp.kz2601.metamon.dsl.Action
import jp.kz2601.metamon.dsl.ActionType
import org.junit.Assert.assertEquals
import org.junit.Test

/** 確認ダイアログの主ボタン(confirmLabel): iOS の RuntimeConfirm.button・Android の RuntimeCopy.confirmButton と同じ語。 */
class ConfirmLabelTest {
    private fun labels(lang: Lang): Map<ActionType, String> {
        val before = Strings.lang
        Strings.lang = lang
        try {
            return listOf(
                ActionType.DELETE_RECORD,
                ActionType.SAVE_RECORD,
                ActionType.EXPORT,
                ActionType.OPEN_URL,
                ActionType.NAVIGATE,
                ActionType.RESET_FORM,
                ActionType.ADD_TO_CALENDAR,
                ActionType.LOOKUP_BOOK,
            ).associateWith { confirmLabel(Action(type = it)) }
        } finally {
            Strings.lang = before
        }
    }

    @Test
    fun confirmLabelPerAction() {
        assertEquals(
            mapOf(
                ActionType.DELETE_RECORD to "削除",
                ActionType.SAVE_RECORD to "保存",
                ActionType.EXPORT to "書き出す",
                ActionType.OPEN_URL to "開く",
                ActionType.NAVIGATE to "開く",
                ActionType.RESET_FORM to "入力を消す",
                ActionType.ADD_TO_CALENDAR to "続ける",
                ActionType.LOOKUP_BOOK to "続ける",
            ),
            labels(Lang.JA),
        )
        assertEquals(
            mapOf(
                ActionType.DELETE_RECORD to "Delete",
                ActionType.SAVE_RECORD to "Save",
                ActionType.EXPORT to "Export",
                ActionType.OPEN_URL to "Open",
                ActionType.NAVIGATE to "Open",
                ActionType.RESET_FORM to "Clear",
                ActionType.ADD_TO_CALENDAR to "Continue",
                ActionType.LOOKUP_BOOK to "Continue",
            ),
            labels(Lang.EN),
        )
    }
}
