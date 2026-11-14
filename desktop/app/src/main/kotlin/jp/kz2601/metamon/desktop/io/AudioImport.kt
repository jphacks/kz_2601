package jp.kz2601.metamon.desktop.io

import jp.kz2601.metamon.desktop.i18n.Lang
import jp.kz2601.metamon.desktop.i18n.Strings
import jp.kz2601.metamon.desktop.voice.Transcriber
import jp.kz2601.metamon.desktop.voice.VoiceFlag
import jp.kz2601.metamon.desktop.voice.VoiceInput
import jp.kz2601.metamon.desktop.voice.WhisperCppTranscriber
import jp.kz2601.metamon.whisper.ModelException
import jp.kz2601.metamon.whisper.WhisperUnavailableException
import jp.kz2601.metamon.whisper.desktop.readWav
import kotlinx.coroutines.CancellationException
import java.io.File
import java.util.Locale

/**
 * fileImport の録音ファイルを端末の中で文字にする(#1751・iOS / Android の AudioFileTranscription と同じ)。
 * 音も文字もサーバーに送らない。[transcriber] が null は音声入力が off。読めるのは wav だけ(m4a・mp3 の変換の依存は足さない)。
 */
class AudioImport(
    private val transcriber: Transcriber?,
    private val language: () -> String = { if (Strings.lang == Lang.EN) "en" else "ja" },
    private val decode: (File) -> FloatArray = ::readWav,
) {
    sealed interface Result {
        data class Text(val text: String) : Result

        /** 文字にしなかった理由([message] は画面にそのまま出す一言)。 */
        data class Refused(val message: String) : Result
    }

    suspend fun transcribe(file: File): Result {
        val t = transcriber ?: return Result.Refused(Strings["desktop_io_audio_off"])
        if (file.extension.lowercase(Locale.ROOT) != "wav") return Result.Refused(Strings["desktop_io_audio_wav_only"])
        if (!t.isAvailable) return Result.Refused(Strings["desktop_voice_unavailable"])
        if (file.length() > MAX_BYTES) return Result.Refused(Strings["desktop_io_file_too_large"])
        val samples = try {
            decode(file)
        } catch (e: CancellationException) {
            throw e
        } catch (e: Exception) {
            return Result.Refused(Strings["desktop_io_audio_unreadable"])
        }
        return try {
            val text = t.transcribe(samples, language()).trim()
            if (text.isEmpty()) Result.Refused(Strings["voice_b_not_heard"]) else Result.Text(text)
        } catch (e: CancellationException) {
            throw e
        } catch (e: ModelException) {
            Result.Refused(VoiceInput.modelFailureMessage(e.failure))
        } catch (e: WhisperUnavailableException) {
            Result.Refused(Strings["desktop_voice_unavailable"])
        } catch (e: Exception) {
            Result.Refused(Strings["desktop_voice_failed"])
        }
    }

    companion object {
        /** 16kHz モノラルで約 1 時間分(float に直すとメモリが何倍にもなるので上限を置く)。 */
        const val MAX_BYTES: Long = 128L * 1024 * 1024

        /** 音声入力が on の時だけ本物の文字起こしを持つ(既定は off)。 */
        fun fromEnv(): AudioImport = AudioImport(if (VoiceFlag.enabled()) WhisperCppTranscriber() else null)
    }
}
