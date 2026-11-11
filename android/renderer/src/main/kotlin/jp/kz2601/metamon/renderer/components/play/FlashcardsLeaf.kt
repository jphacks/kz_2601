package jp.kz2601.metamon.renderer.components.play

import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.gestures.detectHorizontalDragGestures
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.offset
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Button
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableFloatStateOf
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.rotate
import androidx.compose.ui.hapticfeedback.HapticFeedbackType
import androidx.compose.ui.input.pointer.pointerInput
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.platform.LocalHapticFeedback
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.clearAndSetSemantics
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.onClick
import androidx.compose.ui.semantics.role
import androidx.compose.ui.semantics.stateDescription
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.IntOffset
import androidx.compose.ui.unit.dp
import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.logic.FlashcardDeck
import jp.kz2601.metamon.dsl.records.Entity
import jp.kz2601.metamon.dsl.records.FieldType
import jp.kz2601.metamon.dsl.records.Record
import jp.kz2601.metamon.dsl.records.filters
import jp.kz2601.metamon.renderer.DSLRuntime
import jp.kz2601.metamon.renderer.R
import jp.kz2601.metamon.renderer.registry.RenderContext
import jp.kz2601.metamon.renderer.theme.RenderTheme
import jp.kz2601.metamon.renderer.theme.SymbolIcon
import jp.kz2601.metamon.renderer.theme.colors
import jp.kz2601.metamon.renderer.theme.renderCard
import kotlinx.coroutines.launch
import kotlin.math.abs
import kotlin.math.roundToInt

/**
 * `flashcards`(#264): entity の記録(+ filter)を 1 枚ずつ出す単語カード。タップで表(front)と裏(back)をめくる。
 * 右スワイプ = 覚えた・左 = まだ を toggle(boolean)へ保存して次へ(楽観的 UI)。ボタンでも答えられる(読み上げ・スイッチ操作向け)。
 * 進み具合は :dsl の FlashcardDeck を部品の状態に持つだけで保存しない。iOS の FlashcardsLeaf。
 */
@Composable
fun FlashcardsLeaf(c: Component, ctx: RenderContext, modifier: Modifier = Modifier) {
    val all by rememberRecords(ctx.runtime)
    val deck = remember(c.id) { FlashcardDeck() }
    // FlashcardDeck は Compose の状態でないので、変えるたびに数を進めて描き直す
    var version by remember { mutableIntStateOf(0) }
    val scope = rememberCoroutineScope()
    val haptics = LocalHapticFeedback.current
    val entityId = c.entity.orEmpty()
    val entity = ctx.runtime.entityModel(entityId)
    val cards = version.let { FlashcardRules.cards(c, ctx.runtime, all, deck.pending) }
    val card = deck.current(cards)

    fun answer(r: Record, learned: Boolean) {
        val writer = ctx.runtime as? RecordWriter
        if (c.toggle != null && !ctx.runtime.canEdit(entityId, r)) {
            writer?.notifyReadOnly(entityId)
            return
        }
        val toggle = if (writer != null) FlashcardRules.toggleField(c, entity) else null
        val patch = deck.answer(r, learned, toggle)
        version++
        haptics.performHapticFeedback(HapticFeedbackType.TextHandleMove)
        if (patch == null || writer == null) return
        scope.launch {
            writer.updateRecord(entityId, r.id, patch)
            deck.settle(r.id, learned)
            version++
        }
    }

    Column(
        modifier.renderCard(),
        horizontalAlignment = Alignment.CenterHorizontally,
        verticalArrangement = Arrangement.spacedBy(RenderTheme.Spacing.md),
    ) {
        when {
            card != null -> {
                val (position, total) = deck.progress(cards)
                val spoken = stringResource(R.string.play_flashcards_progress_spoken, position, total)
                Text(
                    stringResource(R.string.play_flashcards_progress, position, total),
                    Modifier.clearAndSetSemantics { contentDescription = spoken },
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
                CardFace(c, ctx.runtime, all, entity, card, deck.flipped, onFlip = { deck.flip(); version++ }, onAnswer = { answer(card, it) })
                Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.spacedBy(RenderTheme.Spacing.sm)) {
                    val shape = RoundedCornerShape(RenderTheme.Radius.button)
                    OutlinedButton({ answer(card, false) }, Modifier.weight(1f), shape = shape) {
                        Text(stringResource(R.string.play_flashcards_not_yet))
                    }
                    Button({ answer(card, true) }, Modifier.weight(1f), shape = shape) {
                        Text(stringResource(R.string.play_flashcards_learned), fontWeight = FontWeight.SemiBold)
                    }
                }
            }
            deck.isDone(cards) -> Column(
                horizontalAlignment = Alignment.CenterHorizontally,
                verticalArrangement = Arrangement.spacedBy(RenderTheme.Spacing.sm),
            ) {
                SymbolIcon("checkmark.circle", 40.dp, RenderTheme.colors.brandA)
                Text(stringResource(R.string.play_flashcards_done), style = MaterialTheme.typography.titleMedium)
                OutlinedButton({ deck.restart(); version++ }, shape = RoundedCornerShape(RenderTheme.Radius.button)) {
                    Text(stringResource(R.string.play_flashcards_again))
                }
            }
            else -> PlayNote(
                "rectangle.on.rectangle.angled",
                stringResource(if (FlashcardRules.learnedFilter(c) == false) R.string.play_flashcards_empty_unlearned else R.string.play_flashcards_empty),
            )
        }
    }
}

