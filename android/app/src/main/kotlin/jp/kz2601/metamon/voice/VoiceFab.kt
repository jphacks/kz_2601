package jp.kz2601.metamon.voice

import androidx.compose.animation.AnimatedVisibility
import androidx.compose.animation.EnterTransition
import androidx.compose.animation.ExitTransition
import androidx.compose.animation.core.animateFloatAsState
import androidx.compose.animation.fadeIn
import androidx.compose.animation.fadeOut
import androidx.compose.foundation.ExperimentalFoundationApi
import androidx.compose.foundation.background
import androidx.compose.foundation.combinedClickable
import androidx.compose.foundation.gestures.detectDragGestures
import androidx.compose.foundation.gestures.detectHorizontalDragGestures
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.BoxWithConstraints
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.offset
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.SideEffect
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
import androidx.compose.ui.geometry.Rect
import androidx.compose.ui.geometry.Size
import androidx.compose.ui.input.pointer.pointerInput
import androidx.compose.ui.input.pointer.util.VelocityTracker
import androidx.compose.ui.layout.boundsInRoot
import androidx.compose.ui.layout.onGloballyPositioned
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
import jp.kz2601.metamon.onboarding.CoachMark
import jp.kz2601.metamon.onboarding.CoachMarkBubble
import jp.kz2601.metamon.onboarding.CoachMarkEvent
import jp.kz2601.metamon.renderer.theme.LocalReduceMotion
import jp.kz2601.metamon.renderer.screen.LocalFabCoordinator
import kotlinx.coroutines.delay
import kotlin.math.roundToInt

private val FAB_SIZE = 64.dp
private val HORIZONTAL_INSET = 20.dp
private val AUTO_BOTTOM_INSET = 20.dp
private val TOP_RESERVE = 24.dp

/** 最下段の部品の上端と FAB の間(iOS の `fabClearanceGap`) */
private val CLEARANCE_GAP = 8.dp
private val HANDLE_WIDTH = 14.dp
private val HANDLE_HIT_WIDTH = 28.dp
private val HANDLE_HEIGHT = 56.dp
private val REVEAL_DISTANCE = 20.dp

/** 吹き出しの矢印の先と FAB の間 */
private val BUBBLE_GAP = 4.dp

/**
 * マイク FAB(iOS の `VoiceFab`・#52)。ピクチャーインピクチャー風に動かせる。
 * - タップ =「このアプリを変える」(音声のシート)、長押し = 変身一覧
 * - ドラッグで動かし、離すと近い左右の辺に吸着。端の近くで離すと画面の外へ隠れ、取っ手だけ残る(タップ / 内側へ引くと戻る)
 * - 位置は [FabPositionStore] でアプリ共通に残す。生成中は薄くして触れなくする
 * - [coachMark] があれば FAB の上に吹き出し(#900)。長押し・ドラッグ(TalkBack の操作の一覧も)をしたら [onGesture] で知らせる
 */
