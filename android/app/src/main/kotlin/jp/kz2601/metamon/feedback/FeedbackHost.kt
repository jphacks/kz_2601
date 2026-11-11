package jp.kz2601.metamon.feedback

import android.content.Intent
import android.net.Uri
import android.provider.Settings
import androidx.compose.animation.AnimatedVisibility
import androidx.compose.animation.fadeIn
import androidx.compose.animation.fadeOut
import androidx.compose.animation.slideInVertically
import androidx.compose.animation.slideOutVertically
import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.gestures.Orientation
import androidx.compose.foundation.gestures.draggable
import androidx.compose.foundation.gestures.rememberDraggableState
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.BoxScope
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.offset
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableFloatStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberUpdatedState
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalAccessibilityManager
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.CustomAccessibilityAction
import androidx.compose.ui.semantics.customActions
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.IntOffset
import androidx.compose.ui.unit.dp
import jp.kz2601.metamon.R
import jp.kz2601.metamon.renderer.a11y.TalkBack
import jp.kz2601.metamon.renderer.a11y.rememberTalkBackRunning
import jp.kz2601.metamon.runtime.AppRuntime
import jp.kz2601.metamon.runtime.Confirm
import jp.kz2601.metamon.runtime.RuntimeState
import jp.kz2601.metamon.runtime.UiText
import jp.kz2601.metamon.runtime.resolve
import jp.kz2601.metamon.voice.ErrorCard
import jp.kz2601.metamon.voice.UndoAction
import jp.kz2601.metamon.voice.UndoNotice
import jp.kz2601.metamon.voice.VoiceErrorCopy
import jp.kz2601.metamon.voice.VoiceUiState
import jp.kz2601.metamon.voice.VoiceViewModel
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch
import kotlin.math.roundToInt

/**
 * 生成したアプリの上に重ねる知らせ(#895・iOS の RootView の summaryCard / errorCardOverlay / ToastHost / alert)。
 * - 上: 変身の要約と「元に戻す」/「やり直す」、失敗の状態カード
 * - 下: トースト(3 秒・下に払って閉じる)と「削除しました・元に戻す」(5 秒。TalkBack の時は長め・H8)
 * - 確認ダイアログ(confirmMessage 付きのアクション)
 */
@Composable
fun BoxScope.FeedbackHost(vm: VoiceViewModel, state: VoiceUiState, runtime: AppRuntime, runtimeState: RuntimeState) {
    val context = LocalContext.current

    // undo / redo の結果は 1 つのトーストの枠に流す
    LaunchedEffect(state.notice) {
        val notice = state.notice ?: return@LaunchedEffect
        runtime.showToast(UiText.Res(notice.text))
        vm.dismissNotice()
    }

    if (!state.isInitial) {
        Column(Modifier.align(Alignment.TopCenter).fillMaxWidth().padding(horizontal = 12.dp)) {
            // 変身の要約とホットリロードの「最新の内容に更新しました」(#1108)は 1 枠で、後から来た方に置き換える(#1320)
            TopBannerSlot(vm, state, runtime, runtimeState)
            // シートを閉じた後の失敗(生成は送るとシートを閉じる)。シートの中ではシートが出す
            val error = state.error
            if (error != null && !state.voiceOpen) {
                ErrorCard(
                    VoiceErrorCopy.of(error),
                    onRetry = vm::retry,
                    onDismiss = vm::dismissError,
                    onUpgrade = vm.upgradeHandler(),
                    onOpenSettings = {
                        context.startActivity(
                            Intent(Settings.ACTION_APPLICATION_DETAILS_SETTINGS, Uri.fromParts("package", context.packageName, null))
                                .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK),
                        )
                    },
                )
            }
        }
    }

    Column(
        Modifier.align(Alignment.BottomCenter).padding(horizontal = 16.dp).padding(bottom = TOAST_BOTTOM_INSET),
        horizontalAlignment = Alignment.CenterHorizontally,
        verticalArrangement = Arrangement.spacedBy(8.dp),
    ) {
        RecordUndoToast(runtime, runtimeState)
        RuntimeToast(runtime, runtimeState.toast)
    }

    runtimeState.confirm?.let { ConfirmDialog(it, runtime) }
}

private val UndoNotice.text: Int
    get() = when (this) {
        UndoNotice.NO_EARLIER -> R.string.feedback_no_earlier
        UndoNotice.REDO_FAILED -> R.string.feedback_redo_failed
    }

/**
 * 上部のバナーの枠(iOS の summaryBanner)。変身の要約(修正の直後は「元に戻す」、戻した直後は「やり直す」・iOS の
 * `SummaryBannerActionButton`)とホットリロードの知らせを [TopBannerQueue] で 1 つに絞り、置き換えられた方は閉じる。
 */
