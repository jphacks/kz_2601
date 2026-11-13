package jp.kz2601.metamon.whisper.desktop

import org.junit.Assert.assertArrayEquals
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Assert.fail
import org.junit.Rule
import org.junit.Test
import org.junit.rules.TemporaryFolder
import java.io.EOFException
import java.io.File
import java.nio.ByteBuffer
import java.nio.ByteOrder
import javax.sound.sampled.UnsupportedAudioFileException

/** 手元の確かめの wav の読み込み(readWav・#2800)。wav はテストの中で作る。ネイティブのライブラリは読まない。 */
class ReadWavTest {
    @get:Rule
    val tmp = TemporaryFolder()

    @Test
    fun reads16BitMono16kAsIs() {
        val f = wav(pcm16(0, 16384, -16384, 32767, -32768), rate = 16_000, channels = 1, bits = 16)
        assertArrayEquals(floatArrayOf(0f, 0.5f, -0.5f, 32767 / 32768f, -1f), readWav(f), 1e-6f)
    }

    @Test
    fun emptyDataIsEmpty() {
        assertEquals(0, readWav(wav(ByteArray(0), rate = 16_000, channels = 1, bits = 16)).size)
    }

    @Test
    fun stereoIsAveragedToMono() {
        // (L, R) = (0.5, 0), (-0.5, -0.5)
        val f = wav(pcm16(16384, 0, -16384, -16384), rate = 16_000, channels = 2, bits = 16)
        assertArrayEquals(floatArrayOf(0.25f, -0.5f), readWav(f), 1e-6f)
    }

    @Test
    fun unsigned8BitIsWidenedTo16Bit() {
        // 8bit の wav は符号なし(128 が 0)。192 = +64 → 0.5、64 = -64 → -0.5
        val f = wav(byteArrayOf(128.toByte(), 192.toByte(), 64.toByte()), rate = 16_000, channels = 1, bits = 8)
        // Java Sound は下の 8bit を埋めて広げるので、8bit の 1 段(1/128)までの差は許す
        assertArrayEquals(floatArrayOf(0f, 0.5f, -0.5f), readWav(f), 1 / 128f)
    }

    @Test
    fun otherRatesAreResampledTo16k() {
        val f = wav(pcm16(*IntArray(4_800) { 8192 }), rate = 48_000, channels = 1, bits = 16)
        val out = readWav(f)
        // 0.1 秒 → 16kHz で約 1600 点、値は変わらない
        assertTrue("size=${out.size}", out.size in 1_595..1_600)
        assertTrue(out.all { kotlin.math.abs(it - 0.25f) < 1e-4f })
    }

    @Test
    fun notRiffIsRejected() {
        val f = tmp.newFile("not.wav").apply { writeBytes("hello, this is not a wav file at all".toByteArray()) }
        assertRejected(f)
    }

    @Test
    fun truncatedHeaderIsRejected() {
        val full = wav(pcm16(1, 2, 3), rate = 16_000, channels = 1, bits = 16).readBytes()
        val f = tmp.newFile("short.wav").apply { writeBytes(full.copyOf(20)) }
        assertRejected(f)
    }

    @Test
    fun missingFileThrows() {
        try {
            readWav(File(tmp.root, "missing.wav"))
            fail("読めてしまった")
        } catch (e: java.io.IOException) {
            // FileNotFoundException(IOException)
        }
    }

    private fun assertRejected(f: File) {
        try {
            readWav(f)
            fail("読めてしまった")
        } catch (e: UnsupportedAudioFileException) {
        } catch (e: EOFException) {
        }
    }

    private fun pcm16(vararg v: Int): ByteArray {
        val b = ByteBuffer.allocate(v.size * 2).order(ByteOrder.LITTLE_ENDIAN)
        v.forEach { b.putShort(it.toShort()) }
        return b.array()
    }

    /** RIFF / WAVE / fmt (PCM) / data の最小の wav。 */
    private fun wav(data: ByteArray, rate: Int, channels: Int, bits: Int): File {
        val blockAlign = channels * bits / 8
        val b = ByteBuffer.allocate(44 + data.size).order(ByteOrder.LITTLE_ENDIAN)
        b.put("RIFF".toByteArray()).putInt(36 + data.size).put("WAVE".toByteArray())
        b.put("fmt ".toByteArray()).putInt(16).putShort(1).putShort(channels.toShort())
            .putInt(rate).putInt(rate * blockAlign).putShort(blockAlign.toShort()).putShort(bits.toShort())
        b.put("data".toByteArray()).putInt(data.size).put(data)
        return tmp.newFile().apply { writeBytes(b.array()) }
    }
}
