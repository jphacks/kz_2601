package jp.kz2601.metamon.renderer

import jp.kz2601.metamon.dsl.Metric
import jp.kz2601.metamon.dsl.Period
import jp.kz2601.metamon.renderer.components.health.HealthStatRules
import jp.kz2601.metamon.renderer.components.health.MockHealthSource
import jp.kz2601.metamon.renderer.theme.SymbolIcons
import kotlinx.coroutines.runBlocking
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import java.time.LocalDate
import java.util.Locale

/** `healthStat`(#870)の日の区切り・書式・モックの固定値。iOS の HealthDataTests と同じ性質を見る。 */
class HealthStatTest {
    private val today = LocalDate.of(2026, 10, 25)

    @Test
    fun dayStartsAreOldestFirstAndEndToday() {
        val starts = HealthStatRules.dayStarts(7, today)
        assertEquals(7, starts.size)
        assertEquals(today, starts.last())
        assertEquals(LocalDate.of(2026, 10, 19), starts.first())
        assertEquals("0 日でも 1 日は返す", 1, HealthStatRules.dayStarts(0, today).size)
    }

    @Test
    fun daysFollowMetricAndPeriod() {
        assertEquals(1, HealthStatRules.days(Metric.STEPS, null))
        assertEquals(1, HealthStatRules.days(Metric.STEPS, Period.TODAY))
        assertEquals(7, HealthStatRules.days(Metric.STEPS, Period.WEEK))
        // 体重の today は最新の記録を探しに 30 日さかのぼる(iOS と同じ)
        assertEquals(30, HealthStatRules.days(Metric.WEIGHT, Period.TODAY))
        assertEquals(7, HealthStatRules.days(Metric.WEIGHT, Period.WEEK))
    }

    @Test
    fun stepsAreGroupedAndWeightHasOneDecimal() {
        assertEquals("6,240", HealthStatRules.stepsText(6240.0, Locale.US))
        assertEquals("0", HealthStatRules.stepsText(-3.0, Locale.US))
        assertEquals("61.8", HealthStatRules.weightText(61.75, Locale.US))
        assertEquals("61.8", HealthStatRules.valueText(Metric.WEIGHT, 61.8, Locale.US))
        assertEquals("1,000", HealthStatRules.valueText(Metric.STEPS, 1000.4, Locale.US))
    }

    @Test
    fun targetGivesPercentAndProgress() {
        assertEquals(78, HealthStatRules.achievedPercent(6240.0, 8000.0))
        assertEquals(0.78f, HealthStatRules.progress(6240.0, 8000.0), 0.001f)
        assertEquals("超えてもバーは 1 で止める", 1f, HealthStatRules.progress(9000.0, 8000.0), 0.0f)
        assertNull(HealthStatRules.achievedPercent(100.0, null))
        assertNull(HealthStatRules.achievedPercent(100.0, 0.0))
    }

    @Test
    fun weekSummaries() = runBlocking {
        val week = MockHealthSource.steps(7)
        assertEquals(MockHealthSource.stepSeries, week.map { it.value })
        assertEquals(7362.9, HealthStatRules.averageSteps(week), 0.1)
        val weights = MockHealthSource.weights(7)
        assertEquals(-0.6, HealthStatRules.weightDelta(weights)!!, 0.001)
        assertEquals("-0.6", HealthStatRules.signedWeightText(-0.6, Locale.US))
        assertEquals("+0.6", HealthStatRules.signedWeightText(0.6, Locale.US))
        assertNull(HealthStatRules.weightDelta(weights.take(1)))
    }

    /** 系列より長い日数を聞かれても古い日を 0 で詰めない(iOS の MockHealthData と同じ)。 */
    @Test
    fun mockDoesNotPadOlderDays() = runBlocking {
        val weights = MockHealthSource.weights(30)
        assertEquals(7, weights.size)
        assertEquals(61.8, weights.last().value, 0.001)
        assertEquals(LocalDate.now(), weights.last().day)
        assertEquals(listOf(6240.0), MockHealthSource.steps(1).map { it.value })
    }

    /** 見出しの印は対応表にある名前(丸に落ちない)。 */
    @Test
    fun iconsAreInTheTable() {
        assertEquals("figure.walk", HealthStatRules.icon(Metric.STEPS))
        assertEquals("scalemass", HealthStatRules.icon(Metric.WEIGHT))
        for (m in listOf(Metric.STEPS, Metric.WEIGHT, null)) assertNotNull(SymbolIcons.vector(HealthStatRules.icon(m)))
        assertTrue(HealthStatRules.isWeight(Metric.WEIGHT))
        assertTrue(!HealthStatRules.isWeight(null))
    }
}
