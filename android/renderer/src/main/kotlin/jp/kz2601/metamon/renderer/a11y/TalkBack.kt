package jp.kz2601.metamon.renderer.a11y

import android.content.Context
import android.view.accessibility.AccessibilityManager
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalView

/**
 * TalkBack への読み上げと、TalkBack の時の出し方(iOS の `VoiceOverAnnouncer`・H8)。
 * 読む文は読み上げにだけ使い、ログには出さない(話した内容・AI の返事を含むため)。
 */
object TalkBack {
    /** 自動で消えるまで。TalkBack の時は読み上げを聞いてから探して押せるよう長く(iOS の `holdSeconds`) */
    fun holdMillis(baseMillis: Long, talkBack: Boolean): Long =
        if (talkBack) maxOf(baseMillis * 3, MIN_HOLD_MILLIS) else baseMillis

    /** 「削除しました・元に戻す」で取り消せる時間(iOS の `RecordUndo.seconds`) */
    fun recordUndoMillis(talkBack: Boolean): Long = if (talkBack) RECORD_UNDO_TALKBACK_MILLIS else RECORD_UNDO_MILLIS

    /** 読む文。空白だけなら null(読まない) */
    fun text(raw: String?): String? = raw?.trim()?.takeIf { it.isNotEmpty() }

    /** TalkBack(触って読む)が動いているか */
    fun isRunning(context: Context): Boolean =
        (context.getSystemService(Context.ACCESSIBILITY_SERVICE) as? AccessibilityManager)
            ?.let { it.isEnabled && it.isTouchExplorationEnabled } == true

    const val RECORD_UNDO_MILLIS = 5_000L
    const val RECORD_UNDO_TALKBACK_MILLIS = 10_000L
    private const val MIN_HOLD_MILLIS = 15_000L
}

/** TalkBack が動いているか。途中で入れた・切ったらすぐ変わる。 */
@Composable
fun rememberTalkBackRunning(): Boolean {
    val context = LocalContext.current
    var running by remember(context) { mutableStateOf(TalkBack.isRunning(context)) }
    DisposableEffect(context) {
        val manager = context.getSystemService(Context.ACCESSIBILITY_SERVICE) as? AccessibilityManager
        val listener = AccessibilityManager.TouchExplorationStateChangeListener { running = TalkBack.isRunning(context) }
        manager?.addTouchExplorationStateChangeListener(listener)
        running = TalkBack.isRunning(context)
        onDispose { manager?.removeTouchExplorationStateChangeListener(listener) }
    }
    return running
}

/** TalkBack に 1 文を読ませる関数。読み上げのサービスが無ければ OS 側で何も起きない。 */
@Composable
fun rememberAnnouncer(): (String) -> Unit {
    val view = LocalView.current
    return remember(view) {
        { raw ->
            // 出たのと同時に読ませる(liveRegion は新しく出た要素を読まないことがある)
            @Suppress("DEPRECATION")
            TalkBack.text(raw)?.let(view::announceForAccessibility)
        }
    }
}
