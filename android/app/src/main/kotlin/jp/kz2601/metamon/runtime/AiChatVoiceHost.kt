package jp.kz2601.metamon.runtime

import jp.kz2601.metamon.renderer.components.ai.AiChatVoice
import jp.kz2601.metamon.renderer.components.audio.AudioClaims
import jp.kz2601.metamon.renderer.components.audio.DictationException
import jp.kz2601.metamon.renderer.components.audio.DictationFailure
import jp.kz2601.metamon.renderer.components.audio.DictationHost
import jp.kz2601.metamon.renderer.components.audio.SpeechVoice
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.map
import kotlinx.coroutines.flow.stateIn
import jp.kz2601.metamon.renderer.components.ai.DictationReadiness as AiReadiness
import jp.kz2601.metamon.renderer.components.audio.DictationReadiness as AudioReadiness

/** aiChat の読み上げの声(テストで差し替える)。 */
interface AiChatSpeechEngine {
    /** 読み終わり・止まった・失敗を [utteranceId] で知らせる口。 */
    fun setOnFinished(listener: (utteranceId: String) -> Unit)

    fun speak(text: String, languageTag: String, utteranceId: String)

    fun stop()

    /** 声の仕組みを手放す(画面を離れた)。 */
    fun shutdown()
}

/** 音の取り合い(実機は AudioArbiter・テストは [AudioClaims])。 */
interface AiChatAudioGate {
    fun claim(owner: String, use: AudioClaims.Use, stop: () -> Unit): Boolean

    fun release(owner: String)
}

/**
 * aiChat の話して入力と読み上げ(#1168・iOS の `DictationHost` + `LeafSpeech`)。
 * - 録音は transcribeField と同じ認識器([dictation])。マイク FAB の音声入力中は始めない
 * - マイクの権限は [requestMic](権限の窓口)で聞く。断られたら [DictationFailure.MIC_DENIED]
 * - 録音を始めたら読み上げを止める。録音中は読まない
 * - 声は UI の言語、端末内の声だけ。画面を離れたら [release] で止めて手放す
 */
class AiChatVoiceHost(
    private val dictation: DictationHost,
    private val engine: AiChatSpeechEngine,
    private val gate: AiChatAudioGate,
    scope: CoroutineScope,
    private val uiLanguage: () -> String,
    private val requestMic: suspend () -> Boolean,
    /** 録音の前にほかの読み上げ(speakButton)も止める */
    private val stopOthers: () -> Unit = {},
) : AiChatVoice {
    private val speaking = MutableStateFlow<String?>(null)
    private var recording = false
    private var released = false

    init {
        engine.setOnFinished(::finished)
    }

    override val readiness: StateFlow<AiReadiness?> =
        dictation.readiness.map(::readinessOf).stateIn(scope, SharingStarted.Eagerly, readinessOf(dictation.readiness.value))

    override val voiceInputActive: StateFlow<Boolean> get() = dictation.isVoiceInputActive

    override val speakingOwner: StateFlow<String?> = speaking.asStateFlow()

    override suspend fun startDictation() {
        if (released || dictation.isVoiceInputActive.value) throw DictationException(DictationFailure.FAILED)
        if (!requestMic()) throw DictationException(DictationFailure.MIC_DENIED)
        // 権限を聞いている間にマイク FAB が始まった・画面を離れた
        if (released || dictation.isVoiceInputActive.value) throw DictationException(DictationFailure.FAILED)
        stopSpeaking()
        stopOthers()
        if (!gate.claim(MIC_OWNER, AudioClaims.Use.DICTATION) { cancelDictation() }) throw DictationException(DictationFailure.FAILED)
        recording = true
        try {
            dictation.startDictation()
        } catch (e: Throwable) {
            endRecording()
            throw e
        }
    }

    override suspend fun finishDictation(): String = try {
        dictation.finishDictation()
    } finally {
        endRecording()
    }

    override fun cancelDictation() {
        if (!recording) return
        endRecording()
        dictation.cancelDictation()
    }

    override fun prepareDictation() = dictation.prepareDictation()

    override fun speak(text: String, owner: String) {
        val t = text.trim()
        if (released || recording || t.isEmpty()) return
        if (!gate.claim(TTS_OWNER, AudioClaims.Use.SPEECH) { stopSpeaking() }) return
        speaking.value = owner
        engine.speak(t, SpeechVoice.languageTag(uiLanguage()), owner)
    }

    override fun stop(owner: String) {
        if (speaking.value != owner) return
        stopSpeaking()
    }

    /** 画面を離れた: 読み上げと録音を止めて、声の仕組みを手放す。 */
    fun release() {
        if (released) return
        stopSpeaking()
        cancelDictation()
        released = true
        engine.shutdown()
    }

    private fun stopSpeaking() {
        if (speaking.value == null) return
        speaking.value = null
        engine.stop()
        gate.release(TTS_OWNER)
    }

    private fun endRecording() {
        recording = false
        gate.release(MIC_OWNER)
    }

    private fun finished(utteranceId: String) {
        if (utteranceId != speaking.value) return
        speaking.value = null
        gate.release(TTS_OWNER)
    }

    companion object {
        const val MIC_OWNER = "ai-chat-dictation"
        const val TTS_OWNER = "ai-chat-speech"

        /** 端末内の認識が使えない時はマイクを出さない(null)。 */
        fun readinessOf(r: AudioReadiness): AiReadiness? = when (r) {
            AudioReadiness.READY -> AiReadiness.READY
            AudioReadiness.PREPARING -> AiReadiness.PREPARING
            AudioReadiness.NEEDS_MODEL -> AiReadiness.NEEDS_MODEL
            AudioReadiness.UNAVAILABLE -> null
        }
    }
}
