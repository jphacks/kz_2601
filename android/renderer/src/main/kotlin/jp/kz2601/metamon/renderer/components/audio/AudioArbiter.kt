package jp.kz2601.metamon.renderer.components.audio

import android.content.Context
import android.media.AudioAttributes
import android.media.AudioFocusRequest
import android.media.AudioManager
import android.os.Handler
import android.os.Looper

/**
 * アプリの音の窓口(iOS の `AudioSessionLease`)。録音・再生・読み上げ・話して入力が [claim] で順に持つ。
 * 中身の取り合いは [AudioClaims]、OS とは AudioFocus で話す(電話・ほかのアプリに奪われたら今の持ち主を止める)。
 * メインスレッドから呼ぶ。
 */
object AudioArbiter {
    private val claims = AudioClaims()
    private var focus: AudioFocusRequest? = null
    private var manager: AudioManager? = null

    val current: AudioClaims.Use? get() = claims.current

    fun isHeldBy(owner: String): Boolean = claims.owner == owner

    /** 取れたら true。話して入力は認識器が自分でフォーカスを取るので、ここでは取り合いだけ。 */
    fun claim(context: Context, owner: String, use: AudioClaims.Use, stop: () -> Unit): Boolean {
        if (!claims.claim(owner, use, stop)) return false
        abandonFocus()
        if (use == AudioClaims.Use.DICTATION) return true
        val am = context.applicationContext.getSystemService(AudioManager::class.java) ?: return true
        val attributes = AudioAttributes.Builder()
            .setUsage(if (use == AudioClaims.Use.SPEECH) AudioAttributes.USAGE_ASSISTANT else AudioAttributes.USAGE_MEDIA)
            .setContentType(if (use == AudioClaims.Use.PLAYBACK) AudioAttributes.CONTENT_TYPE_MUSIC else AudioAttributes.CONTENT_TYPE_SPEECH)
            .build()
        val gain = if (use == AudioClaims.Use.RECORDING) AudioManager.AUDIOFOCUS_GAIN_TRANSIENT_EXCLUSIVE else AudioManager.AUDIOFOCUS_GAIN_TRANSIENT
        val request = AudioFocusRequest.Builder(gain)
            .setAudioAttributes(attributes)
            .setOnAudioFocusChangeListener({ change ->
                if (change == AudioManager.AUDIOFOCUS_LOSS || change == AudioManager.AUDIOFOCUS_LOSS_TRANSIENT) {
                    abandonFocus()
                    claims.interrupt()
                }
            }, Handler(Looper.getMainLooper()))
            .build()
        // 取れなくても(通話中など)録音・再生は試す。失敗はそれぞれが出す
        am.requestAudioFocus(request)
        focus = request
        manager = am
        return true
    }

    fun release(owner: String) {
        if (!isHeldBy(owner)) return
        claims.release(owner)
        abandonFocus()
    }

    private fun abandonFocus() {
        val f = focus ?: return
        manager?.abandonAudioFocusRequest(f)
        focus = null
    }
}
