package jp.kz2601.metamon.runtime

import jp.kz2601.metamon.renderer.components.audio.DictationException
import jp.kz2601.metamon.renderer.components.audio.DictationFailure
import jp.kz2601.metamon.renderer.components.audio.DictationHost
import jp.kz2601.metamon.renderer.components.audio.DictationReadiness
import jp.kz2601.metamon.services.SpeechTranscriber
import jp.kz2601.metamon.services.TranscriberFailure
import jp.kz2601.metamon.services.TranscriberStatus
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.map
import kotlinx.coroutines.flow.stateIn
import kotlinx.coroutines.launch

/**
 * transcribeField の話して入力(#864・iOS の `AppRuntime+Dictation`)。本体の音声入力と同じ認識器([SpeechTranscriber])を使い、
 * 声は端末の外へ出さない。マイク FAB の音声入力のシートが出ている間は [isVoiceInputActive] が true(部品は録音を捨てる)。
 */
class TranscriberDictationHost(
    private val transcriber: SpeechTranscriber,
    private val scope: CoroutineScope,
) : DictationHost {
    private val voiceActive = MutableStateFlow(false)

    override val readiness: StateFlow<DictationReadiness> =
        transcriber.status.map(::readinessOf).stateIn(scope, SharingStarted.Eagerly, readinessOf(transcriber.status.value))

    override val isVoiceInputActive: StateFlow<Boolean> = voiceActive.asStateFlow()

    override val listening: StateFlow<Boolean> get() = transcriber.listening

    /** 音声入力のシートの開け閉め(MetamonApp が VoiceUiState.voiceOpen を渡す)。 */
    fun setVoiceInputActive(active: Boolean) {
        voiceActive.value = active
    }

    override fun prepareDictation() {
        if (transcriber.status.value == TranscriberStatus.NotDownloaded) transcriber.downloadModel() else scope.launch { transcriber.prepare() }
    }

    override suspend fun startDictation() {
        if (voiceActive.value) throw DictationException(DictationFailure.FAILED)
        try {
            transcriber.startRecording()
        } catch (f: TranscriberFailure) {
            throw DictationException(failureOf(f))
        }
    }

    override suspend fun finishDictation(): String = try {
        transcriber.stopAndTranscribe()
    } catch (f: TranscriberFailure) {
        throw DictationException(failureOf(f))
    }

    override fun cancelDictation() = transcriber.cancelRecording()

    companion object {
        fun readinessOf(status: TranscriberStatus): DictationReadiness = when (status) {
            TranscriberStatus.Ready, TranscriberStatus.Recognizing -> DictationReadiness.READY
            TranscriberStatus.Downloading -> DictationReadiness.PREPARING
            TranscriberStatus.NotDownloaded -> DictationReadiness.NEEDS_MODEL
            is TranscriberStatus.Unavailable -> DictationReadiness.UNAVAILABLE
        }

        /** 認識器の失敗 → 部品の文言の種類(本体の `VoiceErrorCopy` と同じ分け方)。 */
        fun failureOf(f: TranscriberFailure): DictationFailure = when (f) {
            TranscriberFailure.MicrophoneDenied -> DictationFailure.MIC_DENIED
            TranscriberFailure.NotReady -> DictationFailure.NOT_READY
            is TranscriberFailure.TooShort, is TranscriberFailure.Silent, TranscriberFailure.NotHeard -> DictationFailure.NOT_HEARD
            TranscriberFailure.OnDeviceRecognitionUnavailable -> DictationFailure.UNAVAILABLE
            TranscriberFailure.NoInputAvailable, is TranscriberFailure.Other -> DictationFailure.FAILED
        }
    }
}
