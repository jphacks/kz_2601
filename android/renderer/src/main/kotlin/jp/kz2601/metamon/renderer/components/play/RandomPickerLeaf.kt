package jp.kz2601.metamon.renderer.components.play

import android.provider.Settings
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Button
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.hapticfeedback.HapticFeedbackType
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalHapticFeedback
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.LiveRegionMode
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.liveRegion
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.logic.RandomPick
import jp.kz2601.metamon.dsl.records.FieldCoding
import jp.kz2601.metamon.dsl.records.JSONValue
import jp.kz2601.metamon.dsl.records.Record
import jp.kz2601.metamon.renderer.ActionContext
import jp.kz2601.metamon.renderer.DSLRuntime
import jp.kz2601.metamon.renderer.R
import jp.kz2601.metamon.renderer.components.lists.LeafData
import jp.kz2601.metamon.renderer.components.lists.ValueLabels
import jp.kz2601.metamon.renderer.components.lists.rememberValueLabels
import jp.kz2601.metamon.renderer.registry.RenderContext
import jp.kz2601.metamon.renderer.theme.RenderTheme
import jp.kz2601.metamon.renderer.theme.SymbolIcon
import jp.kz2601.metamon.renderer.theme.colors
import jp.kz2601.metamon.renderer.theme.renderCard
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch

/**
 * `randomPicker`(#263): ボタンで entity(+ filter)から 1 件をランダムに選び、titleField(+ subtitleField)を大きく出す。
 * 選んだ id は部品の状態にだけ持つ(保存しない)。表示はその id で今の記録を引き直すので、編集に追従し、消されたら未選択に戻る。
 * 演出は短いシャッフル(約 0.7 秒)。端末でアニメーションを切っていれば結果だけ出す。iOS の RandomPickerLeaf。
 */
