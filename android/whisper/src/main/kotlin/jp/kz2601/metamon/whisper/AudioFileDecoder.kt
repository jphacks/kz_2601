package jp.kz2601.metamon.whisper

import android.content.Context
import android.media.AudioFormat
import android.media.MediaCodec
import android.media.MediaExtractor
import android.media.MediaFormat
import android.media.MediaMetadataRetriever
import android.net.Uri
import kotlinx.coroutines.currentCoroutineContext
import kotlinx.coroutines.ensureActive
import java.io.IOException
import java.nio.ByteOrder

/** 録音ファイルを読めない(音の入っていないファイル・端末に復号器が無い形式)。 */
class AudioDecodeException(message: String, cause: Throwable? = null) : IOException(message, cause)

/**
 * 録音ファイル(m4a・mp3・wav・ogg など端末の MediaCodec が読めるもの)を先頭から読み、16kHz モノラルの float にして少しずつ渡す。
 * ファイル全体をメモリに載せない。ファイルは読むだけで、コピーも送信もしない。
 */
object AudioFileDecoder {
    private const val TIMEOUT_US = 10_000L

    /** 長さ(秒)。読めなければ null。 */
    fun durationSeconds(context: Context, uri: Uri): Double? {
        val retriever = MediaMetadataRetriever()
        return try {
            retriever.setDataSource(context, uri)
            retriever.extractMetadata(MediaMetadataRetriever.METADATA_KEY_DURATION)?.toLongOrNull()?.let { it / 1000.0 }
        } catch (_: RuntimeException) {
            null
        } finally {
            runCatching { retriever.release() }
        }
    }

    /** [onSamples] に 16kHz モノラルの float を順に渡す。呼んだ coroutine のキャンセルで止まる。 */
    suspend fun decode(context: Context, uri: Uri, onSamples: suspend (FloatArray) -> Unit) {
        val extractor = MediaExtractor()
        var codec: MediaCodec? = null
        try {
            try {
                extractor.setDataSource(context, uri, null)
            } catch (e: IOException) {
                throw AudioDecodeException("cannot open", e)
            }
            val track = (0 until extractor.trackCount).firstOrNull {
                extractor.getTrackFormat(it).getString(MediaFormat.KEY_MIME)?.startsWith("audio/") == true
            } ?: throw AudioDecodeException("no audio track")
            extractor.selectTrack(track)
            val format = extractor.getTrackFormat(track)
            val mime = format.getString(MediaFormat.KEY_MIME) ?: throw AudioDecodeException("no mime")
            val c = try {
                MediaCodec.createDecoderByType(mime).also { codec = it }
            } catch (e: IOException) {
                throw AudioDecodeException("no decoder for $mime", e)
            } catch (e: IllegalArgumentException) {
                throw AudioDecodeException("no decoder for $mime", e)
            }
            c.configure(format, null, null, 0)
            c.start()

            var channels = format.intOr(MediaFormat.KEY_CHANNEL_COUNT, 1)
            var encoding = AudioFormat.ENCODING_PCM_16BIT
            var resampler = StreamingResampler(format.intOr(MediaFormat.KEY_SAMPLE_RATE, WhisperAudio.SAMPLE_RATE))
            val info = MediaCodec.BufferInfo()
            var inputDone = false
            while (true) {
                currentCoroutineContext().ensureActive()
                if (!inputDone) {
                    val index = c.dequeueInputBuffer(TIMEOUT_US)
                    if (index >= 0) {
                        val buffer = c.getInputBuffer(index) ?: throw AudioDecodeException("no input buffer")
                        val size = extractor.readSampleData(buffer, 0)
                        if (size < 0) {
                            c.queueInputBuffer(index, 0, 0, 0, MediaCodec.BUFFER_FLAG_END_OF_STREAM)
                            inputDone = true
                        } else {
                            c.queueInputBuffer(index, 0, size, extractor.sampleTime, 0)
                            extractor.advance()
                        }
                    }
                }
                val out = c.dequeueOutputBuffer(info, TIMEOUT_US)
                if (out == MediaCodec.INFO_OUTPUT_FORMAT_CHANGED) {
                    val f = c.outputFormat
                    channels = f.intOr(MediaFormat.KEY_CHANNEL_COUNT, channels)
                    encoding = f.intOr(MediaFormat.KEY_PCM_ENCODING, AudioFormat.ENCODING_PCM_16BIT)
                    resampler = StreamingResampler(f.intOr(MediaFormat.KEY_SAMPLE_RATE, WhisperAudio.SAMPLE_RATE))
                } else if (out >= 0) {
                    val mono = c.getOutputBuffer(out)?.let { buffer ->
                        buffer.position(info.offset)
                        buffer.limit(info.offset + info.size)
                        val ordered = buffer.slice().order(ByteOrder.nativeOrder())
                        if (encoding == AudioFormat.ENCODING_PCM_FLOAT) {
                            val fb = ordered.asFloatBuffer()
                            WhisperAudio.monoFromFloat(FloatArray(fb.remaining()).also { fb.get(it) }, channels)
                        } else {
                            val sb = ordered.asShortBuffer()
                            WhisperAudio.monoFromPcm16(ShortArray(sb.remaining()).also { sb.get(it) }, channels)
                        }
                    }
                    val end = info.flags and MediaCodec.BUFFER_FLAG_END_OF_STREAM != 0
                    c.releaseOutputBuffer(out, false)
                    if (mono != null && mono.isNotEmpty()) onSamples(resampler.process(mono))
                    if (end) break
                }
            }
        } catch (e: IllegalStateException) {
            throw AudioDecodeException("codec failed", e)
        } finally {
            codec?.let {
                runCatching { it.stop() }
                runCatching { it.release() }
            }
            extractor.release()
        }
    }

    private fun MediaFormat.intOr(key: String, fallback: Int): Int = if (containsKey(key)) getInteger(key) else fallback
}