@Composable
private fun CardFace(
    c: Component,
    runtime: DSLRuntime,
    all: Map<String, List<Record>>,
    entity: Entity?,
    card: Record,
    flipped: Boolean,
    onFlip: () -> Unit,
    onAnswer: (Boolean) -> Unit,
) {
    val field = if (flipped) c.back else c.front
    val text = FlashcardRules.value(runtime, all, entity, card, field)
    val fieldName = field?.let { entity?.field(it)?.name }.orEmpty()
    var drag by remember(card.id) { mutableFloatStateOf(0f) }
    val threshold = with(LocalDensity.current) { 90.dp.toPx() }
    val side = stringResource(if (flipped) R.string.play_flashcards_back else R.string.play_flashcards_front)
    val flipLabel = stringResource(R.string.play_flashcards_flip)
    val learned = stringResource(R.string.play_flashcards_learned)
    val notYet = stringResource(R.string.play_flashcards_not_yet)
    val colors = RenderTheme.colors
    Box(
        Modifier
            .fillMaxWidth()
            .heightIn(min = 160.dp)
            .offset { IntOffset(drag.roundToInt(), 0) }
            .rotate(drag / 25f / 4f)
            .background(colors.brandSoft.copy(alpha = if (flipped) colors.brandSoft.alpha else colors.brandSoft.alpha * 0.5f), RoundedCornerShape(16.dp))
            .pointerInput(card.id) {
                detectHorizontalDragGestures(
                    onDragEnd = {
                        if (abs(drag) >= threshold) onAnswer(drag > 0)
                        drag = 0f
                    },
                    onDragCancel = { drag = 0f },
                ) { _, dx -> drag += dx }
            }
            .clickable(onClick = onFlip)
            .clearAndSetSemantics {
                contentDescription = text
                stateDescription = side
                role = Role.Button
                onClick(flipLabel) { onFlip(); true }
            }
            .padding(RenderTheme.Spacing.md),
        contentAlignment = Alignment.Center,
    ) {
        Column(horizontalAlignment = Alignment.CenterHorizontally, verticalArrangement = Arrangement.spacedBy(RenderTheme.Spacing.xs)) {
            Text(fieldName, style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
            Text(
                text,
                style = MaterialTheme.typography.headlineMedium.copy(fontWeight = FontWeight.Bold),
                color = if (flipped) colors.brandA else MaterialTheme.colorScheme.onSurface,
                textAlign = TextAlign.Center,
            )
        }
        if (abs(drag) > 20f) {
            Text(
                if (drag > 0) learned else notYet,
                Modifier
                    .align(Alignment.TopCenter)
                    .background(MaterialTheme.colorScheme.surface.copy(alpha = 0.85f), RoundedCornerShape(50))
                    .padding(horizontal = RenderTheme.Spacing.sm, vertical = RenderTheme.Spacing.xs),
                style = MaterialTheme.typography.titleSmall,
            )
        }
    }
}

/** 単語カードの山札の決まり(描画と JVM テストで共用)。 */
object FlashcardRules {
    /** toggle に使える field(boolean・computed でない)。条件を満たさなければ null(めくるだけ)。 */
    fun toggleField(c: Component, entity: Entity?): String? {
        val id = c.toggle ?: return null
        val f = entity?.field(id) ?: return null
        return id.takeIf { !f.isComputed && f.type == FieldType.BOOLEAN }
    }

    /** toggle の field を boolean で比べる filter が出す側(覚えた = true / まだ = false)。無ければ null。 */
    fun learnedFilter(c: Component): Boolean? =
        c.filters.firstNotNullOfOrNull { FlashcardDeck.learnedFilter(it, c.toggle) }

    /** 出題するカード。toggle を比べる filter は「値の無いカード = まだ」で評価するため、ほかの filter だけを当てる。 */
    fun cards(c: Component, runtime: DSLRuntime, all: Map<String, List<Record>>, pending: Map<String, Boolean>): List<Record> {
        val filters = c.filters
        val learned = learnedFilter(c)
        val rest = if (learned != null) filters.filter { FlashcardDeck.learnedFilter(it, c.toggle) == null } else filters
        return FlashcardDeck.cards(PlayRecords.filtered(runtime, all, c, rest), c.toggle, learned, pending)
    }

    /** 面に出す文字(空なら「—」)。 */
    fun value(runtime: DSLRuntime, all: Map<String, List<Record>>, entity: Entity?, r: Record, field: String?): String {
        if (field.isNullOrEmpty()) return ""
        return PlayRecords.display(runtime, all, r[field], entity?.field(field)).ifEmpty { "—" }
    }
}
