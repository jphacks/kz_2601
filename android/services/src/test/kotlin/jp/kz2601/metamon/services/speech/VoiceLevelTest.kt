package jp.kz2601.metamon.services.speech

import org.junit.Assert.assertEquals
import org.junit.Test

/** #1014: onRmsChanged の dB → 0..1(マイクの輪の大きさ)。 */
class VoiceLevelTest {
    @Test
    fun rmsDbMapsToZeroOneAndClamps() {
        assertEquals(0f, VoiceLevel.fromRmsDb(-2f), 0.001f)
        assertEquals(0f, VoiceLevel.fromRmsDb(-10f), 0.001f)
        assertEquals(0.5f, VoiceLevel.fromRmsDb(4f), 0.001f)
        assertEquals(1f, VoiceLevel.fromRmsDb(10f), 0.001f)
        assertEquals(1f, VoiceLevel.fromRmsDb(30f), 0.001f)
        assertEquals(0f, VoiceLevel.fromRmsDb(Float.NaN), 0.001f)
    }

    @Test
    fun smoothRisesAtOnceAndFallsSlowly() {
        assertEquals(0.8f, VoiceLevel.smooth(0.2f, 0.8f), 0.001f)
        assertEquals(0.56f, VoiceLevel.smooth(0.8f, 0f), 0.001f)
        assertEquals(0.5f, VoiceLevel.smooth(0.5f, 0.5f), 0.001f)
    }
}
