package jp.kz2601.metamon.voice.stage

import androidx.compose.animation.AnimatedContent
import androidx.compose.animation.Crossfade
import androidx.compose.animation.fadeIn
import androidx.compose.animation.fadeOut
import androidx.compose.animation.togetherWith
import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.interaction.MutableInteractionSource
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.LiveRegionMode
import androidx.compose.ui.semantics.liveRegion
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import jp.kz2601.metamon.R
import jp.kz2601.metamon.renderer.theme.LocalReduceMotion
import jp.kz2601.metamon.renderer.theme.RenderTheme
import jp.kz2601.metamon.renderer.theme.colors
import jp.kz2601.metamon.ui.CraneMascot
import jp.kz2601.metamon.ui.CranePose
import jp.kz2601.metamon.voice.stage.BuildChoreography.AssemblyPlan
import jp.kz2601.metamon.voice.stage.BuildChoreography.LiveDraft
import jp.kz2601.metamon.voice.stage.BuildChoreography.WaitingScript

/**
 * create(初期状態からの生成)の全画面「変身中」(#894・iOS の `GeneratingView`)。
 * 見出し = 依頼文、段階の文言、組み立て台。結果が届いたら([plan])本物の部品を上から置いて組み上げ、本物の画面に切り替える。
 * 組み上げ中は触ると飛ばして本物の画面へ([onSkip])。
 */
@Composable
fun GeneratingScreen(
    transcript: String,
    elapsedSeconds: Double,
    live: LiveDraft?,
    plan: AssemblyPlan?,
    onCancel: () -> Unit,
    onSkip: () -> Unit,
    modifier: Modifier = Modifier,
) {
    val script = remember(transcript) { WaitingScript(transcript) }
    val stage = GeneratingStage.of(elapsedSeconds)
    val warn = plan == null && stage.warns
    val message = stringResource(if (plan != null) R.string.voice_gen_building else stage.message(elapsedSeconds, edit = false))
    val colors = RenderTheme.colors
    Surface(
        modifier
            .fillMaxSize()
            .testTag("generating.create")
            // 組み上げ中は触ると飛ばす(待たせない)。待っている間は下の画面に触らせない
            .clickable(interactionSource = remember { MutableInteractionSource() }, indication = null) { if (plan != null) onSkip() },
        color = MaterialTheme.colorScheme.background,
    ) {
        Column(
            Modifier.fillMaxSize().padding(horizontal = 24.dp, vertical = 32.dp),
            horizontalAlignment = Alignment.CenterHorizontally,
            verticalArrangement = Arrangement.spacedBy(20.dp, Alignment.CenterVertically),
        ) {
            // 見出し: 作業中のツル(PC とスマホを数秒ごとに持ち替える)・バッジ・アプリ名・依頼文
            Column(horizontalAlignment = Alignment.CenterHorizontally, verticalArrangement = Arrangement.spacedBy(8.dp)) {
                val pose = if (LocalReduceMotion.current || (elapsedSeconds / 4).toInt() % 2 == 0) CranePose.PC else CranePose.PHONE
                Crossfade(pose, label = "crane") { CraneMascot(it, size = 96.dp, sway = 3f) }
                Text(
                    stringResource(R.string.voice_gen_badge).uppercase(),
                    style = MaterialTheme.typography.labelMedium,
                    fontWeight = FontWeight.SemiBold,
                    color = colors.brandA,
                )
                live?.appName?.let {
                    Text(it, style = MaterialTheme.typography.titleMedium, fontWeight = FontWeight.SemiBold, color = colors.brandA, maxLines = 1)
                }
                Text(
                    stringResource(R.string.voice_quoted, GeneratingStage.displayTranscript(transcript)),
                    style = MaterialTheme.typography.titleLarge,
                    fontWeight = FontWeight.SemiBold,
                    textAlign = TextAlign.Center,
                    maxLines = 3,
                    overflow = TextOverflow.Ellipsis,
                )
            }
            BuildStageView(script, elapsedSeconds, plan, live, Modifier.fillMaxWidth())
            // 文言・経過秒・キャンセル
            Column(horizontalAlignment = Alignment.CenterHorizontally, verticalArrangement = Arrangement.spacedBy(8.dp)) {
                Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    CircularProgressIndicator(Modifier.size(16.dp), strokeWidth = 2.dp, color = if (warn) colors.warning else colors.brandA)
                    AnimatedContent(message, transitionSpec = { fadeIn() togetherWith fadeOut() }, label = "message") {
                        Text(
                            it,
                            Modifier.semantics { liveRegion = LiveRegionMode.Polite },
                            style = MaterialTheme.typography.bodyMedium,
                            fontWeight = FontWeight.Medium,
                            color = if (warn) colors.warning else MaterialTheme.colorScheme.onSurfaceVariant,
                            textAlign = TextAlign.Center,
                        )
                    }
                }
                val seconds = stringResource(R.string.voice_gen_seconds, elapsedSeconds.toInt())
                val estimate = stage.estimate(edit = false)?.takeIf { plan == null }?.let { stringResource(it) }
                Text(
                    if (estimate != null) "$seconds · $estimate" else seconds,
                    style = MaterialTheme.typography.labelSmall,
                    color = colors.tertiaryLabel,
                )
                // 最初から小さく出し、30 秒を過ぎたら枠付きにする
                if (plan == null) {
                    if (stage.cancelIsProminent) {
                        OutlinedButton(onClick = onCancel) { Text(stringResource(R.string.voice_cancel), fontWeight = FontWeight.SemiBold) }
                    } else {
                        TextButton(onClick = onCancel, modifier = Modifier.heightIn(min = 48.dp)) {
                            Text(stringResource(R.string.voice_cancel), style = MaterialTheme.typography.bodySmall, color = colors.secondaryLabel)
                        }
                    }
                }
            }
        }
    }
}

