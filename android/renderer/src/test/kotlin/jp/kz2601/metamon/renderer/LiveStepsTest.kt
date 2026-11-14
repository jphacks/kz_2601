package jp.kz2601.metamon.renderer

import jp.kz2601.metamon.renderer.components.health.HealthDailyValue
import jp.kz2601.metamon.renderer.components.health.LiveStepDelta
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test
import java.time.LocalDate

/** `healthStat` の `live`(#3694): TYPE_STEP_COUNTER の累計 → 増分、一発読みの今日に足す(iOS の HealthLiveStepsTests と同じ性質)。 */
class LiveStepsTest {
    @Test
    fun firstEventIsTheAnchorAndLaterEventsAreDeltas() {
        val d = LiveStepDelta()
        assertEquals(0.0, d.delta(123_456f), 0.0)
        assertEquals(7.0, d.delta(123_463f), 0.0)
        assertEquals("累計なので置き換え(足さない)", 30.0, d.delta(123_486f), 0.0)
    }

    @Test
    fun counterGoingBackwardsNeverGoesNegative() {
        val d = LiveStepDelta()
        d.delta(500f)
        assertEquals(0.0, d.delta(480f), 0.0)
    }

    @Test
    fun applyAddsDeltaToTodayOnly() {
        val week = (0 until 7).map { HealthDailyValue(LocalDate.of(2026, 11, 8).plusDays(it.toLong()), 1000.0 * (it + 1)) }
        val out = LiveStepDelta.apply(week, 12.0)
        assertEquals(7, out.size)
        assertEquals(week.dropLast(1), out.dropLast(1))
        assertEquals(7012.0, out.last().value, 0.0)
        assertEquals(week.last().day, out.last().day)
    }

    @Test
    fun applyOnEmptyBaselineIsNoop() {
        assertTrue(LiveStepDelta.apply(emptyList(), 5.0).isEmpty())
    }

    @Test
    fun newDeltaAfterReloadDoesNotCountTwice() {
        // 前面に戻って一発読みを取り直したら、増分も最初の値から数え直す
        val before = LiveStepDelta()
        before.delta(100f)
        assertEquals(40.0, before.delta(140f), 0.0)
        val baseline = listOf(HealthDailyValue(LocalDate.of(2026, 11, 14), 6280.0)) // 140 歩ぶんを含む値
        val after = LiveStepDelta()
        assertEquals(6280.0, LiveStepDelta.apply(baseline, after.delta(140f)).single().value, 0.0)
        assertEquals(6283.0, LiveStepDelta.apply(baseline, after.delta(143f)).single().value, 0.0)
    }
}
