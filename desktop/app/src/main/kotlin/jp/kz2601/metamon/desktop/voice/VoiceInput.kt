package jp.kz2601.metamon.desktop.voice

import jp.kz2601.metamon.desktop.i18n.Lang
import jp.kz2601.metamon.desktop.i18n.Strings
import jp.kz2601.metamon.whisper.ModelException
import jp.kz2601.metamon.whisper.ModelFailure
import jp.kz2601.metamon.whisper.WhisperAudio
import jp.kz2601.metamon.whisper.WhisperUnavailableException
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.launch
import kotlin.coroutines.cancellation.CancellationException

/** 音声入力を使うか(既定は使わない・#1751)。`TAILORIT_VOICE=1`(か `-Dtailorit.voice=true`)で使う。 */
object VoiceFlag {
    fun enabled(
        props: (String) -> String? = System::getProperty,
        env: (String) -> String? = System::getenv,
    ): Boolean = (props("tailorit.voice") ?: env("TAILORIT_VOICE"))?.trim()?.lowercase() in setOf("1", "true", "on", "yes")
}

/**
 * openVoice の録音 → 文字起こし(端末の中だけ)。押すたびに 待ち → 録音 → 文字にする → 待ち と進む。
 * - 1 回目: マイクを開いて録り始める
 * - 2 回目: 止めて、裏で文字にし、終わったら [post] に結果の文言を渡す(画面のスレッドではない)
 * - 文字にしている間に押しても何もしない
 * 文字にした後の「声でアプリを変える」チャットの画面はまだ無い(結果は知らせの文言で出す)。
 */
class VoiceInput(
    private val mic: Microphone,
    private val transcriber: Transcriber,
    private val scope: CoroutineScope = CoroutineScope(SupervisorJob() + Dispatchers.IO),
    private val language: () -> String = { if (Strings.lang == Lang.EN) "en" else "ja" },
    /** 入力装置の名前(null は OS の既定)。選ぶ画面は後で足す。 */
    var device: String? = null,
) {
    enum class State { IDLE, RECORDING, TRANSCRIBING }

    @Volatile
    var state: State = State.IDLE
        private set

    private var recording: Microphone.Recording? = null

    /** 最後に始めた文字起こし(テストが終わりを待つ)。 */
    internal var job: Job? = null
        private set

    /** 押した時。すぐ出す文言を返し、文字にし終えた後の文言は [post] に渡す。 */
    @Synchronized
    fun toggle(post: (String) -> Unit): String = when (state) {
        State.IDLE -> start()
        State.RECORDING -> stopAndTranscribe(post)
        State.TRANSCRIBING -> Strings["desktop_voice_busy"]
    }

    /** 録音をやめて捨てる(ウィンドウを閉じる時など)。 */
    @Synchronized
    fun cancel() {
        recording?.cancel()
        recording = null
        job?.cancel()
        state = State.IDLE
    }

    private fun start(): String {
        if (!transcriber.isAvailable) return Strings["desktop_voice_unavailable"]
        recording = try {
            mic.start(device)
        } catch (e: MicException) {
            return Strings[if (e.reason == MicException.Reason.BUSY) "desktop_voice_mic_busy" else "desktop_voice_no_mic"]
        }
        state = State.RECORDING
        return Strings["desktop_voice_recording"]
    }

    private fun stopAndTranscribe(post: (String) -> Unit): String {
        val samples = recording?.stop() ?: FloatArray(0)
        recording = null
        silenceMessage(samples)?.let {
            state = State.IDLE
            return it
        }
        state = State.TRANSCRIBING
        val download = transcriber.pendingDownloadMegabytes
        val lang = language()
        job = scope.launch {
            val msg = try {
                val text = transcriber.transcribe(samples, lang).trim()
                if (text.isEmpty()) Strings["voice_b_not_heard"] else Strings.format("desktop_voice_heard", text)
            } catch (e: CancellationException) {
                throw e
            } catch (e: ModelException) {
                modelFailureMessage(e.failure)
            } catch (e: WhisperUnavailableException) {
                Strings["desktop_voice_unavailable"]
            } catch (e: Exception) {
                Strings["desktop_voice_failed"]
            } finally {
                state = State.IDLE
            }
            post(msg)
        }
        return if (download != null) Strings.format("desktop_voice_downloading", download) else Strings["desktop_voice_transcribing"]
    }

    companion object {
        /**
         * 録った音が文字にする価値の無い時の文言。null なら文字にする。
         * 全部 0 はマイクを OS が止めている(Windows のプライバシーの設定・Linux の PulseAudio / PipeWire でミュート)。
         */
        fun silenceMessage(samples: FloatArray): String? = when {
            samples.size < WhisperAudio.SAMPLE_RATE * WhisperAudio.MINIMUM_SECONDS -> Strings["desktop_voice_too_short"]
            WhisperAudio.peak(samples) == 0f -> Strings["desktop_voice_mic_denied"]
            !WhisperAudio.hasVoice(samples) -> Strings["voice_b_silent"]
            else -> null
        }

        fun modelFailureMessage(f: ModelFailure): String = when (f) {
            ModelFailure.Network -> Strings["desktop_voice_model_network"]
            ModelFailure.Checksum -> Strings["desktop_voice_model_broken"]
            ModelFailure.Storage -> Strings["desktop_voice_model_storage"]
            is ModelFailure.NotEnoughStorage -> Strings.format("desktop_voice_model_space", (f.requiredBytes + 999_999) / 1_000_000)
        }

        /** フラグが立っている時だけ作る(既定は null = openVoice は「まだ使えない」)。 */
        fun fromEnv(): VoiceInput? = if (VoiceFlag.enabled()) VoiceInput(JavaSoundMicrophone(), WhisperCppTranscriber()) else null
    }
}