@Composable
private fun TopBannerSlot(vm: VoiceViewModel, state: VoiceUiState, runtime: AppRuntime, runtimeState: RuntimeState) {
    val summaryText = state.summary ?: when (state.undone) {
        UndoAction.UNDO -> stringResource(R.string.feedback_undone)
        UndoAction.REDO -> stringResource(R.string.feedback_redone)
        null -> null
    }
    val summaryKey = summaryText?.let { it to state.summaryAction }
    val hotReload = runtimeState.banner
    val queue = remember { TopBannerQueue() }
    val pick = queue.pick(mapOf(TopBannerQueue.Source.SUMMARY to summaryKey, TopBannerQueue.Source.HOT_RELOAD to hotReload?.id))
    val currentSummaryKey by rememberUpdatedState(summaryKey)
    val dismiss: (TopBannerItem) -> Unit = { item ->
        when (item.source) {
            TopBannerQueue.Source.SUMMARY -> if (item.key == currentSummaryKey) vm.dismissSummary()
            TopBannerQueue.Source.HOT_RELOAD -> runtime.dismissBanner(item.key as Long)
        }
    }
    val item = when (pick.shown) {
        TopBannerQueue.Source.SUMMARY -> summaryKey?.let { TopBannerItem(TopBannerQueue.Source.SUMMARY, it, it.first, state.summaryAction) }
        TopBannerQueue.Source.HOT_RELOAD -> hotReload?.let { TopBannerItem(TopBannerQueue.Source.HOT_RELOAD, it.id, it.text.resolve()) }
        null -> null
    }
    // 置き換えられた方は閉じる(後で戻ってこない)
    LaunchedEffect(pick.superseded, summaryKey, hotReload?.id) {
        for (source in pick.superseded) {
            when (source) {
                TopBannerQueue.Source.SUMMARY -> summaryKey?.let { dismiss(TopBannerItem(source, it, it.first)) }
                TopBannerQueue.Source.HOT_RELOAD -> hotReload?.let { dismiss(TopBannerItem(source, it.id, "")) }
            }
        }
    }
    TopBanner(item, onDismiss = dismiss) { action ->
        val enabled = !state.undoing && !state.isGenerating &&
            (if (action == UndoAction.UNDO) state.canUndo else state.canRedo)
        // 押した時点の版を送る(連打の 2 回目を捨てる)
        val version = state.document.version
        CapsuleButton(
            label = stringResource(if (action == UndoAction.UNDO) R.string.feedback_undo else R.string.feedback_redo),
            enabled = enabled,
            onClick = { if (action == UndoAction.UNDO) vm.undo(version) else vm.redo(version) },
        )
    }
}

/** 音声のシートの上部の「元に戻す」(戻した直後は「やり直す」も)(iOS の `UndoHeaderButtons`)。 */
@Composable
fun UndoHeaderButtons(state: VoiceUiState, vm: VoiceViewModel, modifier: Modifier = Modifier) {
    if (state.isInitial) return
    val busy = state.undoing || state.isGenerating
    val version = state.document.version
    Row(modifier.padding(horizontal = 16.dp), horizontalArrangement = Arrangement.spacedBy(8.dp)) {
        CapsuleButton(stringResource(R.string.feedback_undo), enabled = state.canUndo && !busy, onClick = { vm.undo(version) })
        if (state.canRedo) {
            CapsuleButton(stringResource(R.string.feedback_redo), enabled = !busy, onClick = { vm.redo(version) })
        }
    }
}

@Composable
private fun CapsuleButton(label: String, enabled: Boolean, onClick: () -> Unit) {
    Surface(
        onClick = onClick,
        enabled = enabled,
        shape = CircleShape,
        color = MaterialTheme.colorScheme.surface,
        border = BorderStroke(1.dp, MaterialTheme.colorScheme.outlineVariant),
        modifier = Modifier.padding(start = 8.dp),
    ) {
        Text(
            label,
            Modifier.padding(horizontal = 14.dp, vertical = 8.dp),
            style = MaterialTheme.typography.labelLarge,
            fontWeight = FontWeight.SemiBold,
            // 押せない時は押せないと分かるよう薄く
            color = MaterialTheme.colorScheme.onSurface.copy(alpha = if (enabled) 1f else 0.4f),
            maxLines = 1,
        )
    }
}

/** 下部のトースト(iOS の ToastHost)。3 秒で消え、下に払っても閉じる。TalkBack の時は長めに出す(H8)。 */
@Composable
private fun RuntimeToast(runtime: AppRuntime, message: UiText?) {
    val a11y = LocalAccessibilityManager.current
    val talkBack = rememberTalkBackRunning()
    if (message != null) {
        LaunchedEffect(message) {
            val recommended = a11y?.calculateRecommendedTimeoutMillis(TOAST_MILLIS, containsIcons = false, containsText = true, containsControls = false) ?: TOAST_MILLIS
            delay(maxOf(recommended, TalkBack.holdMillis(TOAST_MILLIS, talkBack)))
            if (runtime.toast == message) runtime.dismissToast()
        }
    }
    AnimatedVisibility(visible = message != null, enter = slideInVertically { it } + fadeIn(), exit = slideOutVertically { it } + fadeOut()) {
        val close = stringResource(R.string.feedback_close)
        ToastCapsule(onDismiss = runtime::dismissToast, dismissLabel = close) {
            Text(message?.resolve().orEmpty(), style = MaterialTheme.typography.bodySmall, fontWeight = FontWeight.Medium, textAlign = TextAlign.Center)
        }
    }
}

