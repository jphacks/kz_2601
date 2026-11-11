package jp.kz2601.metamon.feedback

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class ShakeDetectorTest {
    private val strong = 30f // 約 3G
    private val rest = 9.8f

    private fun ShakeDetector.feed(vararg samples: Pair<Long, Float>): List<Long> =
        samples.filter { (t, a) -> onSample(a, 0f, 0f, t) }.map { it.first }

    @Test
    fun restingOrWalkingIsNotAShake() {
        val d = ShakeDetector()
        // 置いたまま(1G)・歩く程度(1.5G)
        val fired = d.feed(*(0L until 3_000L step 20).map { it to if (it % 400 == 0L) 15f else rest }.toTypedArray())
        assertTrue(fired.isEmpty())
    }

    @Test
    fun threeStrongPeaksWithinTheWindowIsOneShake() {
        val d = ShakeDetector()
        assertEquals(listOf(400L), d.feed(0L to strong, 200L to strong, 400L to strong))
    }

    @Test
    fun peaksTooFarApartAreNotAShake() {
        val d = ShakeDetector()
        assertTrue(d.feed(0L to strong, 600L to strong, 1_200L to strong, 1_800L to strong).isEmpty())
    }

    @Test
    fun samplesOfTheSamePeakCountOnce() {
        val d = ShakeDetector()
        // 1 つの山に 3 サンプル(10ms 間隔)は 1 回
        assertTrue(d.feed(0L to strong, 10L to strong, 20L to strong).isEmpty())
    }

    @Test
    fun keepShakingOpensOnlyOnceDuringCooldown() {
        val d = ShakeDetector()
        val samples = (0L..2_200L step 150).map { it to strong }.toTypedArray()
        val fired = d.feed(*samples)
        assertEquals(listOf(300L, 2_100L), fired)
    }

    @Test
    fun resetForgetsPartialShakes() {
        val d = ShakeDetector()
        d.feed(0L to strong, 200L to strong)
        d.reset()
        assertFalse(d.onSample(strong, 0f, 0f, 300L))
    }
}
