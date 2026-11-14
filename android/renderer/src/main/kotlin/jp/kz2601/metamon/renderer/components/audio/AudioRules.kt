package jp.kz2601.metamon.renderer.components.audio

import jp.kz2601.metamon.dsl.records.JSONValue
import java.util.Locale
import java.util.UUID
import kotlin.math.ceil
import kotlin.math.log10

// 音の部品(#864)の決まりごと。Android の部品に触らない(JVM テスト)。iOS の LocalAudio.swift・LeafSpeech・DictationText と同じ。

/**
 * audio field の値(iOS の `AudioValue`)。録音本体は録音した端末のアプリ領域にだけ置き、
 * レコードには `"audio:<id>|<秒>"` だけを入れる(サーバーへは上げない)。
 */
data class AudioValue(val id: String, val seconds: Int) {
    val encoded: String get() = "$PREFIX$id|${seconds.coerceAtLeast(0)}"

    companion object {
        const val PREFIX = "audio:"

        fun parse(value: JSONValue?): AudioValue? = (value as? JSONValue.Str)?.value?.let(::parse)

        fun parse(s: String): AudioValue? {
            if (!s.startsWith(PREFIX)) return null
            val parts = s.substring(PREFIX.length).split('|', limit = 2)
            val id = parts[0]
            if (!isValidId(id)) return null
            return AudioValue(id, (parts.getOrNull(1)?.toIntOrNull() ?: 0).coerceAtLeast(0))
        }

        /** id は英数・-・_ だけ・80 文字まで(パスを組み立てるので `..` や `/` を通さない)。 */
        fun isValidId(id: String): Boolean =
            id.isNotEmpty() && id.length <= 80 && id.all { (it.code < 0x80 && it.isLetterOrDigit()) || it == '-' || it == '_' }

        fun newId(): String = UUID.randomUUID().toString().uppercase(Locale.ROOT)

        /** レコードの値の中にある端末内録音の id(削除時に一緒にファイルを消す)。 */
        fun localIds(data: Map<String, JSONValue>): Set<String> = data.values.mapNotNull(::parse).map { it.id }.toSet()
    }
}

/** 録音の上限・表示(iOS の `AudioRecorderPolicy`)。 */
object AudioRecorderPolicy {
    /** maxDuration の既定(10 分)と上限(2 時間・backend `dsl.AudioMaxDuration` と同じ)。 */
    const val DEFAULT_MAX_DURATION = 600
    const val MAX_ALLOWED_DURATION = 7200

    // 録音の形式(iOS と同じ AAC・44.1kHz・モノラル・64kbps。Android は MPEG-4 の入れ物で .m4a)
    const val SAMPLE_RATE = 44_100
    const val CHANNELS = 1
    const val BIT_RATE = 64_000

    fun maxDuration(raw: Int?): Int = if (raw == null || raw <= 0) DEFAULT_MAX_DURATION else minOf(raw, MAX_ALLOWED_DURATION)

    /** 1 時間未満は「m:ss」、以上は「h:mm:ss」。 */
    fun elapsedText(seconds: Double): String {
        val t = if (seconds.isFinite()) seconds.toInt().coerceAtLeast(0) else 0
        val h = t / 3600
        val m = (t % 3600) / 60
        val s = t % 60
        return if (h > 0) String.format(Locale.ROOT, "%d:%02d:%02d", h, m, s) else String.format(Locale.ROOT, "%d:%02d", m, s)
    }

    /** レコードに残す長さ(秒)。端数は切り上げ。 */
    fun storedSeconds(duration: Double): Int = if (!duration.isFinite() || duration <= 0) 0 else ceil(duration).toInt()

    /** 音量メーター: dBFS(-50 dB 以下は 0)を 0...1 に(iOS と同じ)。 */
    const val METER_FLOOR = -50.0

    fun meterLevel(power: Double): Double = if (!power.isFinite()) 0.0 else ((power - METER_FLOOR) / -METER_FLOOR).coerceIn(0.0, 1.0)

    /** MediaRecorder.getMaxAmplitude(0...32767)→ dBFS。 */
    fun amplitudeToPower(amplitude: Int): Double = if (amplitude <= 0) Double.NEGATIVE_INFINITY else 20 * log10(amplitude / 32767.0)
}

