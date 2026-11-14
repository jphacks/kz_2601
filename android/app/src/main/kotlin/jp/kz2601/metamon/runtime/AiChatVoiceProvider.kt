package jp.kz2601.metamon.runtime

import android.content.Context
import android.os.Handler
import android.os.Looper
import android.speech.tts.TextToSpeech
import android.speech.tts.UtteranceProgressListener
import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.rememberUpdatedState
import androidx.compose.ui.platform.LocalConfiguration
import androidx.compose.ui.platform.LocalContext
import jp.kz2601.metamon.renderer.components.ai.LocalAiChatVoice
import jp.kz2601.metamon.renderer.components.audio.AudioArbiter
import jp.kz2601.metamon.renderer.components.audio.AudioClaims
import jp.kz2601.metamon.renderer.components.audio.DictationHost
import jp.kz2601.metamon.renderer.components.audio.LeafSpeech
import jp.kz2601.metamon.renderer.components.audio.rememberMicrophoneRequest
import kotlinx.coroutines.suspendCancellableCoroutine
import java.util.Locale
import kotlin.coroutines.resume

/**
 * 生成したアプリの aiChat に話して入力と読み上げを渡す(#1168)。認識器が無いランタイムでは渡さない(マイクとスピーカーを出さない)。
 * マイクの権限は権限の窓口(MicrophoneAccessProvider)で聞く。画面を離れたら読み上げを止め、TextToSpeech を手放す。
 */
@Composable
fun ProvideAiChatVoice(dictation: DictationHost?, content: @Composable () -> Unit) {
    if (dictation == null) {
        content()
        return
    }
    val context = LocalContext.current
    val language = LocalConfiguration.current.locales[0]?.language.orEmpty()
    val latestLanguage by rememberUpdatedState(language)
    val requestMic = rememberMicrophoneRequest()
    val latestRequest by rememberUpdatedState(requestMic)
    val scope = rememberCoroutineScope()
    val host = remember(dictation) {
        val app = context.applicationContext
        AiChatVoiceHost(
            dictation = dictation,
            engine = AndroidAiChatSpeechEngine(app),
            gate = ArbiterGate(app),
            scope = scope,
            uiLanguage = { latestLanguage },
            requestMic = { suspendCancellableCoroutine { cont -> latestRequest { granted -> if (cont.isActive) cont.resume(granted) } } },
            stopOthers = { LeafSpeech.stop() },
        )
    }
    DisposableEffect(host) { onDispose { host.release() } }
    CompositionLocalProvider(LocalAiChatVoice provides host, content = content)
}

private class ArbiterGate(private val context: Context) : AiChatAudioGate {
    override fun claim(owner: String, use: AudioClaims.Use, stop: () -> Unit): Boolean = AudioArbiter.claim(context, owner, use, stop)

    override fun release(owner: String) = AudioArbiter.release(owner)
}

/** 端末の TextToSpeech(ネットワークの声は使わない)。メインスレッドで使う。初めて読む時に作る。 */
private class AndroidAiChatSpeechEngine(private val app: Context) : AiChatSpeechEngine {
    private val main = Handler(Looper.getMainLooper())
    private var tts: TextToSpeech? = null
    private var ready = false
    private var pending: (() -> Unit)? = null
    private var pendingId: String? = null
    private var onFinished: (String) -> Unit = {}
    private var shut = false

    override val maxInputLength: Int get() = TextToSpeech.getMaxSpeechInputLength()

    override fun setOnFinished(listener: (utteranceId: String) -> Unit) {
        onFinished = listener
    }

    override fun speak(text: String, languageTag: String, utteranceId: String) {
        if (shut) return
        val run: () -> Unit = {
            val engine = tts
            if (engine == null) {
                onFinished(utteranceId)
            } else {
                // データが無い・対応していない言語は端末の既定の言語で読む
                val locale = AiChatTts.applyLanguage(Locale.forLanguageTag(languageTag), Locale.getDefault()) { engine.setLanguage(it) }
                if (locale != null) {
                    engine.voices?.filter { it.locale.language == locale.language && !it.isNetworkConnectionRequired }
                        ?.maxByOrNull { it.quality }
                        ?.let { engine.voice = it }
                }
                val clipped = AiChatTts.clip(text, TextToSpeech.getMaxSpeechInputLength())
                // 受け付けられなかった読み上げは終わりの知らせが来ないので、すぐ終わったことにする
                if (engine.speak(clipped, TextToSpeech.QUEUE_FLUSH, null, utteranceId) == TextToSpeech.ERROR) onFinished(utteranceId)
            }
            Unit
        }
        if (ready) run() else {
            pending = run
            pendingId = utteranceId
            ensureEngine()
        }
    }

    override fun stop() {
        pending = null
        tts?.stop()
    }

    override fun shutdown() {
        shut = true
        pending = null
        tts?.stop()
        tts?.shutdown()
        tts = null
        ready = false
    }

    private fun ensureEngine() {
        if (tts != null) return
        tts = TextToSpeech(app) { status ->
            main.post {
                if (shut) return@post
                ready = status == TextToSpeech.SUCCESS
                if (ready) {
                    pending?.invoke()
                } else {
                    // 読み上げの仕組みが無い端末。次に読む時にもう一度作る
                    tts?.shutdown()
                    tts = null
                    pendingId?.takeIf { pending != null }?.let(onFinished)
                }
                pending = null
                pendingId = null
            }
        }.also { engine ->
            engine.setOnUtteranceProgressListener(object : UtteranceProgressListener() {
                override fun onStart(utteranceId: String?) = Unit
                override fun onDone(utteranceId: String?) = finished(utteranceId)

                @Deprecated("Deprecated in Java")
                override fun onError(utteranceId: String?) = finished(utteranceId)
                override fun onStop(utteranceId: String?, interrupted: Boolean) = finished(utteranceId)
            })
        }
    }

    private fun finished(utteranceId: String?) {
        if (utteranceId == null) return
        main.post { onFinished(utteranceId) }
    }
}