/** edit(2 回目以降)の軽いインジケータ(iOS の `EditingIndicator`)。今の画面は薄くし、上部に小さなカプセルを出す。 */
@Composable
fun EditingIndicator(transcript: String, elapsedSeconds: Double, onStop: () -> Unit, modifier: Modifier = Modifier) {
    val stage = GeneratingStage.of(elapsedSeconds)
    val colors = RenderTheme.colors
    Surface(
        modifier.widthIn(max = 480.dp).padding(horizontal = 16.dp).testTag("generating.edit"),
        shape = CircleShape,
        tonalElevation = 6.dp,
        shadowElevation = 6.dp,
        border = BorderStroke(1.dp, colors.hairline),
    ) {
        Row(
            Modifier.padding(horizontal = 16.dp, vertical = 10.dp),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(10.dp),
        ) {
            if (stage.warns) {
                CircularProgressIndicator(Modifier.size(16.dp), strokeWidth = 2.dp, color = colors.warning)
            } else {
                EditShuffleGlyph(elapsedSeconds)
            }
            Column(Modifier.weight(1f, fill = false)) {
                Text(
                    stringResource(stage.message(elapsedSeconds, edit = true)),
                    Modifier.semantics { liveRegion = LiveRegionMode.Polite },
                    style = MaterialTheme.typography.bodySmall,
                    fontWeight = FontWeight.SemiBold,
                    color = if (stage.warns) colors.warning else MaterialTheme.colorScheme.onSurface,
                )
                Text(
                    stringResource(R.string.voice_quoted, transcript),
                    style = MaterialTheme.typography.labelSmall,
                    color = colors.secondaryLabel,
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis,
                )
            }
            // 最初から出す(言い間違いに気づいたらすぐ止められる)
            OutlinedButton(onClick = onStop) { Text(stringResource(R.string.voice_gen_stop), style = MaterialTheme.typography.labelMedium) }
        }
    }
}

/** 編集の待ち: カプセルの左に置く小さな部品の並べ直し。3 つの部品が順に入れ替わる。動きを減らす設定では回さない。 */
@Composable
private fun EditShuffleGlyph(elapsedSeconds: Double) {
    val step = if (LocalReduceMotion.current) 0 else (elapsedSeconds / 0.7).toInt()
    val colors = RenderTheme.colors
    Column(Modifier.size(20.dp), verticalArrangement = Arrangement.spacedBy(2.dp, Alignment.CenterVertically)) {
        (0 until 3).forEach { i ->
            // 部品ごとの帯の長さ(並べ直すと長さの順が入れ替わる)
            val width = SHUFFLE_WIDTHS[(step + i) % SHUFFLE_WIDTHS.size]
            Box(
                Modifier
                    .size(width = width.dp, height = 4.dp)
                    .background(if (i == 0) colors.brandA else colors.brandA.copy(alpha = 0.3f), CircleShape),
            )
        }
    }
}

private val SHUFFLE_WIDTHS = listOf(18, 12, 16, 10, 14)
