package jp.kz2601.metamon.whisper.desktop

import jp.kz2601.metamon.whisper.StreamingResampler
import jp.kz2601.metamon.whisper.WhisperAudio
import java.nio.ByteBuffer
import java.nio.ByteOrder
import javax.sound.sampled.AudioFormat

/** Java Sound の PCM(16bit 符号付き)を whisper に渡す形(16kHz・モノラル・float)にする。 */
object Pcm {
    /** whisper にそのまま渡せる録音の形(16kHz・16bit・モノラル・リトルエンディアン)。 */
    val WHISPER_FORMAT = AudioFormat(WhisperAudio.SAMPLE_RATE.toFloat(), 16, 1, true, false)

    /** 16bit 符号付きの PCM のバイト列 → short(チャンネルは交互のまま)。 */
    fun shorts(bytes: ByteArray, length: Int = bytes.size, bigEndian: Boolean = false): ShortArray {
        val sb = ByteBuffer.wrap(bytes, 0, length - length % 2).order(if (bigEndian) ByteOrder.BIG_ENDIAN else ByteOrder.LITTLE_ENDIAN).asShortBuffer()
        return ShortArray(sb.remaining()).also { sb.get(it) }
    }

    /** [format](16bit 符号付き)のバイト列 → 16kHz モノラルの float。区切って渡す時は同じ [resampler] を使う。 */
    fun toWhisper(bytes: ByteArray, length: Int, format: AudioFormat, resampler: StreamingResampler): FloatArray {
        require(format.encoding == AudioFormat.Encoding.PCM_SIGNED && format.sampleSizeInBits == 16) { "16bit の PCM だけ: $format" }
        val mono = WhisperAudio.monoFromPcm16(shorts(bytes, length, format.isBigEndian), format.channels)
        return resampler.process(mono)
    }
}
