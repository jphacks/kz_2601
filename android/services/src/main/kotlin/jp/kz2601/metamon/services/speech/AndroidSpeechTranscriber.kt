package jp.kz2601.metamon.services.speech

import android.Manifest
import android.annotation.TargetApi
import android.content.Context
import android.content.Intent
import android.content.pm.PackageManager
import android.os.Build
import android.os.Bundle
import android.os.SystemClock
import android.speech.RecognitionListener
import android.speech.RecognitionSupport
import android.speech.RecognitionSupportCallback
import android.speech.RecognizerIntent
import android.speech.SpeechRecognizer
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.suspendCancellableCoroutine
import kotlinx.coroutines.withContext
import kotlinx.coroutines.withTimeoutOrNull
import kotlin.coroutines.resume

/**
 * 端末内の `SpeechRecognizer`(#354 §3・android-050)。
 * - `createOnDeviceSpeechRecognizer` だけを使う。端末内が無い端末は [TranscriberStatus.Unavailable](サーバーの認識には落とさない)
 * - 言語は UI の言語(ja-JP / en-US)。途中の聞き取りを [partial] に流す
 * - iOS と同じくタップで始めてタップで止める。無音で認識器が自分から止まった時は [listening] が false になる
 * SpeechRecognizer はメインスレッドから触る決まりなので、呼び出しは全部 Main に移す。
 */
