package jp.kz2601.metamon.settings

import jp.kz2601.metamon.services.RemainingToday
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import java.time.Instant

/** #630 設定の「今日の残り」: 数・上限・Pro・少ない・戻る時刻(iOS の RemainingTodaySettingsSection と同じ出し分け)。 */
class RemainingTodayRowTest {
    private val now = Instant.parse("2026-11-11T03:00:00Z") // 12:00 JST
    private val reset = Instant.parse("2026-11-11T15:00:00Z") // 翌 0:00 JST

    @Test
    fun unknownUntilAiIsUsed() {
        val row = RemainingTodayRow.of(null, now, isPro = true)
        assertNull("まだ分からない時は「AI を使うと表示されます」", row.amount)
        assertFalse("数が無い時は Pro の印を出さない", row.pro)
        assertNull(row.resetAt)
    }

    @Test
    fun showsRemainingOverLimitAndResetTime() {
        val row = RemainingTodayRow.of(RemainingToday(24, 60, reset, now), now, isPro = false)
        assertEquals("24 / 60", row.amount)
        assertFalse(row.low)
        assertFalse(row.pro)
        assertEquals(reset, row.resetAt)
    }

    @Test
    fun countOnlyWhenLimitIsUnknown() {
        assertEquals("24", RemainingTodayRow.of(RemainingToday(24, null, reset, now), now, isPro = false).amount)
    }

    @Test
    fun lowAndProAreMarked() {
        val row = RemainingTodayRow.of(RemainingToday(6, 300, reset, now), now, isPro = true)
        assertTrue("新しく作るのが 2 回分以下なら目立たせる", row.low)
        assertTrue(row.pro)
        assertFalse(RemainingTodayRow.of(RemainingToday(7, 300, reset, now), now, isPro = true).low)
    }

    @Test
    fun staleValueAfterResetIsHidden() {
        val row = RemainingTodayRow.of(RemainingToday(0, 60, reset, now), reset.plusSeconds(1), isPro = false)
        assertNull("戻る時刻を過ぎた値は出さない", row.amount)
        assertNull(row.resetAt)
    }
}
