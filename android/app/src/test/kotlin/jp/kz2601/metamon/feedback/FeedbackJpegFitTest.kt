package jp.kz2601.metamon.feedback

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

class FeedbackJpegFitTest {
    @Test
    fun lowersQualityFirstThenSize() {
        val tried = mutableListOf<Pair<Int, Int>>()
        // 大きさ = 画素の割合 × 画質 × 100
        val got = FeedbackScreenshot.fit(maxBytes = 1_500) { scale, q ->
            tried += Math.round(scale * 10) to q
            ByteArray((scale * scale * q * 100).toInt())
        }
        assertEquals(listOf(10 to 70, 10 to 50, 10 to 35, 10 to 20, 7 to 70, 7 to 50, 7 to 35, 7 to 20), tried)
        assertTrue(got!!.size <= 1_500)
    }

    @Test
    fun givesUpAfterFourSizes() {
        var calls = 0
        assertNull(FeedbackScreenshot.fit(maxBytes = 10) { _, _ -> calls++; ByteArray(11) })
        assertEquals(16, calls)
    }
}
