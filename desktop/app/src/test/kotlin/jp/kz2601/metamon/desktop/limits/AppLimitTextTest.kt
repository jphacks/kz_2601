package jp.kz2601.metamon.desktop.limits

import jp.kz2601.metamon.desktop.i18n.Lang
import jp.kz2601.metamon.desktop.i18n.StringTable
import jp.kz2601.metamon.desktop.i18n.Strings
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test
import java.time.Instant
import java.time.ZoneOffset

/** アプリの数の上限のダイアログの文言: 読み上げる 1 行(名前・最後に開いた日・操作)と確認の文(#2974)。 */
class AppLimitTextTest {
    private val own = AppReplacePlan.Row("a", "家計簿", AppReplacePlan.Role.OWN, Instant.parse("2026-10-01T12:00:00Z"), isCurrent = false)
    private val joined = AppReplacePlan.Row("b", "旅行", AppReplacePlan.Role.JOINED, null, isCurrent = true)

    @Test
    fun rowReadsNameLastOpenedAndAction() = Strings.withLang(Lang.JA) {
        val text = AppLimitText.rowDescription(own, ZoneOffset.UTC, Lang.JA.locale)
        assertEquals("家計簿、最後に開いた日 2026/10/01、削除", text)
        assertEquals("旅行、使用中、開いた記録なし、抜ける", AppLimitText.rowDescription(joined, ZoneOffset.UTC, Lang.JA.locale))
    }

    @Test
    fun rowReadsInEnglish() = Strings.withLang(Lang.EN) {
        assertEquals("家計簿, Last opened Oct 1, 2026, Delete", AppLimitText.rowDescription(own, ZoneOffset.UTC, Lang.EN.locale))
    }

    @Test
    fun confirmWarnsDataGoesOnDeleteOnly() = Strings.withLang(Lang.JA) {
        assertEquals("「家計簿」を削除しますか?", AppLimitText.confirmTitle(own))
        assertTrue(AppLimitText.confirmBody(own).contains("消えます"))
        assertEquals("「旅行」から抜けますか?", AppLimitText.confirmTitle(joined))
        assertTrue(AppLimitText.confirmBody(joined).contains("招待リンク"))
    }

    @Test
    fun allKeysResolveInBothLanguages() {
        val t = StringTable.fromClasspath()
        val keys = listOf(
            "voice_plan_limit_title", "inputs_cancel", "runtime_confirm_delete", "members_leave_confirm", "members_leave_message",
            "desktop_applimit_body", "desktop_applimit_confirm_delete", "desktop_applimit_confirm_leave", "desktop_applimit_delete_message",
            "desktop_applimit_empty", "desktop_applimit_last_opened", "desktop_applimit_never_opened", "desktop_applimit_row_current",
        )
        for (lang in Lang.entries) for (k in keys) assertTrue("$k ($lang)", t.hasString(lang, k))
    }
}
