package jp.kz2601.metamon.feedback

import androidx.annotation.StringRes
import androidx.compose.animation.AnimatedVisibility
import androidx.compose.animation.fadeIn
import androidx.compose.animation.fadeOut
import androidx.compose.animation.slideInVertically
import androidx.compose.animation.slideOutVertically
import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberUpdatedState
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalAccessibilityManager
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.CustomAccessibilityAction
import androidx.compose.ui.semantics.clearAndSetSemantics
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.customActions
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import jp.kz2601.metamon.R
import jp.kz2601.metamon.renderer.a11y.TalkBack
import jp.kz2601.metamon.renderer.a11y.rememberTalkBackRunning
import jp.kz2601.metamon.runtime.RuntimeBanner
import jp.kz2601.metamon.runtime.resolve
import jp.kz2601.metamon.services.sync.HotReloadStatus
import jp.kz2601.metamon.ui.CraneMascot
import jp.kz2601.metamon.ui.CranePose
import kotlinx.coroutines.delay

/** ホットリロードのつながり方の文言(#1108)。購読していない時は出さない。 */
object HotReloadStatusCopy {
    data class Copy(@StringRes val label: Int, @StringRes val description: Int, val live: Boolean)

    fun of(status: HotReloadStatus): Copy? = when (status) {
        HotReloadStatus.OFF -> null
        HotReloadStatus.REALTIME -> Copy(R.string.hot_reload_realtime, R.string.hot_reload_realtime_a11y, live = true)
        HotReloadStatus.POLLING -> Copy(R.string.hot_reload_polling, R.string.hot_reload_polling_a11y, live = false)
    }
}

/** 「リアルタイム / ポーリング中」の小さな表示(下部の開発用の行)。TalkBack は説明つきの 1 項目として読む。 */
@Composable
fun HotReloadStatusBadge(status: HotReloadStatus, modifier: Modifier = Modifier) {
    val copy = HotReloadStatusCopy.of(status) ?: return
    val description = stringResource(copy.description)
    val dot = if (copy.live) MaterialTheme.colorScheme.primary else MaterialTheme.colorScheme.outline
    Row(
        modifier.padding(horizontal = 8.dp).clearAndSetSemantics { contentDescription = description },
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(4.dp),
    ) {
        Box(Modifier.size(6.dp).background(dot, CircleShape))
        Text(stringResource(copy.label), style = MaterialTheme.typography.labelSmall)
    }
}

/**
 * 上部のバナー(iOS の summaryBanner)。ホットリロードの「最新の内容に更新しました」を出す。
 * 3 秒で消え、タップでも閉じる。読み上げは TalkBackAnnouncer が出したのと同時に行う。TalkBack の時は長めに出す(H8)。
 */
@Composable
fun RuntimeBannerView(banner: RuntimeBanner?, onDismiss: (Long) -> Unit) {
    val a11y = LocalAccessibilityManager.current
    val talkBack = rememberTalkBackRunning()
    val dismiss by rememberUpdatedState(onDismiss)
    if (banner != null) {
        LaunchedEffect(banner.id) {
            val recommended = a11y?.calculateRecommendedTimeoutMillis(BANNER_MILLIS, containsIcons = false, containsText = true, containsControls = false) ?: BANNER_MILLIS
            delay(maxOf(recommended, TalkBack.holdMillis(BANNER_MILLIS, talkBack)))
            dismiss(banner.id)
        }
    }
    // 消える間も直前の文を出しておく
    val last = remember { arrayOfNulls<RuntimeBanner>(1) }
    if (banner != null) last[0] = banner
    val shown = last[0]
    val close = stringResource(R.string.feedback_close)
    AnimatedVisibility(
        visible = banner != null,
        enter = slideInVertically { -it } + fadeIn(),
        exit = slideOutVertically { -it } + fadeOut(),
    ) {
        val text = shown?.text?.resolve().orEmpty()
        Surface(
            shape = RoundedCornerShape(16.dp),
            tonalElevation = 3.dp,
            shadowElevation = 2.dp,
            border = BorderStroke(1.dp, MaterialTheme.colorScheme.outlineVariant),
            modifier = Modifier.fillMaxWidth().padding(top = 12.dp)
                .clickable { shown?.let { dismiss(it.id) } }
                .semantics { customActions = listOf(CustomAccessibilityAction(close) { shown?.let { dismiss(it.id) }; true }) },
        ) {
            Row(Modifier.padding(horizontal = 12.dp, vertical = 10.dp), verticalAlignment = Alignment.CenterVertically) {
                CraneMascot(CranePose.JOY, size = 44.dp, sway = 6f)
                Text(text, Modifier.weight(1f).padding(start = 10.dp), style = MaterialTheme.typography.bodyMedium)
            }
        }
    }
}

private const val BANNER_MILLIS = 3_000L
