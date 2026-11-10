package jp.kz2601.metamon.services.speech

/**
 * `SpeechRecognizer.ERROR_*` → [TranscriberFailure]。端末に依らないので JVM でテストする。
 * 値は android.speech.SpeechRecognizer と同じ(テストで android.jar を読まないよう写してある)。
 */
object SpeechErrorMapping {
    const val ERROR_NETWORK_TIMEOUT = 1
    const val ERROR_NETWORK = 2
    const val ERROR_AUDIO = 3
    const val ERROR_SERVER = 4
    const val ERROR_CLIENT = 5
    const val ERROR_SPEECH_TIMEOUT = 6
    const val ERROR_NO_MATCH = 7
    const val ERROR_RECOGNIZER_BUSY = 8
    const val ERROR_INSUFFICIENT_PERMISSIONS = 9
    const val ERROR_TOO_MANY_REQUESTS = 10
    const val ERROR_SERVER_DISCONNECTED = 11
    const val ERROR_LANGUAGE_NOT_SUPPORTED = 12
    const val ERROR_LANGUAGE_UNAVAILABLE = 13
    const val ERROR_CANNOT_CHECK_SUPPORT = 14

    /** これ未満の録音は短すぎ(iOS の `minimumSeconds` と同じ) */
    const val MINIMUM_SECONDS = 0.5

    /** `onRmsChanged` の dB がこれを超えたら声が届いた扱い(端末の目安は -2〜10) */
    const val SPEECH_RMS_DB = 2f

    /**
     * @param recordedSeconds 録音の始めから止めた(止まった)までの秒数
     * @param heardSpeech `onBeginningOfSpeech` が来た・音量が [SPEECH_RMS_DB] を超えた
     */
    fun failure(errorCode: Int, recordedSeconds: Double, heardSpeech: Boolean): TranscriberFailure = when (errorCode) {
        ERROR_INSUFFICIENT_PERMISSIONS -> TranscriberFailure.MicrophoneDenied
        ERROR_AUDIO, ERROR_RECOGNIZER_BUSY -> TranscriberFailure.NoInputAvailable
        ERROR_LANGUAGE_NOT_SUPPORTED -> TranscriberFailure.OnDeviceRecognitionUnavailable
        ERROR_LANGUAGE_UNAVAILABLE, ERROR_CANNOT_CHECK_SUPPORT -> TranscriberFailure.NotReady
        // 端末内の認識はネットワークを使わない。来たら端末内で完結できない = 使えない扱い(サーバーへは送らない)
        ERROR_NETWORK_TIMEOUT, ERROR_NETWORK, ERROR_SERVER, ERROR_SERVER_DISCONNECTED, ERROR_TOO_MANY_REQUESTS ->
            TranscriberFailure.OnDeviceRecognitionUnavailable
        // 止めるのが早すぎると CLIENT、話さないと SPEECH_TIMEOUT、話しても文字にならないと NO_MATCH
        ERROR_CLIENT, ERROR_SPEECH_TIMEOUT, ERROR_NO_MATCH -> emptyResult(recordedSeconds, heardSpeech)
        else -> TranscriberFailure.Other(errorCode)
    }

    /** 文字が出なかった時の理由(iOS: 短すぎ → 無音 → 聞き取れない の順に見る)。 */
    fun emptyResult(recordedSeconds: Double, heardSpeech: Boolean): TranscriberFailure = when {
        recordedSeconds < MINIMUM_SECONDS -> TranscriberFailure.TooShort(recordedSeconds)
        !heardSpeech -> TranscriberFailure.Silent(recordedSeconds)
        else -> TranscriberFailure.NotHeard
    }

    /** `checkRecognitionSupport` の言語の一覧("ja-JP" など)に含まれるか。言語だけ合えばよい("ja" と "ja-JP")。 */
    fun containsLanguage(tags: List<String>, tag: String): Boolean {
        val lang = tag.substringBefore('-').lowercase()
        return tags.any { it.equals(tag, ignoreCase = true) || it.substringBefore('-').substringBefore('_').lowercase() == lang }
    }
}
