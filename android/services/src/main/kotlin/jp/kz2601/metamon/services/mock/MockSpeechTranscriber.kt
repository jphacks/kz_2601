package jp.kz2601.metamon.services.mock

import jp.kz2601.metamon.services.SpeechTranscriber
import jp.kz2601.metamon.services.TranscriberFailure
import jp.kz2601.metamon.services.TranscriberStatus
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow

/**
 * 音声入力のモック(iOS の `MockTranscriber`)。エミュレータ・テスト用。
 * [results] を順に返す(尽きたら [fallbackText])。[startFailure] があれば録音の開始で投げる。
 */
class MockSpeechTranscriber(
    status: TranscriberStatus = TranscriberStatus.Ready,
    results: List<Result<String>> = emptyList(),
    private val fallbackText: String = "家計簿アプリが欲しい",
) : SpeechTranscriber {
    private val _status = MutableStateFlow(status)
    override val status: StateFlow<TranscriberStatus> = _status

    private val _partial = MutableStateFlow("")
    override val partial: StateFlow<String> = _partial

    private val _listening = MutableStateFlow(false)
    override val listening: StateFlow<Boolean> = _listening

    private val queue = ArrayDeque(results)
    var startFailure: TranscriberFailure? = null
    var startCount = 0
        private set
    var cancelCount = 0
        private set

    fun setStatus(value: TranscriberStatus) {
        _status.value = value
    }

    /** 途中の聞き取りを流す(テスト用) */
    fun emitPartial(text: String) {
        _partial.value = text
    }

    /** 認識器が無音で自分から止まったことにする(テスト用) */
    fun endBySelf() {
        _listening.value = false
    }

    override suspend fun startRecording() {
        startFailure?.let { throw it }
        if (_status.value is TranscriberStatus.Unavailable) throw TranscriberFailure.OnDeviceRecognitionUnavailable
        startCount++
        _partial.value = ""
        _listening.value = true
    }

    override suspend fun stopAndTranscribe(): String {
        _listening.value = false
        val next = queue.removeFirstOrNull() ?: Result.success(fallbackText)
        val text = next.getOrThrow().trim()
        if (text.isEmpty()) throw TranscriberFailure.NotHeard
        return text
    }

    override fun cancelRecording() {
        if (_listening.value) cancelCount++
        _listening.value = false
    }
}
