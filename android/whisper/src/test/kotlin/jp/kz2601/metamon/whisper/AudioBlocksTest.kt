package jp.kz2601.metamon.whisper

import org.junit.Assert.assertArrayEquals
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import kotlin.math.abs
import kotlin.math.sin

class AudioBlocksTest {
    @Test
    fun pcm16IsDownmixedToMonoFloat() {
        val stereo = shortArrayOf(16384, -16384, 32767, 32767, -32768, 0)
        val mono = WhisperAudio.monoFromPcm16(stereo, channels = 2)
        assertEquals(3, mono.size)
        assertEquals(0f, mono[0], 1e-6f)
        assertEquals(32767f / 32768f, mono[1], 1e-6f)
        assertEquals(-0.5f, mono[2], 1e-6f)
        assertArrayEquals(floatArrayOf(0.5f), WhisperAudio.monoFromPcm16(shortArrayOf(16384), 1), 1e-6f)
    }

    @Test
    fun floatPcmIsDownmixed() {
        assertArrayEquals(floatArrayOf(0.5f, -0.25f), WhisperAudio.monoFromFloat(floatArrayOf(1f, 0f, -0.5f, 0f), 2), 1e-6f)
        assertArrayEquals(floatArrayOf(0.1f, 0.2f), WhisperAudio.monoFromFloat(floatArrayOf(0.1f, 0.2f), 1), 1e-6f)
    }

    @Test
    fun sameRateIsCopied() {
        val r = StreamingResampler(16_000)
        assertArrayEquals(floatArrayOf(1f, 2f), r.process(floatArrayOf(1f, 2f)), 0f)
    }

    @Test
    fun downsamplingKeepsDurationAndShape() {
        val from = 48_000
        val input = FloatArray(from) { sin(2 * Math.PI * 440 * it / from).toFloat() } // 1 秒
        val out = StreamingResampler(from).process(input)
        assertTrue("16kHz で約 1 秒(${out.size})", abs(out.size - 16_000) <= 2)
        // 3 点に 1 点を取る: 出力の n 番目は入力の 3n 番目と同じ
        for (n in listOf(0, 100, 5000, 15000)) assertEquals(input[n * 3], out[n], 1e-5f)
    }

    @Test
    fun chunkedResamplingMatchesOneShot() {
        val from = 44_100
        val input = FloatArray(44_100 * 2) { sin(it * 0.01).toFloat() }
        val whole = StreamingResampler(from).process(input)
        val r = StreamingResampler(from)
        val parts = listOf(0..999, 1000..1000, 1001..33332, 33333 until input.size).map { r.process(input.sliceArray(it)) }
        val joined = parts.fold(FloatArray(0)) { acc, p -> acc + p }
        assertEquals(whole.size, joined.size)
        for (i in whole.indices) assertEquals("at $i", whole[i], joined[i], 1e-5f)
    }

    @Test
    fun upsamplingInterpolates() {
        val out = StreamingResampler(8_000).process(floatArrayOf(0f, 1f, 0f))
        assertArrayEquals(floatArrayOf(0f, 0.5f, 1f, 0.5f), out, 1e-6f)
    }

    @Test
    fun blocksAreCutAtTheBlockSizeAndTheRestIsKept() {
        val acc = BlockAccumulator(blockSamples = 4)
        assertTrue(acc.add(floatArrayOf(1f, 2f, 3f)).isEmpty())
        val full = acc.add(floatArrayOf(4f, 5f, 6f, 7f, 8f, 9f))
        assertEquals(2, full.size)
        assertArrayEquals(floatArrayOf(1f, 2f, 3f, 4f), full[0], 0f)
        assertArrayEquals(floatArrayOf(5f, 6f, 7f, 8f), full[1], 0f)
        assertArrayEquals(floatArrayOf(9f), acc.finish(), 0f)
        assertNull(acc.finish())
    }

    @Test
    fun blockIsThirtySecondsOf16kHz() {
        assertEquals(480_000, WhisperAudio.BLOCK_SAMPLES)
    }

    @Test
    fun shortOrSilentAudioIsSkipped() {
        assertFalse("0.5 秒未満", WhisperAudio.hasVoice(FloatArray(7_999) { 0.5f }))
        assertFalse("無音", WhisperAudio.hasVoice(FloatArray(16_000) { 0.0005f }))
        assertTrue(WhisperAudio.hasVoice(FloatArray(8_000) { if (it == 10) -0.2f else 0f }))
    }

    @Test
    fun partsAreTrimmedAndJoinedWithoutBlanks() {
        assertEquals("こんにちは\nhello", WhisperAudio.join(listOf(" こんにちは ", "", "  ", "hello")))
        assertEquals("", WhisperAudio.join(emptyList()))
    }

    @Test
    fun onlyJapaneseAndEnglishAreUsed() {
        assertEquals("ja", WhisperEngine.whisperLanguage("ja"))
        assertEquals("ja", WhisperEngine.whisperLanguage("ja-JP"))
        assertEquals("en", WhisperEngine.whisperLanguage("en"))
        assertEquals("en", WhisperEngine.whisperLanguage("en-US"))
        assertEquals("ja", WhisperEngine.whisperLanguage("fr"))
    }
}