/** 「削除しました・元に戻す」(M11・iOS の `RecordUndoToast`)。時間が来たら閉じる(閉じた後に削除が送られる)。 */
@Composable
private fun RecordUndoToast(runtime: AppRuntime, runtimeState: RuntimeState) {
    val ticket = runtimeState.recordUndo
    val scope = rememberCoroutineScope()
    val talkBack = rememberTalkBackRunning()
    if (ticket != null) {
        LaunchedEffect(ticket.id) {
            // AppRuntime が削除を送るまでの時間と同じ(TalkBack の時は 10 秒)
            delay(TalkBack.recordUndoMillis(talkBack))
            if (runtime.state.value.recordUndo == ticket) runtime.dismissRecordUndo()
        }
    }
    AnimatedVisibility(visible = ticket != null, enter = slideInVertically { it } + fadeIn(), exit = slideOutVertically { it } + fadeOut()) {
        val undo = stringResource(R.string.feedback_undo)
        ToastCapsule(
            onDismiss = runtime::dismissRecordUndo,
            dismissLabel = stringResource(R.string.feedback_close),
            extraAction = undo to { scope.launch { runtime.undoRecordDelete() } },
        ) {
            Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(12.dp)) {
                Text(stringResource(R.string.feedback_deleted), style = MaterialTheme.typography.bodySmall, fontWeight = FontWeight.Medium)
                TextButton(onClick = { scope.launch { runtime.undoRecordDelete() } }) {
                    Text(undo, style = MaterialTheme.typography.bodySmall, fontWeight = FontWeight.SemiBold)
                }
            }
        }
    }
}

/** トーストのカプセル。下に払うと閉じる(iOS の `swipeToDismiss(edge: .bottom)`)。 */
@Composable
private fun ToastCapsule(
    onDismiss: () -> Unit,
    dismissLabel: String,
    extraAction: Pair<String, () -> Unit>? = null,
    content: @Composable () -> Unit,
) {
    var drag by remember { mutableFloatStateOf(0f) }
    val threshold = with(LocalDensity.current) { SWIPE_DISMISS_DISTANCE.toPx() }
    Surface(
        shape = CircleShape,
        tonalElevation = 6.dp,
        shadowElevation = 4.dp,
        border = BorderStroke(1.dp, MaterialTheme.colorScheme.outlineVariant),
        modifier = Modifier
            .widthIn(max = 480.dp)
            .offset { IntOffset(0, drag.roundToInt()) }
            .draggable(
                orientation = Orientation.Vertical,
                state = rememberDraggableState { delta -> drag = (drag + delta).coerceAtLeast(0f) },
                onDragStopped = {
                    if (drag > threshold) onDismiss()
                    drag = 0f
                },
            )
            .semantics {
                customActions = listOfNotNull(
                    extraAction?.let { (label, run) -> CustomAccessibilityAction(label) { run(); true } },
                    CustomAccessibilityAction(dismissLabel) { onDismiss(); true },
                )
            },
    ) {
        Box(Modifier.padding(horizontal = 16.dp, vertical = 10.dp), contentAlignment = Alignment.Center) { content() }
    }
}

/** 確認ダイアログ(iOS の `.alert(item: $rt.confirm)`)。主ボタンで先に進む・キャンセルで何もしない。 */
@Composable
private fun ConfirmDialog(confirm: Confirm, runtime: AppRuntime) {
    val scope = rememberCoroutineScope()
    AlertDialog(
        onDismissRequest = runtime::confirmDismissed,
        title = { Text(confirm.message, style = MaterialTheme.typography.titleMedium) },
        confirmButton = {
            TextButton(onClick = { scope.launch { runtime.confirmAccepted() } }) {
                Text(
                    stringResource(confirm.primaryLabel),
                    color = if (confirm.isDestructive) MaterialTheme.colorScheme.error else MaterialTheme.colorScheme.primary,
                    fontWeight = FontWeight.SemiBold,
                )
            }
        },
        dismissButton = {
            TextButton(onClick = runtime::confirmDismissed) { Text(stringResource(R.string.feedback_cancel)) }
        },
    )
}

private const val TOAST_MILLIS = 3_000L


/** マイク FAB(64dp + 余白 20dp)の上に出す(被せない) */
private val TOAST_BOTTOM_INSET = 100.dp
