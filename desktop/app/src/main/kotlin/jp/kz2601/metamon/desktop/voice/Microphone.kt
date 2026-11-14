package jp.kz2601.metamon.desktop.voice

import jp.kz2601.metamon.whisper.StreamingResampler
import jp.kz2601.metamon.whisper.desktop.Pcm
import java.io.ByteArrayOutputStream
import javax.sound.sampled.AudioFormat
import javax.sound.sampled.AudioSystem
import javax.sound.sampled.DataLine
import javax.sound.sampled.LineUnavailableException
import javax.sound.sampled.Mixer
import javax.sound.sampled.TargetDataLine
import kotlin.concurrent.thread

/** マイクを開けない理由。 */
class MicException(val reason: Reason, cause: Throwable? = null) : Exception(reason.name, cause) {
    enum class Reason {
        /** 入力装置が無い(選んだ名前の装置も無い) */
        NO_DEVICE,

        /** ほかのアプリが使っている・OS が開かせない */
        BUSY,
    }
}

/** 録音の口(本物は [JavaSoundMicrophone]・テストは偽物)。 */
interface Microphone {
    /** 選べる入力装置の名前。 */
    fun devices(): List<String>

    /** 録音を始める。[device] が null なら OS の既定。開けなければ [MicException]。 */
    fun start(device: String? = null): Recording

    interface Recording {
        /** 止めて、録った音を 16kHz モノラルの float で返す。 */
        fun stop(): FloatArray

        /** 止めて捨てる。 */
        fun cancel()
    }
}

/**
 * Java Sound(javax.sound.sampled)のマイク。16kHz・16bit・モノラルで開き、開けない装置は 48k / 44.1k(ステレオも)で開いて 16kHz に直す。
 * 録る長さは [maxSeconds] まで(それより後は捨てる・メモリを使い過ぎない)。
 * Windows の「デスクトップ アプリにマイクへのアクセスを許可する」が切れていると、開けても音が全部 0 になる(VoiceInput が見分けて知らせる)。
 */
class JavaSoundMicrophone(private val maxSeconds: Int = MAX_SECONDS) : Microphone {
    override fun devices(): List<String> = inputMixers().map { it.name }

    override fun start(device: String?): Microphone.Recording {
        val mixers = inputMixers()
        if (mixers.isEmpty()) throw MicException(MicException.Reason.NO_DEVICE)
        val chosen = device?.let { name -> mixers.firstOrNull { it.name == name } ?: throw MicException(MicException.Reason.NO_DEVICE) }
        var lastError: Exception? = null
        for (format in FORMATS) {
            val info = DataLine.Info(TargetDataLine::class.java, format)
            val line = try {
                (if (chosen != null) chosen.mixer.getLine(info) else AudioSystem.getLine(info)) as TargetDataLine
            } catch (e: IllegalArgumentException) {
                lastError = e
                continue
            } catch (e: LineUnavailableException) {
                lastError = e
                continue
            } catch (e: SecurityException) {
                throw MicException(MicException.Reason.BUSY, e)
            }
            try {
                line.open(format)
            } catch (e: LineUnavailableException) {
                lastError = e
                continue
            } catch (e: IllegalArgumentException) {
                lastError = e
                continue
            }
            line.start()
            return LineRecording(line, format, maxSeconds)
        }
        throw MicException(if (lastError is LineUnavailableException) MicException.Reason.BUSY else MicException.Reason.NO_DEVICE, lastError)
    }

    private class NamedMixer(val name: String, val mixer: Mixer)

    private fun inputMixers(): List<NamedMixer> = AudioSystem.getMixerInfo().mapNotNull { info ->
        val mixer = runCatching { AudioSystem.getMixer(info) }.getOrNull() ?: return@mapNotNull null
        if (mixer.targetLineInfo.none { it is DataLine.Info && TargetDataLine::class.java.isAssignableFrom(it.lineClass) }) return@mapNotNull null
        NamedMixer(info.name, mixer)
    }

    private class LineRecording(private val line: TargetDataLine, private val format: AudioFormat, maxSeconds: Int) : Microphone.Recording {
        private val out = ByteArrayOutputStream()
        private val maxBytes = format.frameSize.toLong() * format.frameRate.toLong() * maxSeconds

        @Volatile
        private var running = true
        private val reader = thread(isDaemon = true, name = "mic") {
            val buf = ByteArray((format.frameSize * format.frameRate / 10).toInt().coerceAtLeast(format.frameSize))
            while (running) {
                val n = line.read(buf, 0, buf.size)
                if (n <= 0) continue
                synchronized(out) { if (out.size() < maxBytes) out.write(buf, 0, n) }
            }
        }

        override fun stop(): FloatArray {
            close()
            val bytes = synchronized(out) { out.toByteArray() }
            return Pcm.toWhisper(bytes, bytes.size, format, StreamingResampler(format.sampleRate.toInt()))
        }

        override fun cancel() = close()

        private fun close() {
            running = false
            line.stop()
            line.flush()
            line.close()
            reader.join(1_000)
        }
    }

    companion object {
        /** 1 回に録る長さの上限(秒)。 */
        const val MAX_SECONDS = 120

        /** 開く形の順。16kHz モノラルが開ければ直さずに済む。 */
        val FORMATS = listOf(
            Pcm.WHISPER_FORMAT,
            AudioFormat(48_000f, 16, 1, true, false),
            AudioFormat(44_100f, 16, 1, true, false),
            AudioFormat(48_000f, 16, 2, true, false),
            AudioFormat(44_100f, 16, 2, true, false),
        )
    }
}