class AndroidSpeechTranscriber(
    context: Context,
    private val language: () -> String,
    private val log: (String) -> Unit = {},
) : SpeechTranscriber {
    private val context = context.applicationContext

    private val _status = MutableStateFlow<TranscriberStatus>(
        if (SpeechRecognizer.isOnDeviceRecognitionAvailable(this.context)) {
            TranscriberStatus.Ready
        } else {
            TranscriberStatus.Unavailable("onDeviceUnavailable")
        },
    )
    override val status: StateFlow<TranscriberStatus> = _status

    private val _partial = MutableStateFlow("")
    override val partial: StateFlow<String> = _partial

    private val _listening = MutableStateFlow(false)
    override val listening: StateFlow<Boolean> = _listening

    private var recognizer: SpeechRecognizer? = null
    private var result: CompletableDeferred<Outcome>? = null
    private var startedAt = 0L
    private var endedAt = 0L
    private var heardSpeech = false

    private sealed interface Outcome {
        data class Text(val value: String) : Outcome
        data class Error(val code: Int) : Outcome
    }

    private fun intent(): Intent = Intent(RecognizerIntent.ACTION_RECOGNIZE_SPEECH).apply {
        putExtra(RecognizerIntent.EXTRA_LANGUAGE_MODEL, RecognizerIntent.LANGUAGE_MODEL_FREE_FORM)
        putExtra(RecognizerIntent.EXTRA_LANGUAGE, speechLanguageTag(language()))
        putExtra(RecognizerIntent.EXTRA_PARTIAL_RESULTS, true)
        putExtra(RecognizerIntent.EXTRA_PREFER_OFFLINE, true)
        // iOS はタップで止める。話の切れ目ですぐ止まらないよう、無音の待ちを少し長くする(端末によっては無視される)
        putExtra(RecognizerIntent.EXTRA_SPEECH_INPUT_COMPLETE_SILENCE_LENGTH_MILLIS, 2_500L)
        putExtra(RecognizerIntent.EXTRA_SPEECH_INPUT_POSSIBLY_COMPLETE_SILENCE_LENGTH_MILLIS, 2_000L)
    }

    override suspend fun prepare() {
        if (_status.value is TranscriberStatus.Unavailable || Build.VERSION.SDK_INT < Build.VERSION_CODES.TIRAMISU) return
        withContext(Dispatchers.Main.immediate) { _status.value = checkSupport() }
    }

    @TargetApi(Build.VERSION_CODES.TIRAMISU)
    private suspend fun checkSupport(): TranscriberStatus {
        val tag = speechLanguageTag(language())
        val probe = SpeechRecognizer.createOnDeviceSpeechRecognizer(context)
        return try {
            suspendCancellableCoroutine { cont ->
                probe.checkRecognitionSupport(
                    intent(),
                    context.mainExecutor,
                    object : RecognitionSupportCallback {
                        override fun onSupportResult(support: RecognitionSupport) {
                            val status = when {
                                SpeechErrorMapping.containsLanguage(support.installedOnDeviceLanguages, tag) -> TranscriberStatus.Ready
                                SpeechErrorMapping.containsLanguage(support.pendingOnDeviceLanguages, tag) -> TranscriberStatus.Downloading
                                SpeechErrorMapping.containsLanguage(support.supportedOnDeviceLanguages, tag) -> TranscriberStatus.NotDownloaded
                                else -> TranscriberStatus.Unavailable("languageNotSupported")
                            }
                            log("speech: support $tag → $status")
                            if (cont.isActive) cont.resume(status)
                        }

                        override fun onError(error: Int) {
                            // 確かめられないだけなら録音を試す(録音で ERROR_LANGUAGE_* が出たらそこで理由を出す)
                            log("speech: checkRecognitionSupport error=$error")
                            if (cont.isActive) cont.resume(TranscriberStatus.Ready)
                        }
                    },
                )
            }
        } finally {
            probe.destroy()
        }
    }

    override fun downloadModel() {
        if (Build.VERSION.SDK_INT < Build.VERSION_CODES.TIRAMISU) return
        if (_status.value !is TranscriberStatus.NotDownloaded) return
        val r = SpeechRecognizer.createOnDeviceSpeechRecognizer(context)
        try {
            r.triggerModelDownload(intent())
            _status.value = TranscriberStatus.Downloading
        } finally {
            r.destroy()
        }
    }

    override suspend fun startRecording() = withContext(Dispatchers.Main.immediate) {
        if (recognizer != null) return@withContext
        when (_status.value) {
            is TranscriberStatus.Unavailable -> throw TranscriberFailure.OnDeviceRecognitionUnavailable
            TranscriberStatus.NotDownloaded, TranscriberStatus.Downloading -> throw TranscriberFailure.NotReady
            else -> Unit
        }
        if (context.checkSelfPermission(Manifest.permission.RECORD_AUDIO) != PackageManager.PERMISSION_GRANTED) {
            throw TranscriberFailure.MicrophoneDenied
        }
        val r = SpeechRecognizer.createOnDeviceSpeechRecognizer(context)
        val deferred = CompletableDeferred<Outcome>()
        recognizer = r
        result = deferred
        heardSpeech = false
        startedAt = SystemClock.elapsedRealtime()
        endedAt = 0L
        _partial.value = ""
        r.setRecognitionListener(Listener(deferred))
        r.startListening(intent())
        _listening.value = true
        _status.value = TranscriberStatus.Recognizing
        log("speech: start ${speechLanguageTag(language())}")
    }

    override suspend fun stopAndTranscribe(): String = withContext(Dispatchers.Main.immediate) {
        val r = recognizer ?: throw TranscriberFailure.NotHeard
        val deferred = result ?: throw TranscriberFailure.NotHeard
        if (_listening.value) {
            endedAt = SystemClock.elapsedRealtime()
            r.stopListening()
        }
        val outcome = withTimeoutOrNull(FINAL_RESULT_TIMEOUT_MS) { deferred.await() }
        val seconds = ((if (endedAt > 0) endedAt else SystemClock.elapsedRealtime()) - startedAt) / 1000.0
        val partial = _partial.value.trim()
        teardown()
        log("speech: stop ${"%.1f".format(seconds)}s heard=$heardSpeech outcome=$outcome")
        when (outcome) {
            is Outcome.Text -> outcome.value.trim().ifEmpty { partial }.ifEmpty {
                throw SpeechErrorMapping.emptyResult(seconds, heardSpeech)
            }
            is Outcome.Error -> partial.ifEmpty { throw SpeechErrorMapping.failure(outcome.code, seconds, heardSpeech) }
            // 止めてから結果が来ない: 途中の聞き取りを使う(iOS の finalResultTimeout と同じ)
            null -> partial.ifEmpty { throw SpeechErrorMapping.emptyResult(seconds, heardSpeech) }
        }
    }

    override fun cancelRecording() {
        val r = recognizer ?: return
        r.cancel()
        teardown()
        log("speech: cancelled")
    }

    private fun teardown() {
        recognizer?.destroy()
        recognizer = null
        result = null
        _listening.value = false
        if (_status.value == TranscriberStatus.Recognizing) _status.value = TranscriberStatus.Ready
    }

    private inner class Listener(private val deferred: CompletableDeferred<Outcome>) : RecognitionListener {
        private fun finish(outcome: Outcome) {
            if (endedAt == 0L) endedAt = SystemClock.elapsedRealtime()
            deferred.complete(outcome)
            _listening.value = false
        }

        override fun onReadyForSpeech(params: Bundle?) = Unit
        override fun onBeginningOfSpeech() {
            heardSpeech = true
        }

        override fun onRmsChanged(rmsdB: Float) {
            if (rmsdB > SpeechErrorMapping.SPEECH_RMS_DB) heardSpeech = true
        }

        override fun onBufferReceived(buffer: ByteArray?) = Unit
        override fun onEndOfSpeech() = Unit
        override fun onError(error: Int) {
            log("speech: onError $error")
            finish(Outcome.Error(error))
        }

        override fun onResults(results: Bundle?) {
            finish(Outcome.Text(results.bestText()))
        }

        override fun onPartialResults(partialResults: Bundle?) {
            val text = partialResults.bestText()
            if (text.isNotBlank()) _partial.value = text
        }

        override fun onEvent(eventType: Int, params: Bundle?) = Unit

        private fun Bundle?.bestText(): String =
            this?.getStringArrayList(SpeechRecognizer.RESULTS_RECOGNITION)?.firstOrNull().orEmpty()
    }

    private companion object {
        const val FINAL_RESULT_TIMEOUT_MS = 8_000L
    }
}
