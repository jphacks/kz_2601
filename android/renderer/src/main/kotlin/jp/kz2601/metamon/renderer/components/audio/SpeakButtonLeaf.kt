package jp.kz2601.metamon.renderer.components.audio

import android.content.Context
import android.speech.tts.TextToSpeech
import android.speech.tts.UtteranceProgressListener
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalConfiguration
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.semantics.stateDescription
import androidx.compose.ui.semantics.testTag
import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.renderer.R
import jp.kz2601.metamon.renderer.components.actions.ActionButton
import jp.kz2601.metamon.renderer.components.actions.ActionButtonKind
import jp.kz2601.metamon.renderer.components.actions.QrContent
import jp.kz2601.metamon.renderer.components.actions.rememberEvaluatorFactory
import jp.kz2601.metamon.renderer.registry.RenderContext
import java.util.Locale
import java.util.UUID

/**
 * `speakButton`(iOS の `SpeakButtonLeaf`): text / field / value(式)の文を端末の声で読み上げる(もう一度押すと止まる)。声は UI の言語。
 * 中身の作り方は qrCode と同じ([QrContent.resolve])。画面を離れたら止める。
 */
@Composable
fun SpeakButtonLeaf(c: Component, ctx: RenderContext, modifier: Modifier = Modifier) {
    val context = LocalContext.current
    val language = LocalConfiguration.current.locales[0]?.language.orEmpty()
    val owner = remember { "speak-" + UUID.randomUUID() }
    val text = QrContent.resolve(c, ctx.form, rememberEvaluatorFactory(ctx.runtime))
    val speaking = LeafSpeech.speakingOwner == owner
    val empty = text.isBlank()
    val title = c.label?.takeIf { it.isNotEmpty() } ?: stringResource(R.string.renderer_audio_speak)
    val hint = if (empty) stringResource(R.string.renderer_audio_speak_empty) else ""
    DisposableEffect(owner) { onDispose { LeafSpeech.stop(owner) } }
    ActionButton(
        if (speaking) stringResource(R.string.renderer_audio_stop_speaking) else title,
        ActionButtonKind.SECONDARY,
        onClick = {
            if (speaking) LeafSpeech.stop(owner) else LeafSpeech.speak(context.applicationContext, text, language, owner)
        },
        modifier = modifier.semantics {
            testTag = "speakButton"
            if (hint.isNotEmpty()) stateDescription = hint
        },
        icon = if (speaking) "xmark" else "waveform",
        enabled = !empty || speaking,
    )
}

/**
 * `speakButton` の読み上げ(アプリ全体で 1 本・iOS の `LeafSpeech`)。声は UI の言語、端末内で作れる声を優先する。
 * 録音・話して入力の間は読まない([AudioArbiter] が断る)。メインスレッドで使う。
 */
object LeafSpeech {
    private var tts: TextToSpeech? = null
    private var ready = false
    private var pending: (() -> Unit)? = null

    /** 読み上げ中の部品。null = 読んでいない。 */
    var speakingOwner by mutableStateOf<String?>(null)
        private set

    /** 読み上げる。前の読み上げは打ち切る。録音中・空文字なら false。 */
    fun speak(context: Context, text: String, uiLanguage: String, owner: String): Boolean {
        val t = text.trim()
        if (t.isEmpty()) return false
        if (!AudioArbiter.claim(context, OWNER, AudioClaims.Use.SPEECH) { stop() }) return false
        speakingOwner = owner
        val run: () -> Unit = {
            tts?.let { engine ->
                val locale = Locale.forLanguageTag(SpeechVoice.languageTag(uiLanguage))
                engine.language = locale
                // ネットワークの声を避ける(読み上げる文を端末の外へ出さない)
                engine.voices?.filter { it.locale.language == locale.language && !it.isNetworkConnectionRequired }
                    ?.maxByOrNull { it.quality }
                    ?.let { engine.voice = it }
                engine.speak(t, TextToSpeech.QUEUE_FLUSH, null, owner)
            }
            Unit
        }
        if (ready) run() else {
            pending = run
            ensureEngine(context)
        }
        return true
    }

    /** 止める。[owner] を渡したら、その部品が読んでいる時だけ。 */
    fun stop(owner: String? = null) {
        if (owner != null && owner != speakingOwner) return
        speakingOwner = null
        pending = null
        tts?.stop()
        AudioArbiter.release(OWNER)
    }

    private fun ensureEngine(context: Context) {
        if (tts != null) return
        val main = android.os.Handler(android.os.Looper.getMainLooper())
        tts = TextToSpeech(context.applicationContext) { status ->
            main.post {
                ready = status == TextToSpeech.SUCCESS
                if (ready) {
                    pending?.invoke()
                    pending = null
                } else {
                    // 読み上げの仕組みが無い端末。次に押した時にもう一度作る
                    tts?.shutdown()
                    tts = null
                    stop()
                }
            }
        }.also { engine ->
            engine.setOnUtteranceProgressListener(object : UtteranceProgressListener() {
                override fun onStart(utteranceId: String?) = Unit
                override fun onDone(utteranceId: String?) = main.post { finished(utteranceId) }.let { }
                @Deprecated("Deprecated in Java")
                override fun onError(utteranceId: String?) = main.post { finished(utteranceId) }.let { }
                override fun onStop(utteranceId: String?, interrupted: Boolean) = main.post { finished(utteranceId) }.let { }
            })
        }
    }

    private fun finished(utteranceId: String?) {
        if (utteranceId != null && utteranceId == speakingOwner) {
            speakingOwner = null
            AudioArbiter.release(OWNER)
        }
    }

    private const val OWNER = "leaf-speech"
}
