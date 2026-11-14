package jp.kz2601.metamon.desktop.settings

import jp.kz2601.metamon.desktop.i18n.Lang
import jp.kz2601.metamon.desktop.i18n.StringTable
import jp.kz2601.metamon.desktop.i18n.Strings
import jp.kz2601.metamon.services.RemainingToday
import jp.kz2601.metamon.services.RemainingTodayStore
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import java.time.Instant
import java.time.ZoneId

/** 設定の「今日の残り」の行(#3361): 古い値・まだ分からない・上限なし。 */
class RemainingTodaySettings3361Test {
    private val now = Instant.parse("2026-11-13T03:00:00Z") // JST 12:00
    private val reset = Instant.parse("2026-11-13T15:00:00Z") // JST 0:00
    private val jst = ZoneId.of("Asia/Tokyo")

    private fun amount(row: RemainingTodayRow, lang: Lang = Lang.JA) = Strings.withLang(lang) { RemainingTodayText.amount(row) }

    @Test
    fun unknownSaysShownAfterUsingAi() {
        val row = RemainingTodayRow.of(null, now, isPro = true)
        assertNull(row.amount)
        assertFalse("数が分からない時は Pro も出さない", row.pro)
        assertNull(row.resetAt)
        assertEquals("AI を使うと表示されます", amount(row))
        assertEquals("Shown after you use AI", amount(row, Lang.EN))
    }

    @Test
    fun staleValueIsTreatedAsUnknown() {
        // 戻る時刻を過ぎた値
        val past = RemainingToday(10, 60, Instant.parse("2026-11-13T02:00:00Z"), now.minusSeconds(7200))
        assertEquals("AI を使うと表示されます", amount(RemainingTodayRow.of(past, now, isPro = false)))
        // 戻る時刻が無く、前の日に見た値
        val yesterday = RemainingToday(10, 60, null, Instant.parse("2026-11-11T03:00:00Z"))
        assertNull(RemainingTodayRow.of(yesterday, now, isPro = false).amount)
    }

    @Test
    fun withLimitShowsRemainingSlashLimitAndResetTime() {
        val row = RemainingTodayRow.of(RemainingToday(24, 60, reset, now), now, isPro = true)
        assertEquals("24 / 60", amount(row))
        assertTrue(row.pro)
        assertFalse(row.low)
        assertEquals(reset, row.resetAt)
        assertEquals("11月14日 0:00", RemainingTodayText.resetText(reset, Lang.JA, jst))
        assertEquals("Nov 14, 0:00", RemainingTodayText.resetText(reset, Lang.EN, jst))
    }

    @Test
    fun noLimitShowsOnlyTheNumber() {
        val row = RemainingTodayRow.of(RemainingToday(24, null, reset, now), now, isPro = false)
        assertEquals("24", amount(row))
        assertFalse(row.pro)
    }

    @Test
    fun lowAtOrBelowTwoCreates() {
        assertTrue(RemainingTodayRow.of(RemainingToday(6, 60, reset, now), now, false).low)
        assertFalse(RemainingTodayRow.of(RemainingToday(7, 60, reset, now), now, false).low)
    }

    @Test
    fun hintFallsBackToTomorrowWithoutResetTime() {
        val unknown = RemainingTodayRow.of(null, now, false)
        assertEquals("新しく作ると 3、直すと 2、相談すると 1 減ります。明日に戻ります", RemainingTodayText.hint(unknown, Lang.JA, jst))
        val known = RemainingTodayRow.of(RemainingToday(24, 60, reset, now), now, false)
        assertEquals("Creating an app uses 3, editing 2, and chatting 1. Refills Nov 14, 0:00", RemainingTodayText.hint(known, Lang.EN, jst))
    }

    @Test
    fun readsTheLatestValueFromTheSharedStore() {
        val store = RemainingTodayStore()
        store.record(RemainingToday(30, 60, reset, now))
        store.record(RemainingToday(40, 60, reset, now.minusSeconds(60))) // 後から届いた古い応答
        assertEquals("30 / 60", RemainingTodayRow.of(store.value.value, now, false).amount)
    }

    @Test
    fun stringsResolveInBothLanguages() {
        val t = StringTable.fromClasspath()
        val keys = listOf(
            "desktop_reset_time_pattern", "voice_remaining_hint", "voice_remaining_settings_reset",
            "voice_remaining_settings_title", "voice_remaining_settings_unknown", "voice_tomorrow",
        )
        for (lang in Lang.entries) for (k in keys) assertTrue("$k ($lang)", t.hasString(lang, k))
    }
}