@OptIn(ExperimentalFoundationApi::class)
@Composable
fun VoiceFab(
    busy: Boolean,
    onTap: () -> Unit,
    onLongPress: () -> Unit,
    modifier: Modifier = Modifier,
    coachMark: CoachMark? = null,
    onCoachMarkEvent: (CoachMark, CoachMarkEvent) -> Unit = { _, _ -> },
    onGesture: (CoachMark) -> Unit = {},
) {
    val longPress = {
        onGesture(CoachMark.FAB_LONG_PRESS)
        onLongPress()
    }
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
    // 弾いた方向で辺を選ぶための速さ(#1204・iOS の predictedEndTranslation)
    val velocity = remember { VelocityTracker() }
    // 描画器とのやり取り: 最下段の部品の上端を受け取り、止まっている FAB の矩形を渡す(#83 / #157)
    val coordinator = LocalFabCoordinator.current
    var areaInRoot by remember { mutableStateOf(Rect.Zero) }
    DisposableEffect(coordinator) { onDispose { coordinator?.fabFrame = null } }

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

    BoxWithConstraints(
        modifier.fillMaxSize().alpha(if (busy) 0.35f else 1f).onGloballyPositioned { areaInRoot = it.boundsInRoot() },
    ) {
        val density = LocalDensity.current
        val width = constraints.maxWidth.toFloat()
        val height = constraints.maxHeight.toFloat()
        val size = with(density) { FAB_SIZE.toPx() }
        val inset = with(density) { HORIZONTAL_INSET.toPx() }
        // 吸着した側の最下段に部品がある画面では、その上端まで上げる(#83)
        val autoInset = FabDock.autoInset(
            base = with(density) { AUTO_BOTTOM_INSET.toPx() },
            obstructionTop = coordinator?.obstructionTop(leadingSide = side == FabSide.LEFT),
            areaBottom = areaInRoot.bottom,
            gap = with(density) { CLEARANCE_GAP.toPx() },
        )
        val maxBottom = height - size - with(density) { TOP_RESERVE.toPx() }
        val storedPx = if (storedBottomDp < 0) FabDock.AUTO_BOTTOM else with(density) { storedBottomDp.dp.toPx() }
        val bottom = FabDock.displayedBottom(storedPx, autoInset, maxBottom)
        val restX = if (side == FabSide.LEFT) inset + size / 2 else width - inset - size / 2
        val restY = height - bottom - size / 2
        // 止まっている FAB の矩形(ルート座標)。隠れている・動かしている間は渡さない(#157)
        val frame = if (hidden || dragging || areaInRoot.isEmpty) {
            null
        } else {
            Rect(Offset(areaInRoot.left + restX - size / 2, areaInRoot.top + restY - size / 2), Size(size, size))
        }
        SideEffect { coordinator?.fabFrame = frame }

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
                            onDragStart = {
                                if (!longPressFired) dragging = true
                                velocity.resetTracking()
                            },
                            onDragEnd = {
                                if (dragging) {
                                    val cx = restX + drag.x
                                    val predicted = FabDock.predictedCenterX(cx, velocity.calculateVelocity().x)
                                    val snap = FabDock.snap(cx, predicted, width, size)
                                    val newBottom = FabDock.storedBottom(restY + drag.y, height, size, autoInset, maxBottom)
                                    apply(snap.side, snap.hidden, newBottom / density.density)
                                    onGesture(CoachMark.FAB_DRAG)
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
                            // FAB は指と一緒に動くので、指の位置ではなく動かした量で速さを測る
                            velocity.addPosition(change.uptimeMillis, drag)
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
                            longPress()
                        },
                    )
                    .semantics {
                        contentDescription = label
                        stateDescription = hint
                        customActions = listOf(
                            CustomAccessibilityAction(switcher) { longPress(); true },
                            CustomAccessibilityAction(moveOther) {
                                apply(if (side == FabSide.LEFT) FabSide.RIGHT else FabSide.LEFT, false)
                                onGesture(CoachMark.FAB_DRAG)
                                true
                            },
                            CustomAccessibilityAction(hide) {
                                apply(side, true)
                                onGesture(CoachMark.FAB_DRAG)
                                true
                            },
                        )
                    }
                    .testTag("voice.fab"),
                contentAlignment = Alignment.Center,
            ) {
                MicGlyph(MaterialTheme.colorScheme.onPrimary, stop = false, modifier = Modifier.size(FAB_SIZE * 0.42f))
            }
            // 吹き出し(#900)。動かしている間・生成中は出さない。FAB の真上に置き、矢印は FAB の中心を指す
            val mark = coachMark?.takeUnless { busy || dragging }
            val reduceMotion = LocalReduceMotion.current
            AnimatedVisibility(
                visible = mark != null,
                enter = if (reduceMotion) EnterTransition.None else fadeIn(),
                exit = if (reduceMotion) ExitTransition.None else fadeOut(),
                modifier = Modifier
                    .align(if (side == FabSide.LEFT) Alignment.TopStart else Alignment.TopEnd)
                    .padding(horizontal = HORIZONTAL_INSET)
                    .height(with(density) { (restY - size / 2).coerceAtLeast(0f).toDp() - BUBBLE_GAP }.coerceAtLeast(0.dp)),
            ) {
                // 消える間も中身を描くため、最後に出したものを覚えておく
                var shown by remember { mutableStateOf(mark) }
                if (mark != null) shown = mark
                shown?.let { m ->
                    Box(contentAlignment = if (side == FabSide.LEFT) Alignment.BottomStart else Alignment.BottomEnd) {
                        CoachMarkBubble(
                            mark = m,
                            onEvent = { onCoachMarkEvent(m, it) },
                            arrowAtEnd = side == FabSide.RIGHT,
                            arrowStart = FAB_SIZE / 2,
                        )
                    }
                }
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
                    .combinedClickable(enabled = !busy, onClick = { apply(side, false) }, onLongClick = longPress)
                    .semantics {
                        contentDescription = handleLabel
                        customActions = listOf(CustomAccessibilityAction(switcher) { longPress(); true })
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
