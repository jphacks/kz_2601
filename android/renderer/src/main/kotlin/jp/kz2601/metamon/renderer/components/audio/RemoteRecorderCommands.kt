package jp.kz2601.metamon.renderer.components.audio

import android.content.Context
import android.content.Intent
import android.media.session.MediaSession
import android.media.session.PlaybackState
import android.view.KeyEvent

/**
 * イヤホンの 再生 / 一時停止 ボタンを録音部品に渡す(iOS の `RemoteRecorderCommands`)。
 * 録音部品が画面にある間だけ MediaSession を立て、離れたら外す(同時に 1 つ・後から出た部品が持つ)。
 * Android がボタンを届けるのは「最後に音を扱ったセッション」なので、ほかのアプリが音楽を流していると届かない。
 */
object RemoteRecorderCommands {
    private var owner: String? = null
    private var session: MediaSession? = null

    fun attach(context: Context, owner: String, title: String, handler: (AudioRecorderMachine.RemoteCommand) -> Unit) {
        detachAll()
        this.owner = owner
        val s = MediaSession(context.applicationContext, "metamon-audioRecorder")
        s.setCallback(object : MediaSession.Callback() {
            @Suppress("DEPRECATION") // 型つきの getParcelableExtra は 33+(minSdk 31)
            override fun onMediaButtonEvent(mediaButtonIntent: Intent): Boolean {
                val key = mediaButtonIntent.getParcelableExtra<KeyEvent>(Intent.EXTRA_KEY_EVENT) ?: return false
                if (key.action != KeyEvent.ACTION_DOWN || key.repeatCount > 0) return true
                val command = when (key.keyCode) {
                    KeyEvent.KEYCODE_HEADSETHOOK, KeyEvent.KEYCODE_MEDIA_PLAY_PAUSE -> AudioRecorderMachine.RemoteCommand.TOGGLE_PLAY_PAUSE
                    KeyEvent.KEYCODE_MEDIA_PLAY -> AudioRecorderMachine.RemoteCommand.PLAY
                    KeyEvent.KEYCODE_MEDIA_PAUSE -> AudioRecorderMachine.RemoteCommand.PAUSE
                    else -> return false
                }
                handler(command)
                return true
            }
        })
        s.setPlaybackState(
            PlaybackState.Builder()
                .setActions(PlaybackState.ACTION_PLAY or PlaybackState.ACTION_PAUSE or PlaybackState.ACTION_PLAY_PAUSE)
                .setState(PlaybackState.STATE_PAUSED, 0, 1f)
                .build(),
        )
        s.setMetadata(android.media.MediaMetadata.Builder().putString(android.media.MediaMetadata.METADATA_KEY_TITLE, title).build())
        s.isActive = true
        session = s
    }

    /** その部品が今の持ち主の時だけ外す(画面遷移で新しい部品が先に attach しても消さない)。 */
    fun detach(owner: String) {
        if (this.owner == owner) detachAll()
    }

    private fun detachAll() {
        session?.let {
            it.isActive = false
            it.release()
        }
        session = null
        owner = null
    }
}
