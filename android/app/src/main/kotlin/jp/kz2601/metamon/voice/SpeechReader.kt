package jp.kz2601.metamon.voice

import android.content.Context
import android.os.Handler
import android.os.Looper
import android.speech.tts.TextToSpeech
import android.speech.tts.UtteranceProgressListener
import jp.kz2601.metamon.renderer.components.audio.AudioArbiter
import jp.kz2601.metamon.renderer.components.audio.AudioClaims
import jp.kz2601.metamon.renderer.components.audio.LeafSpeech
import java.util.Locale

/** 声で読む先(テストで差し替える)。 */
interface SpeechEngine {
    /** [languageTag] は ja-JP / en-US */
    fun speak(text: String, languageTag: String)
    fun stop()
}

/**
 * 挨拶・AI の質問・完成の要約を読み上げる(iOS の `SpeechReader`・#43)。
 * - 既定 OFF(設定の「挨拶と質問を読み上げる」を ON にした人だけ)。[enabled] は毎回読むので切り替えがすぐ効く
 * - マイクを押したら [stopForRecording] で即止め、録音が終わる([recordingFinished])まで新しい読み上げを抑える
 * - 声の言語は UI ではなく本文で決める(かな・漢字があれば ja-JP)
 */
class SpeechReader(
    private val enabled: () -> Boolean,
    private val engine: SpeechEngine,
    /** 録音の前にほかの読み上げ(生成したアプリの speakButton・#1084)も止める */
    private val stopOthers: () -> Unit = {},
) {
    private var micBusy = false

    /** 読み上げる。前の読み上げは打ち切る。OFF・録音中・空文字なら false(何もしない) */
    fun speak(text: String): Boolean {
        val t = text.trim()
        if (!enabled() || t.isEmpty() || micBusy) return false
        engine.speak(t, language(t))
        return true
    }

    fun stop() = engine.stop()

    /** マイクを押した: 即止めて、録音が終わるまで読まない */
    fun stopForRecording() {
        micBusy = true
        stopOthers()
        engine.stop()
    }

    /** 録音が終わった(成功・失敗・取り消しとも) */
    fun recordingFinished() {
        micBusy = false
    }

    companion object {
        /** 本文にかな・漢字があれば ja-JP、無ければ en-US(iOS と同じ) */
        fun language(text: String): String =
            if (text.any { it.code in 0x3040..0x30FF || it.code in 0x4E00..0x9FFF }) "ja-JP" else "en-US"

        /** 何もしない読み上げ(モック・テストの既定) */
        val NONE = SpeechReader(enabled = { false }, engine = object : SpeechEngine {
            override fun speak(text: String, languageTag: String) = Unit
            override fun stop() = Unit
        })
    }
}

/** 設定の「読み上げ」(iOS / Mac の `metamon.speech.enabled` と同じキー)。既定 OFF。 */
class SpeechSettings(context: Context) {
    private val prefs = context.applicationContext.getSharedPreferences(PREFS, Context.MODE_PRIVATE)

    var enabled: Boolean
        get() = prefs.getBoolean(KEY_ENABLED, false)
        set(value) = prefs.edit().putBoolean(KEY_ENABLED, value).apply()

    private companion object {
        const val PREFS = "metamon.speech"
        const val KEY_ENABLED = "metamon.speech.enabled"
    }
}

/**
 * 端末の TextToSpeech で読む(#1084 の speakButton と同じ扱い: 音は [AudioArbiter] で取り合い、録音中は断られる・
 * ネットワークの声は使わない)。メインスレッドで使う。
 */
class AndroidSpeechEngine(context: Context) : SpeechEngine {
    companion object {
        /** 設定に従う読み上げ(録音の前に speakButton も止める) */
        fun reader(context: Context): SpeechReader {
            val settings = SpeechSettings(context)
            return SpeechReader(enabled = { settings.enabled }, engine = AndroidSpeechEngine(context), stopOthers = { LeafSpeech.stop() })
        }

        private const val OWNER = "voice-speech"
    }

    private val app = context.applicationContext
    private val main = Handler(Looper.getMainLooper())
    private var tts: TextToSpeech? = null
    private var ready = false
    private var pending: (() -> Unit)? = null

    override fun speak(text: String, languageTag: String) {
        if (!AudioArbiter.claim(app, OWNER, AudioClaims.Use.SPEECH) { stop() }) return
        val run = {
            tts?.let { engine ->
                val locale = Locale.forLanguageTag(languageTag)
                engine.language = locale
                engine.voices?.filter { it.locale.language == locale.language && !it.isNetworkConnectionRequired }
                    ?.maxByOrNull { it.quality }
                    ?.let { engine.voice = it }
                engine.speak(text, TextToSpeech.QUEUE_FLUSH, null, OWNER)
            }
            Unit
        }
        if (ready) run() else {
            pending = run
            ensureEngine()
        }
    }

    override fun stop() {
        pending = null
        tts?.stop()
        AudioArbiter.release(OWNER)
    }

    private fun ensureEngine() {
        if (tts != null) return
        tts = TextToSpeech(app) { status ->
            main.post {
                ready = status == TextToSpeech.SUCCESS
                if (ready) {
                    pending?.invoke()
                    pending = null
                } else {
                    // 読み上げの仕組みが無い端末。次にもう一度作る
                    tts?.shutdown()
                    tts = null
                    stop()
                }
            }
        }.also { engine ->
            engine.setOnUtteranceProgressListener(object : UtteranceProgressListener() {
                override fun onStart(utteranceId: String?) = Unit
                override fun onDone(utteranceId: String?) {
                    main.post { AudioArbiter.release(OWNER) }
                }

                @Deprecated("Deprecated in Java")
                override fun onError(utteranceId: String?) {
                    main.post { AudioArbiter.release(OWNER) }
                }
            })
        }
    }
}
