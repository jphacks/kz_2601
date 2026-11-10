package jp.kz2601.metamon.voice.stage

import android.os.SystemClock
import androidx.compose.animation.AnimatedVisibility
import androidx.compose.animation.core.RepeatMode
import androidx.compose.animation.core.animateFloat
import androidx.compose.animation.core.infiniteRepeatable
import androidx.compose.animation.core.rememberInfiniteTransition
import androidx.compose.animation.core.tween
import androidx.compose.animation.fadeIn
import androidx.compose.animation.fadeOut
import androidx.compose.animation.slideInVertically
import androidx.compose.animation.slideOutVertically
import androidx.compose.foundation.Canvas
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.BoxScope
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Button
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedCard
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.produceState
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.alpha
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.clearAndSetSemantics
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import jp.kz2601.metamon.R
import jp.kz2601.metamon.renderer.screen.LocalChangeHighlight
import jp.kz2601.metamon.renderer.theme.LocalReduceMotion
import jp.kz2601.metamon.renderer.theme.RenderTheme
import jp.kz2601.metamon.renderer.theme.colors
import jp.kz2601.metamon.ui.CraneMascot
import jp.kz2601.metamon.ui.CranePose
import jp.kz2601.metamon.voice.VoiceUiState
import kotlinx.coroutines.delay

/**
 * 生成中の演出と完成の後のお知らせをまとめて出す(#894)。本体の画面([content])を包み、上に重ねる。
 * - create: 全画面の組み立て台([GeneratingScreen])→ 結果の組み上げ → 本物の画面
 * - edit: 今の画面を薄くして上部にカプセル([EditingIndicator])→ 変わった部品の印(描画器の [LocalChangeHighlight])
 * - 完成の後: 要約の位置語のハイライト・最初のアプリの直後の「こう話すと直せます」(要約が閉じてから・[PostBuildNotice])
 * 要約のバナーそのものは FeedbackHost(#895)が出す。
 */
@Composable
fun BoxScope.BuildStageHost(
    state: VoiceUiState,
    onCancel: () -> Unit,
    onOpenVoice: () -> Unit,
    content: @Composable BoxScope.() -> Unit,
) {
    val context = LocalContext.current
    val stage = remember { BuildStageState(EditHintStore.of(context)) }
    val reduceMotion = LocalReduceMotion.current
    LaunchedEffect(state) { stage.update(state, SystemClock.elapsedRealtime(), reduceMotion) }

    val job = stage.job
    val elapsed by produceState(0.0, job?.startedAtMillis, stage.reveal) {
        val start = job?.startedAtMillis ?: stage.reveal?.job?.startedAtMillis ?: return@produceState
        while (true) {
            value = (SystemClock.elapsedRealtime() - start) / 1000.0
            delay(250)
        }
    }

    // 組み上げが終わったら本物の画面へ
    LaunchedEffect(stage.reveal) {
        val reveal = stage.reveal ?: return@LaunchedEffect
        delay((reveal.plan.total * 1000).toLong())
        stage.finishReveal(reveal.serial)
    }
    // 印は出現が済んだ頃に消す
    LaunchedEffect(stage.highlight) {
        val h = stage.highlight
        if (h.isEmpty) return@LaunchedEffect
        delay(HIGHLIGHT_MILLIS)
        stage.clearHighlight(h)
    }

    Box(Modifier.fillMaxSize().alpha(if (stage.showsEditing) EDITING_DIM else 1f)) {
        CompositionLocalProvider(LocalChangeHighlight provides stage.highlight) { content() }
    }

    val notice = stage.currentNotice(state)
    // 要約の位置語(「右下のボタンから」)を領域のハイライトに
    val anchor = stage.anchor
    if (anchor != null && notice == PostBuildNotice.SUMMARY && state.summary != null && !stage.showsFullScreen && !state.isInitial) {
        PositionHighlight(anchor, reduceMotion)
    }

    // 「こう話すと直せます」: 完成の切り替えが落ち着いてから(iOS の 0.8 秒)
    val hintEligible = notice == PostBuildNotice.EDIT_HINT && !state.isInitial && !state.isGenerating && !state.voiceOpen &&
        !stage.showsFullScreen
    var hintVisible by remember { mutableStateOf(false) }
    LaunchedEffect(hintEligible) {
        if (hintEligible) delay(EDIT_HINT_DELAY_MILLIS)
        hintVisible = hintEligible
    }
    AnimatedVisibility(
        visible = hintVisible,
        enter = slideInVertically { it } + fadeIn(),
        exit = slideOutVertically { it } + fadeOut(),
        modifier = Modifier.align(Alignment.BottomCenter).padding(horizontal = 16.dp).padding(bottom = FAB_CLEARANCE),
    ) {
        FirstEditHintCard(
            onClose = stage::dismissEditHint,
            onTalk = {
                stage.dismissEditHint()
                onOpenVoice()
            },
        )
    }

    AnimatedVisibility(
        visible = stage.showsEditing && state.isGenerating,
        enter = slideInVertically { -it } + fadeIn(),
        exit = slideOutVertically { -it } + fadeOut(),
        modifier = Modifier.align(Alignment.TopCenter).padding(top = 8.dp),
    ) {
        EditingIndicator(job?.transcript.orEmpty(), elapsed, onStop = onCancel)
    }

    // 消えていく間も最後のコマのまま(組み上げた画面が待ちの絵に戻らないように)
    val held = remember { HeldFrame() }
    if (stage.showsFullScreen) {
        held.transcript = (job ?: stage.reveal?.job)?.transcript.orEmpty()
        held.live = stage.live
        held.plan = stage.reveal?.plan
    }
    AnimatedVisibility(visible = stage.showsFullScreen, enter = fadeIn(), exit = fadeOut(tween(if (reduceMotion) 200 else 450))) {
        GeneratingScreen(
            transcript = held.transcript,
            elapsedSeconds = elapsed,
            live = held.live,
            plan = held.plan,
            onCancel = onCancel,
            onSkip = { stage.finishReveal() },
        )
    }
}

