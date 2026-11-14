package jp.kz2601.metamon.desktop.voice

import jp.kz2601.metamon.desktop.i18n.Lang
import jp.kz2601.metamon.desktop.i18n.StringTable
import jp.kz2601.metamon.services.RemainingToday
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test
import java.time.Instant

/** 「今日の残り」の出し分け(分からない・少ない・0)と文言(#3008)。 */
class RemainingTodayDisplayTest {
    private val now = Instant.parse("2026-11-13T03:00:00Z") // JST 12:00
    private val reset = Instant.parse("2026-11-13T15:00:00Z") // JST 0:00

    private fun value(remaining: Int, resetAt: Instant? = reset, observedAt: Instant = now) =
        RemainingToday(remaining, 30, resetAt, observedAt)

    private fun of(v: RemainingToday?, pro: Boolean = false) = RemainingTodayDisplay.of(v, now, pro)

    @Test
    fun unknownIsHidden() {
        assertEquals(RemainingTodayDisplay.Hidden, of(null))
    }

    @Test
    fun staleIsHidden() {
        // 戻る時刻を過ぎた値は出さない
        assertEquals(RemainingTodayDisplay.Hidden, of(value(10, resetAt = Instant.parse("2026-11-13T02:00:00Z"))))
        // 戻る時刻が無く、前の日に見た値
        assertEquals(RemainingTodayDisplay.Hidden, of(value(10, resetAt = null, observedAt = Instant.parse("2026-11-11T03:00:00Z"))))
    }

    @Test
    fun plentyIsNormal() {
        assertEquals(RemainingTodayDisplay.Normal(20, false), of(value(20)))
        assertEquals(RemainingTodayDisplay.Normal(7, true), of(value(7), pro = true))
    }

    @Test
    fun lowIsAtOrBelowTwoCreates() {
        assertEquals(6, RemainingTodayDisplay.LOW_THRESHOLD)
        assertEquals(RemainingTodayDisplay.Low(6, false), of(value(6)))
        assertEquals(RemainingTodayDisplay.Low(1, true), of(value(1), pro = true))
    }

    @Test
    fun zeroIsExhaustedWithResetTime() {
        assertEquals(RemainingTodayDisplay.Exhausted(reset), of(value(0)))
    }

    private fun textFor(lang: Lang): RemainingText {
        val table = StringTable(
            mapOf(
                Lang.JA to mapOf(
                    "voice_remaining_today" to "今日の残り %1\$d",
                    "voice_remaining_a11y_low" to "%1\$s。少なくなっています",
                    "voice_remaining_a11y_pro" to "Pro。%1\$s",
                ),
                Lang.EN to mapOf(
                    "voice_remaining_today" to "%1\$d left today",
                    "voice_remaining_a11y_low" to "%1\$s. Running low",
                    "voice_remaining_a11y_pro" to "Pro. %1\$s",
                ),
            ),
        )
        return { key, args -> table.format(lang, key, *args) }
    }

    @Test
    fun labelHasNoUnitAndHidesWhenUnknownOrZero() {
        assertEquals("今日の残り 20", RemainingTodayDisplay.Normal(20, false).label(textFor(Lang.JA)))
        assertEquals("3 left today", RemainingTodayDisplay.Low(3, false).label(textFor(Lang.EN)))
        assertNull(RemainingTodayDisplay.Hidden.label(textFor(Lang.JA)))
        assertNull(RemainingTodayDisplay.Exhausted(reset).label(textFor(Lang.JA)))
    }

    @Test
    fun a11yAddsLowAndPro() {
        assertEquals("今日の残り 20", RemainingTodayDisplay.Normal(20, false).a11y(textFor(Lang.JA)))
        assertEquals("今日の残り 3。少なくなっています", RemainingTodayDisplay.Low(3, false).a11y(textFor(Lang.JA)))
        assertEquals("Pro. 3 left today. Running low", RemainingTodayDisplay.Low(3, true).a11y(textFor(Lang.EN)))
        assertNull(RemainingTodayDisplay.Hidden.a11y(textFor(Lang.EN)))
    }

    @Test
    fun realBundleHasTheAndroidKeysInBothLanguages() {
        val table = StringTable.fromClasspath()
        for (lang in Lang.entries) {
            for (key in listOf("voice_remaining_today", "voice_remaining_a11y_low", "voice_remaining_a11y_pro")) {
                assertEquals("$lang $key", true, table.hasString(lang, key))
            }
        }
        assertEquals("5 left today", table.format(Lang.EN, "voice_remaining_today", 5))
    }
}
