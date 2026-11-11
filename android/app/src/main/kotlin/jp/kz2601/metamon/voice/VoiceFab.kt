package jp.kz2601.metamon.voice

import androidx.compose.animation.core.animateFloatAsState
import androidx.compose.foundation.ExperimentalFoundationApi
import androidx.compose.foundation.background
import androidx.compose.foundation.combinedClickable
import androidx.compose.foundation.gestures.detectDragGestures
import androidx.compose.foundation.gestures.detectHorizontalDragGestures
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.BoxWithConstraints
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.offset
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
import androidx.compose.runtime.mutableFloatStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.alpha
import androidx.compose.ui.draw.clip
import androidx.compose.ui.draw.scale
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.input.pointer.pointerInput
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.CustomAccessibilityAction
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.customActions
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.semantics.stateDescription
import androidx.compose.ui.unit.IntOffset
import androidx.compose.ui.unit.dp
import jp.kz2601.metamon.R
import kotlinx.coroutines.delay
import kotlin.math.roundToInt

private val FAB_SIZE = 64.dp
private val HORIZONTAL_INSET = 20.dp
private val AUTO_BOTTOM_INSET = 20.dp
private val TOP_RESERVE = 24.dp
private val HANDLE_WIDTH = 14.dp
private val HANDLE_HIT_WIDTH = 28.dp
private val HANDLE_HEIGHT = 56.dp
private val REVEAL_DISTANCE = 20.dp

/**
 * マイク FAB(iOS の `VoiceFab`・#52)。ピクチャーインピクチャー風に動かせる。
 * - タップ =「このアプリを変える」(音声のシート)、長押し = 変身一覧
 * - ドラッグで動かし、離すと近い左右の辺に吸着。端の近くで離すと画面の外へ隠れ、取っ手だけ残る(タップ / 内側へ引くと戻る)
 * - 位置は [FabPositionStore] でアプリ共通に残す。生成中は薄くして触れなくする
 */