/** 全画面の生成中が消えていく間に見せ続ける中身 */
private class HeldFrame {
    var transcript: String = ""
    var live: BuildChoreography.LiveDraft? = null
    var plan: BuildChoreography.AssemblyPlan? = null
}

/** 要約の位置語の場所を、ゆっくり脈打つ光で示す(iOS の `PositionHighlight`)。動きを減らす設定では脈打たない。触れない・読み上げない。 */
@Composable
private fun PositionHighlight(anchor: PositionHint.Anchor, reduceMotion: Boolean) {
    val color = RenderTheme.colors.brandB
    val pulse = if (reduceMotion) {
        0.5f
    } else {
        val t = rememberInfiniteTransition(label = "hint")
        t.animateFloat(0f, 1f, infiniteRepeatable(tween(800), RepeatMode.Reverse), label = "pulse").value
    }
    Canvas(Modifier.fillMaxSize().clearAndSetSemantics {}) {
        val center = Offset(size.width * anchor.x, size.height * anchor.y)
        val radius = 120.dp.toPx() * (0.9f + 0.15f * pulse)
        drawCircle(
            Brush.radialGradient(listOf(color.copy(alpha = 0.35f + 0.25f * pulse), Color.Transparent), center = center, radius = radius),
            radius = radius,
            center = center,
        )
    }
}

/** 最初のアプリができた直後に 1 回だけ出す「こう話すと直せます」(iOS の `FirstEditHintCard`)。マイク FAB の上に出す。 */
@Composable
private fun FirstEditHintCard(onClose: () -> Unit, onTalk: () -> Unit) {
    OutlinedCard(Modifier.widthIn(max = 480.dp).fillMaxWidth().testTag("editHint.card"), shape = RoundedCornerShape(RenderTheme.Radius.card)) {
        Column(Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
            Row(horizontalArrangement = Arrangement.spacedBy(10.dp)) {
                CraneMascot(CranePose.POINT, size = 48.dp, sway = 4f)
                Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
                    Text(stringResource(R.string.feedback_edit_hint_title), style = MaterialTheme.typography.titleMedium, fontWeight = FontWeight.SemiBold)
                    Text(
                        stringResource(R.string.feedback_edit_hint_body),
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
            }
            Text(
                stringResource(R.string.feedback_edit_hint_examples),
                style = MaterialTheme.typography.bodyMedium,
                fontWeight = FontWeight.Medium,
                color = RenderTheme.colors.brandText,
            )
            Row(verticalAlignment = Alignment.CenterVertically) {
                TextButton(onClick = onClose, modifier = Modifier.testTag("editHint.close")) {
                    Text(stringResource(R.string.feedback_edit_hint_close), color = MaterialTheme.colorScheme.onSurfaceVariant)
                }
                Spacer(Modifier.weight(1f))
                Button(onClick = onTalk, modifier = Modifier.testTag("editHint.talk")) { Text(stringResource(R.string.feedback_edit_hint_talk)) }
            }
        }
    }
}

/** 編集の待ちの間、今の画面を薄くする */
private const val EDITING_DIM = 0.55f

/** 変わった部品の印を消すまで(描画器の光が 0.5 + 1.1 秒で消えた後) */
private const val HIGHLIGHT_MILLIS = 2_500L

private const val EDIT_HINT_DELAY_MILLIS = 800L

/** マイク FAB(64dp + 余白 20dp)の上に出す */
private val FAB_CLEARANCE = 100.dp