/** 録音部品の状態遷移(iOS の `AudioRecorderMachine`)。再生は別で、ここは録音の有無だけを持つ。 */
class AudioRecorderMachine(hasClip: Boolean) {
    enum class Phase { IDLE, RECORDING, RECORDED }
    enum class Event { START, STOP, FAIL, DISCARD }
    enum class RemoteCommand { TOGGLE_PLAY_PAUSE, PLAY, PAUSE }
    enum class RemoteAction { START_RECORDING, STOP_RECORDING, STOP_PLAYBACK }

    var phase: Phase = if (hasClip) Phase.RECORDED else Phase.IDLE
        private set
    private var hadClip = hasClip

    /** 受け付けたら true(無効な遷移は状態を変えず false)。 */
    fun send(event: Event): Boolean {
        when {
            event == Event.START && phase != Phase.RECORDING -> {
                hadClip = phase == Phase.RECORDED
                phase = Phase.RECORDING
            }
            event == Event.STOP && phase == Phase.RECORDING -> phase = Phase.RECORDED
            event == Event.FAIL && phase == Phase.RECORDING -> phase = if (hadClip) Phase.RECORDED else Phase.IDLE
            event == Event.DISCARD && phase == Phase.RECORDED -> {
                phase = Phase.IDLE
                hadClip = false
            }
            else -> return false
        }
        return true
    }

    /** イヤホンのボタン: 再生中なら止めるだけ。それ以外は 再生/一時停止 = 録音の開始/停止。 */
    fun remoteAction(command: RemoteCommand, isPlaying: Boolean): RemoteAction? {
        if (isPlaying) return if (command == RemoteCommand.PLAY) null else RemoteAction.STOP_PLAYBACK
        return when {
            phase == Phase.RECORDING && command != RemoteCommand.PLAY -> RemoteAction.STOP_RECORDING
            phase != Phase.RECORDING && command != RemoteCommand.PAUSE -> RemoteAction.START_RECORDING
            else -> null
        }
    }
}

/** 文字起こしの結果を今の文の後ろに足す(空なら置き換え、それ以外は改行で区切る・iOS の `DictationText`)。 */
object DictationText {
    fun appending(new: String, existing: String): String {
        val n = new.trim()
        if (n.isEmpty()) return existing
        if (existing.isBlank()) return n
        return if (existing.endsWith("\n")) existing + n else existing + "\n" + n
    }
}

/** 読み上げの声の言語(UI の言語 ja → ja-JP、en → en-US・iOS の `LeafSpeech.voiceLanguage`)。 */
object SpeechVoice {
    fun languageTag(uiLanguage: String): String = if (uiLanguage == "en") "en-US" else "ja-JP"
}

/**
 * 音の取り合いを 1 か所で決める(録音・再生・読み上げ・話して入力)。同時に持てるのは 1 つ。
 * 新しく取ったら前の持ち主を止める。ただし録音・話して入力の間は読み上げを始めない(TTS の音をマイクが拾わない・iOS の micBusy)。
 */
class AudioClaims {
    enum class Use(val usesMic: Boolean) { RECORDING(true), DICTATION(true), PLAYBACK(false), SPEECH(false) }

    private class Holder(val owner: String, val use: Use, val stop: () -> Unit)

    private var holder: Holder? = null

    val current: Use? get() = holder?.use
    val owner: String? get() = holder?.owner

    /** 取れたら true。取れない時(マイク使用中の読み上げ)は false で、前の持ち主は止めない。 */
    fun claim(owner: String, use: Use, stop: () -> Unit): Boolean {
        val prev = holder
        if (use == Use.SPEECH && prev != null && prev.owner != owner && prev.use.usesMic) return false
        holder = Holder(owner, use, stop)
        if (prev != null && prev.owner != owner) prev.stop()
        return true
    }

    /** その持ち主が今の持ち主の時だけ手放す。 */
    fun release(owner: String) {
        if (holder?.owner == owner) holder = null
    }

    /** 外から奪われた(電話・ほかのアプリ)。今の持ち主を止めて手放す。 */
    fun interrupt() {
        val prev = holder ?: return
        holder = null
        prev.stop()
    }
}
