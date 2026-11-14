package jp.kz2601.metamon.onboarding

import androidx.annotation.StringRes
import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.Canvas
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Button
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.shadow
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.graphics.Path
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.clearAndSetSemantics
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import jp.kz2601.metamon.R
import jp.kz2601.metamon.renderer.a11y.rememberAnnouncer
import jp.kz2601.metamon.renderer.a11y.rememberTalkBackRunning

/** 吹き出しの文言(iOS の `CoachMarkCopy`)。[talkBack] は TalkBack の利用者向け(長押し・ドラッグの代わりに操作の一覧を使う)。 */
object CoachMarkCopy {
    @StringRes
    fun title(mark: CoachMark): Int = when (mark) {
        CoachMark.FAB_LONG_PRESS -> R.string.coach_mark_fab_long_press_title
        CoachMark.FAB_DRAG -> R.string.coach_mark_fab_drag_title
    }

    @StringRes
    fun body(mark: CoachMark): Int = when (mark) {
        CoachMark.FAB_LONG_PRESS -> R.string.coach_mark_fab_long_press_body
        CoachMark.FAB_DRAG -> R.string.coach_mark_fab_drag_body
    }

    @StringRes
    fun talkBack(mark: CoachMark): Int = when (mark) {
        CoachMark.FAB_LONG_PRESS -> R.string.coach_mark_fab_long_press_talkback
        CoachMark.FAB_DRAG -> R.string.coach_mark_fab_drag_talkback
    }
}

/**
 * 部品を指す吹き出し(iOS の `CoachMarkBubble`)。矢印は下向き(部品の上に出す)。
 * [arrowStart] = 矢印の先の位置(吹き出しの [arrowAtEnd] でない側の端から)。
 * 出た時に題と TalkBack 向けの説明を読み上げる。矢印は飾りなので TalkBack から隠す。
 */
@Composable
fun CoachMarkBubble(
    mark: CoachMark,
    onEvent: (CoachMarkEvent) -> Unit,
    modifier: Modifier = Modifier,
    arrowAtEnd: Boolean = false,
    arrowStart: Dp = 24.dp,
) {
    val talkBack = rememberTalkBackRunning()
    val announce = rememberAnnouncer()
    val title = stringResource(CoachMarkCopy.title(mark))
    val talkBackText = stringResource(CoachMarkCopy.talkBack(mark))
    LaunchedEffect(mark) {
        onEvent(CoachMarkEvent.SHOWN)
        announce(title + "\n" + talkBackText)
    }
    val surface = MaterialTheme.colorScheme.surfaceContainerHigh
    Column(modifier.testTag("coachMark.${mark.key}"), horizontalAlignment = if (arrowAtEnd) Alignment.End else Alignment.Start) {
        Surface(
            shape = RoundedCornerShape(16.dp),
            color = surface,
            border = BorderStroke(1.dp, MaterialTheme.colorScheme.outlineVariant),
            modifier = Modifier.widthIn(max = 300.dp).shadow(12.dp, RoundedCornerShape(16.dp)),
        ) {
            Column(Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
                Text(title, style = MaterialTheme.typography.titleSmall, fontWeight = FontWeight.Bold)
                Text(
                    if (talkBack) talkBackText else stringResource(CoachMarkCopy.body(mark)),
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
                Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(12.dp)) {
                    TextButton(onClick = { onEvent(CoachMarkEvent.NEVER_AGAIN) }, modifier = Modifier.testTag("coachMark.never")) {
                        Text(stringResource(R.string.coach_mark_never_again), color = MaterialTheme.colorScheme.onSurfaceVariant)
                    }
                    Button(onClick = { onEvent(CoachMarkEvent.DISMISSED) }, modifier = Modifier.testTag("coachMark.gotIt")) {
                        Text(stringResource(R.string.coach_mark_got_it))
                    }
                }
            }
        }
        val start = (arrowStart - ARROW_WIDTH / 2).coerceAtLeast(0.dp)
        Canvas(
            Modifier
                .padding(start = if (arrowAtEnd) 0.dp else start, end = if (arrowAtEnd) start else 0.dp)
                .size(ARROW_WIDTH, ARROW_HEIGHT)
                .clearAndSetSemantics {},
        ) {
            val path = Path().apply {
                moveTo(0f, 0f)
                lineTo(size.width, 0f)
                lineTo(size.width / 2, size.height)
                close()
            }
            drawPath(path, surface)
            drawLine(surface, Offset(0f, 0f), Offset(size.width, 0f))
        }
    }
}

private val ARROW_WIDTH = 18.dp
private val ARROW_HEIGHT = 9.dp