@Composable
fun RandomPickerLeaf(c: Component, ctx: RenderContext, modifier: Modifier = Modifier) {
    val all by rememberRecords(ctx.runtime)
    val candidates = LeafData(ctx.runtime, all).filtered(c)
    var pickedId by rememberSaveable(c.id) { mutableStateOf<String?>(null) }
    var spinning by remember { mutableStateOf<Record?>(null) }
    var job by remember { mutableStateOf<Job?>(null) }
    val scope = rememberCoroutineScope()
    val haptics = LocalHapticFeedback.current
    val reduceMotion = rememberReduceMotion()
    val untitled = stringResource(R.string.play_random_untitled)
    val valueLabels = rememberValueLabels()
    val picked = pickedId?.let { id -> candidates.firstOrNull { it.id == id } }
    val shown = spinning ?: picked
    val isFinal = spinning == null

    Column(
        modifier.renderCard(),
        horizontalAlignment = Alignment.CenterHorizontally,
        verticalArrangement = Arrangement.spacedBy(RenderTheme.Spacing.md),
    ) {
        c.title?.takeIf { it.isNotEmpty() }?.let {
            Text(
                it,
                Modifier.fillMaxWidth().semantics { heading() },
                style = MaterialTheme.typography.titleSmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
        Box(Modifier.fillMaxWidth(), contentAlignment = Alignment.Center) {
            when {
                shown != null -> PickedResult(
                    title = randomPickerTitle(c, ctx.runtime, all, shown, valueLabels) ?: untitled,
                    subtitle = if (isFinal) randomPickerSubtitle(c, ctx.runtime, all, shown, valueLabels) else null,
                    isFinal = isFinal,
                    onOpen = if (isFinal && c.onItemTap != null) {
                        { c.onItemTap?.let { ctx.dispatch(it, ActionContext(ctx.route, ctx.form, recordId = shown.id)) } }
                    } else {
                        null
                    },
                )
                candidates.isEmpty() -> PlayNote("dice", stringResource(R.string.play_random_empty))
                else -> Column(horizontalAlignment = Alignment.CenterHorizontally, verticalArrangement = Arrangement.spacedBy(RenderTheme.Spacing.sm)) {
                    Box(
                        Modifier.size(60.dp).background(RenderTheme.colors.brandSoft, CircleShape),
                        contentAlignment = Alignment.Center,
                    ) { SymbolIcon("questionmark", 30.dp, RenderTheme.colors.brandA.copy(alpha = 0.6f)) }
                    Text(
                        stringResource(R.string.play_random_hint),
                        style = MaterialTheme.typography.bodyMedium,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                        textAlign = TextAlign.Center,
                    )
                }
            }
        }
        Button(
            onClick = {
                val winner = RandomPick.pick(candidates, pickedId) ?: return@Button
                job?.cancel()
                val frames = if (reduceMotion) emptyList() else RandomPick.reel(candidates, winner, frames = 9)
                job = scope.launch {
                    frames.forEachIndexed { i, f ->
                        spinning = f
                        delay(40L + i * 10L) // だんだん遅くなる
                    }
                    spinning = null
                    pickedId = winner.id
                    haptics.performHapticFeedback(HapticFeedbackType.LongPress)
                }
            },
            enabled = candidates.isNotEmpty() && spinning == null,
            modifier = Modifier.fillMaxWidth(),
            shape = RoundedCornerShape(RenderTheme.Radius.button),
        ) {
            Text(c.label?.takeIf { it.isNotEmpty() } ?: stringResource(R.string.play_random_default_label), fontWeight = FontWeight.SemiBold)
        }
    }
}

@Composable
private fun PickedResult(title: String, subtitle: String?, isFinal: Boolean, onOpen: (() -> Unit)?) {
    val open = stringResource(R.string.play_random_open)
    val base = Modifier.fillMaxWidth()
    val m = if (onOpen != null) base.clickable(role = Role.Button, onClickLabel = open, onClick = onOpen) else base
    Column(
        m.semantics { if (isFinal) liveRegion = LiveRegionMode.Polite },
        horizontalAlignment = Alignment.CenterHorizontally,
        verticalArrangement = Arrangement.spacedBy(RenderTheme.Spacing.xs),
    ) {
        Text(
            title,
            style = MaterialTheme.typography.displaySmall.copy(fontWeight = FontWeight.Bold),
            color = if (isFinal) RenderTheme.colors.brandA else MaterialTheme.colorScheme.onSurfaceVariant,
            textAlign = TextAlign.Center,
            maxLines = 2,
            overflow = TextOverflow.Ellipsis,
        )
        subtitle?.let {
            Text(it, style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant, maxLines = 1)
        }
    }
}

/** 見出し(titleField → entity の見出し)。空なら null(呼び側が「(無題)」)。 */
fun randomPickerTitle(c: Component, runtime: DSLRuntime, all: Map<String, List<Record>>, r: Record, labels: ValueLabels = ValueLabels.JAPANESE): String? {
    val entity = runtime.entityModel(c.entity.orEmpty())
    val f = c.titleField ?: entity?.displayTitleField ?: ""
    return LeafData(runtime, all, labels = labels).display(r[f], entity?.field(f)).ifEmpty { null }
}

/** 補足(subtitleField)。空・評価できない値は null。 */
fun randomPickerSubtitle(c: Component, runtime: DSLRuntime, all: Map<String, List<Record>>, r: Record, labels: ValueLabels = ValueLabels.JAPANESE): String? {
    val f = c.subtitleField?.takeIf { it.isNotEmpty() } ?: return null
    val entity = runtime.entityModel(c.entity.orEmpty())
    val s = LeafData(runtime, all, labels = labels).display(r[f] ?: JSONValue.Null, entity?.field(f))
    return s.takeUnless { it.isEmpty() || it == FieldCoding.UNAVAILABLE }
}

/** 端末で「アニメーションを削除」(アニメーション スケール 0)にしているか。iOS の「視差効果を減らす」。 */
@Composable
internal fun rememberReduceMotion(): Boolean {
    val context = LocalContext.current
    return remember(context) {
        runCatching { Settings.Global.getFloat(context.contentResolver, Settings.Global.ANIMATOR_DURATION_SCALE, 1f) == 0f }.getOrDefault(false)
    }
}