@OptIn(ExperimentalFoundationApi::class)
@Composable
fun VoiceFab(busy: Boolean, onTap: () -> Unit, onLongPress: () -> Unit, modifier: Modifier = Modifier) {
    val context = LocalContext.current
    val store = remember { FabPositionStore(context) }
    var side by remember { mutableStateOf(store.side) }
    var storedBottomDp by remember { mutableFloatStateOf(store.bottomDp) }
    var hidden by remember { mutableStateOf(store.hidden) }
    var drag by remember { mutableStateOf(Offset.Zero) }
    var dragging by remember { mutableStateOf(false) }
    var longPressFired by remember { mutableStateOf(false) }
    var pull by remember { mutableFloatStateOf(0f) }
    var showsHint by remember { mutableStateOf(false) }

    fun apply(newSide: FabSide, newHidden: Boolean, bottomDp: Float = storedBottomDp) {
        val wasHidden = hidden
        side = newSide
        hidden = newHidden
        storedBottomDp = bottomDp
        store.side = newSide
        store.hidden = newHidden
        store.bottomDp = bottomDp
        if (FabHideHint.shouldShow(wasHidden, newHidden, store.hideHintShown)) {
            store.hideHintShown = true
            showsHint = true
        }
        if (!newHidden) showsHint = false
    }
    LaunchedEffect(showsHint) {
        if (showsHint) {
            delay(FabHideHint.MILLIS)
            showsHint = false
        }
    }

    val switcher = stringResource(R.string.voice_fab_switcher)
    val moveOther = stringResource(R.string.voice_fab_move_other_side)
    val hide = stringResource(R.string.voice_fab_hide)
    val label = stringResource(R.string.voice_change_this_app)
    val hint = stringResource(R.string.voice_fab_a11y_hint)
    val handleLabel = stringResource(R.string.voice_fab_handle)
    val scale by animateFloatAsState(if (dragging) 1.08f else 1f, label = "fabScale")

    BoxWithConstraints(modifier.fillMaxSize().alpha(if (busy) 0.35f else 1f)) {
        val density = LocalDensity.current
        val width = constraints.maxWidth.toFloat()
        val height = constraints.maxHeight.toFloat()
        val size = with(density) { FAB_SIZE.toPx() }
        val inset = with(density) { HORIZONTAL_INSET.toPx() }
        val autoInset = with(density) { AUTO_BOTTOM_INSET.toPx() }
        val maxBottom = height - size - with(density) { TOP_RESERVE.toPx() }
        val storedPx = if (storedBottomDp < 0) FabDock.AUTO_BOTTOM else with(density) { storedBottomDp.dp.toPx() }
        val bottom = FabDock.displayedBottom(storedPx, autoInset, maxBottom)
        val restX = if (side == FabSide.LEFT) inset + size / 2 else width - inset - size / 2
        val restY = height - bottom - size / 2

        if (!hidden) {
            Box(
                Modifier
                    .offset { IntOffset((restX + drag.x - size / 2).roundToInt(), (restY + drag.y - size / 2).roundToInt()) }
                    .size(FAB_SIZE)
                    .scale(scale)
                    .clip(CircleShape)
                    .background(MaterialTheme.colorScheme.primary)
                    .pointerInput(busy, width, height, restX, restY) {
                        if (busy) return@pointerInput
                        detectDragGestures(
                            onDragStart = { if (!longPressFired) dragging = true },
                            onDragEnd = {
                                if (dragging) {
                                    val cx = restX + drag.x
                                    val snap = FabDock.snap(cx, cx, width, size)
                                    val newBottom = FabDock.storedBottom(restY + drag.y, height, size, autoInset, maxBottom)
                                    apply(snap.side, snap.hidden, newBottom / density.density)
                                }
                                dragging = false
                                longPressFired = false
                                drag = Offset.Zero
                            },
                            onDragCancel = {
                                dragging = false
                                longPressFired = false
                                drag = Offset.Zero
                            },
                        ) { change, amount ->
                            if (!dragging) return@detectDragGestures
                            change.consume()
                            drag += amount
                        }
                    }
                    .combinedClickable(
                        enabled = !busy,
                        onClick = {
                            longPressFired = false
                            onTap()
                        },
                        onLongClick = {
                            longPressFired = true
                            onLongPress()
                        },
                    )
                    .semantics {
                        contentDescription = label
                        stateDescription = hint
                        customActions = listOf(
                            CustomAccessibilityAction(switcher) { onLongPress(); true },
                            CustomAccessibilityAction(moveOther) {
                                apply(if (side == FabSide.LEFT) FabSide.RIGHT else FabSide.LEFT, false)
                                true
                            },
                            CustomAccessibilityAction(hide) { apply(side, true); true },
                        )
                    }
                    .testTag("voice.fab"),
                contentAlignment = Alignment.Center,
            ) {
                MicGlyph(MaterialTheme.colorScheme.onPrimary, stop = false, modifier = Modifier.size(FAB_SIZE * 0.42f))
            }
        } else {
            val hitWidth = with(density) { HANDLE_HIT_WIDTH.toPx() }
            val handleHeight = with(density) { HANDLE_HEIGHT.toPx() }
            val reveal = with(density) { REVEAL_DISTANCE.toPx() }
            val x = if (side == FabSide.LEFT) pull else width - hitWidth - pull
            val shape = if (side == FabSide.LEFT) {
                RoundedCornerShape(topEnd = HANDLE_WIDTH / 2, bottomEnd = HANDLE_WIDTH / 2)
            } else {
                RoundedCornerShape(topStart = HANDLE_WIDTH / 2, bottomStart = HANDLE_WIDTH / 2)
            }
            Box(
                Modifier
                    .offset { IntOffset(x.roundToInt(), (restY - handleHeight / 2).roundToInt()) }
                    .size(HANDLE_HIT_WIDTH, HANDLE_HEIGHT)
                    .pointerInput(side) {
                        detectHorizontalDragGestures(
                            onDragEnd = {
                                if (FabDock.shouldReveal(if (side == FabSide.LEFT) pull else -pull, side, reveal)) apply(side, false)
                                pull = 0f
                            },
                            onDragCancel = { pull = 0f },
                        ) { change, amount ->
                            change.consume()
                            val inward = if (side == FabSide.LEFT) amount else -amount
                            pull = (pull + inward).coerceIn(0f, reveal * 1.5f)
                        }
                    }
                    .combinedClickable(enabled = !busy, onClick = { apply(side, false) }, onLongClick = onLongPress)
                    .semantics {
                        contentDescription = handleLabel
                        customActions = listOf(CustomAccessibilityAction(switcher) { onLongPress(); true })
                    }
                    .testTag("voice.fabHandle"),
                contentAlignment = if (side == FabSide.LEFT) Alignment.CenterStart else Alignment.CenterEnd,
            ) {
                Box(Modifier.size(HANDLE_WIDTH, HANDLE_HEIGHT).clip(shape).background(MaterialTheme.colorScheme.primary))
            }
            if (showsHint) {
                val hintY = with(density) { (restY - 18.dp.toPx()).roundToInt() }
                Surface(
                    shape = RoundedCornerShape(50),
                    color = MaterialTheme.colorScheme.inverseSurface,
                    modifier = Modifier
                        .align(if (side == FabSide.LEFT) Alignment.TopStart else Alignment.TopEnd)
                        .offset { IntOffset(0, hintY) }
                        .padding(horizontal = HANDLE_HIT_WIDTH + 4.dp),
                ) {
                    Text(
                        stringResource(R.string.voice_fab_hide_hint),
                        style = MaterialTheme.typography.labelMedium,
                        color = MaterialTheme.colorScheme.inverseOnSurface,
                        modifier = Modifier.padding(horizontal = 12.dp, vertical = 8.dp),
                    )
                }
            }
        }
    }
}
