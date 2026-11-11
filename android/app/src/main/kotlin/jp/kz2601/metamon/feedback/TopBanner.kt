package jp.kz2601.metamon.feedback

import androidx.compose.animation.AnimatedVisibility
import androidx.compose.animation.fadeIn
import androidx.compose.animation.fadeOut
import androidx.compose.animation.slideInVertically
import androidx.compose.animation.slideOutVertically
import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.clickable
import androidx.compose.foundation.gestures.Orientation
import androidx.compose.foundation.gestures.draggable
import androidx.compose.foundation.gestures.rememberDraggableState
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.offset
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableFloatStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberUpdatedState
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalAccessibilityManager
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.CustomAccessibilityAction
import androidx.compose.ui.semantics.customActions
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.IntOffset
import androidx.compose.ui.unit.dp
import jp.kz2601.metamon.R
import jp.kz2601.metamon.renderer.a11y.TalkBack
import jp.kz2601.metamon.renderer.a11y.rememberTalkBackRunning
import jp.kz2601.metamon.ui.CraneMascot
import jp.kz2601.metamon.ui.CranePose
import jp.kz2601.metamon.voice.UndoAction
import kotlinx.coroutines.delay
import kotlin.math.roundToInt

/** 上部のバナーの 1 回分。[key] が変われば出し直す(時間も数え直す)。 */
data class TopBannerItem(val source: TopBannerQueue.Source, val key: Any, val text: String, val action: UndoAction? = null)

/**
 * 上部のバナー(iOS の summaryCard)。変身の要約・ホットリロードの「最新の内容に更新しました」を同じ部品で出す。
 * 上に払う・タップで閉じる。文字数に応じて 3〜5 秒(ボタン付きは 6 秒)で消える。TalkBack の時は長めに出す(#894・H8)。
 * 読み上げは TalkBackAnnouncer が出したのと同時に行う(liveRegion と二重に読まない)。
 * [onDismiss] には閉じる回の項目を渡す(古い回の時間切れ・閉じる途中のタップで新しい回を消さないよう、呼ぶ側で鍵を確かめる)。
 */
@Composable
fun TopBanner(item: TopBannerItem?, onDismiss: (TopBannerItem) -> Unit, actionButton: @Composable (UndoAction) -> Unit) {
    val a11y = LocalAccessibilityManager.current
    val talkBack = rememberTalkBackRunning()
    val dismiss by rememberUpdatedState(onDismiss)
    if (item != null) {
        LaunchedEffect(item.key) {
            val base = TopBannerTiming.baseMillis(item.text, item.action != null)
            val recommended = a11y?.calculateRecommendedTimeoutMillis(base, containsIcons = false, containsText = true, containsControls = item.action != null) ?: base
            delay(maxOf(recommended, TalkBack.holdMillis(base, talkBack)))
            dismiss(item)
        }
    }
    // 消える間も直前の文を出しておく
    val last = remember { arrayOfNulls<TopBannerItem>(1) }
    if (item != null) last[0] = item
    val shown = last[0]
    var drag by remember { mutableFloatStateOf(0f) }
    val threshold = with(LocalDensity.current) { SWIPE_DISMISS_DISTANCE.toPx() }
    val close = stringResource(R.string.feedback_close)
    AnimatedVisibility(
        visible = item != null,
        enter = slideInVertically { -it } + fadeIn(),
        exit = slideOutVertically { -it } + fadeOut(),
    ) {
        Surface(
            shape = RoundedCornerShape(16.dp),
            tonalElevation = 3.dp,
            shadowElevation = 2.dp,
            border = BorderStroke(1.dp, MaterialTheme.colorScheme.outlineVariant),
            modifier = Modifier.fillMaxWidth().padding(top = 12.dp)
                .offset { IntOffset(0, drag.roundToInt()) }
                // 上に払って閉じる(元に戻す付きでも・iOS の swipeToDismiss)
                .draggable(
                    orientation = Orientation.Vertical,
                    state = rememberDraggableState { delta -> drag = (drag + delta).coerceAtMost(0f) },
                    onDragStopped = {
                        if (-drag > threshold) shown?.let(dismiss)
                        drag = 0f
                    },
                )
                .clickable { shown?.let(dismiss) }
                .semantics { customActions = listOf(CustomAccessibilityAction(close) { shown?.let(dismiss); true }) },
        ) {
            Row(Modifier.padding(horizontal = 12.dp, vertical = 10.dp), verticalAlignment = Alignment.CenterVertically) {
                CraneMascot(CranePose.JOY, size = 44.dp, sway = 6f)
                Text(shown?.text.orEmpty(), Modifier.weight(1f).padding(start = 10.dp), style = MaterialTheme.typography.bodyMedium)
                shown?.action?.let { actionButton(it) }
            }
        }
    }
}

internal val SWIPE_DISMISS_DISTANCE = 40.dp
