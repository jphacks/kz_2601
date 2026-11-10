package jp.kz2601.metamon.renderer.components.play

import android.widget.Toast
import androidx.compose.foundation.border
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateMapOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.hapticfeedback.HapticFeedbackType
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalHapticFeedback
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.semantics.stateDescription
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.expr.DSLClock
import jp.kz2601.metamon.dsl.logic.BracketDraw
import jp.kz2601.metamon.dsl.records.Entity
import jp.kz2601.metamon.dsl.records.FieldCoding
import jp.kz2601.metamon.dsl.records.FieldType
import jp.kz2601.metamon.dsl.records.JSONValue
import jp.kz2601.metamon.dsl.records.Record
import jp.kz2601.metamon.renderer.DSLRuntime
import jp.kz2601.metamon.renderer.R
import jp.kz2601.metamon.renderer.registry.RenderContext
import jp.kz2601.metamon.renderer.theme.RenderTheme
import jp.kz2601.metamon.renderer.theme.SymbolIcon
import jp.kz2601.metamon.renderer.theme.colors
import jp.kz2601.metamon.renderer.theme.renderCard
import kotlinx.coroutines.launch

/**
 * `bracket`(トーナメント表): entity の出場者から組み合わせを作り、回戦を列にして勝ち上がりを出す。
 * 試合の 1 人をタップするとその人を勝者にし(もう一度で取り消し)、winsField に保存する(楽観的 UI)。
 * 組み合わせと勝ち上がりの計算は :dsl の BracketDraw。iOS の BracketLeaf。
 */
@Composable
fun BracketLeaf(c: Component, ctx: RenderContext, modifier: Modifier = Modifier) {
    val all by rememberRecords(ctx.runtime)
    val pending = remember(c.id) { mutableStateMapOf<String, Int>() }
    val draw = BracketRules.draw(c, ctx.runtime, all, pending.toMap())
    val scope = rememberCoroutineScope()
    val haptics = LocalHapticFeedback.current
    val context = LocalContext.current
    val lockedText = stringResource(R.string.play_bracket_locked)
    val entityId = c.entity.orEmpty()
    val entity = ctx.runtime.entityModel(entityId)

    fun tap(m: BracketDraw.Match, side: BracketDraw.Side) {
        val winsField = BracketRules.winsField(c, entity) ?: return
        val writer = ctx.runtime as? RecordWriter ?: return
        when (val result = draw.tap(m, side, winsField, pending.toMap())) {
            BracketDraw.TapResult.NotPlayable -> return
            BracketDraw.TapResult.Locked -> {
                Toast.makeText(context, lockedText, Toast.LENGTH_SHORT).show()
                return
            }
            is BracketDraw.TapResult.Update -> {
                val records = PlayRecords.records(all, entityId)
                val started = BracketRules.startedField(c, entity)
                val starts = draw.startWrites(
                    result.wins, records, winsField, started, pending.toMap(),
                    now = FieldCoding.formatDatetime(DSLClock.now()),
                )
                val plan = BracketRules.writePlan(result.wins, starts, winsField, started)
                for ((id, _) in plan) {
                    val r = records.firstOrNull { it.id == id } ?: continue
                    if (!ctx.runtime.canEdit(entityId, r)) {
                        writer.notifyReadOnly(entityId)
                        return
                    }
                }
                pending.putAll(result.wins)
                haptics.performHapticFeedback(HapticFeedbackType.TextHandleMove)
                scope.launch {
                    // 下げる方(元の勝者)が先。途中で失敗したら残りは送らない
                    PlaySave.updateInOrder(writer, entityId, plan) { id ->
                        val w = result.wins[id]
                        if (w != null && pending[id] == w) pending.remove(id)
                    }
                }
            }
        }
    }

    Column(
        modifier.renderCard(),
        verticalArrangement = Arrangement.spacedBy(RenderTheme.Spacing.sm),
    ) {
        Row(Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
            Text(
                c.title.orEmpty(),
                Modifier.weight(1f).semantics { if (c.title?.isNotEmpty() == true) heading() },
                style = MaterialTheme.typography.titleSmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            draw.champion?.let { champ ->
                Row(horizontalArrangement = Arrangement.spacedBy(RenderTheme.Spacing.xs), verticalAlignment = Alignment.CenterVertically) {
                    SymbolIcon("trophy", 16.dp, RenderTheme.colors.brandA)
                    Text(
                        stringResource(R.string.play_bracket_champion, BracketRules.name(c, ctx.runtime, all, champ)),
                        style = MaterialTheme.typography.bodyMedium.copy(fontWeight = FontWeight.SemiBold),
                        color = RenderTheme.colors.brandA,
                    )
                }
            }
        }
        if (draw.rounds.isEmpty()) {
            if (draw.entrants.size == 1) {
                PlayNote("person.2", stringResource(R.string.play_bracket_need_two))
            } else {
                PlayNote("trophy", stringResource(R.string.play_bracket_add_entrants))
            }
        } else {
            Row(
                Modifier.horizontalScroll(rememberScrollState()),
                horizontalArrangement = Arrangement.spacedBy(RenderTheme.Spacing.sm),
            ) {
                draw.rounds.forEachIndexed { i, matches ->
                    RoundColumn(c, ctx.runtime, all, draw, matches, round = i + 1, onTap = ::tap)
                }
            }
        }
        if (draw.lateEntrants > 0) {
            Text(
                stringResource(R.string.play_bracket_late, draw.lateEntrants),
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
    }
}

@Composable
private fun RoundColumn(
    c: Component,
    runtime: DSLRuntime,
    all: Map<String, List<Record>>,
    draw: BracketDraw,
    matches: List<BracketDraw.Match>,
    round: Int,
    onTap: (BracketDraw.Match, BracketDraw.Side) -> Unit,
) {
    // 1 回戦の 1 試合の高さ。後の回戦は 2 倍ずつ(線で結ばずに、上がる先の試合が真ん中に来る)
    val slot = 76.dp * (1 shl (round - 1))
    val title = roundTitleText(round, draw.rounds.size)
    Column(Modifier.width(150.dp)) {
        Text(
            title,
            Modifier.height(20.dp).semantics { heading() },
            style = MaterialTheme.typography.labelMedium.copy(fontWeight = FontWeight.SemiBold),
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
        matches.forEach { m ->
            val label = stringResource(R.string.play_bracket_match, title, m.index + 1)
            Column(
                Modifier.height(slot).semantics { contentDescription = label },
                verticalArrangement = Arrangement.Center,
            ) {
                MatchCard(c, runtime, all, draw, m, onTap)
            }
        }
    }
}

@Composable
private fun MatchCard(
    c: Component,
    runtime: DSLRuntime,
    all: Map<String, List<Record>>,
    draw: BracketDraw,
    m: BracketDraw.Match,
    onTap: (BracketDraw.Match, BracketDraw.Side) -> Unit,
) {
    val colors = RenderTheme.colors
    val shape = RoundedCornerShape(10.dp)
    Column(
        Modifier
            .fillMaxWidth()
            .padding(vertical = RenderTheme.Spacing.xs)
            .background(colors.brandSoft.copy(alpha = colors.brandSoft.alpha * 0.5f), shape)
            .border(1.dp, colors.brandSoftStroke, shape),
    ) {
        SlotRow(c, runtime, all, draw, m, BracketDraw.Side.A, onTap)
        HorizontalDivider(color = colors.hairline)
        SlotRow(c, runtime, all, draw, m, BracketDraw.Side.B, onTap)
    }
}

@Composable
private fun SlotRow(
    c: Component,
    runtime: DSLRuntime,
    all: Map<String, List<Record>>,
    draw: BracketDraw,
    m: BracketDraw.Match,
    side: BracketDraw.Side,
    onTap: (BracketDraw.Match, BracketDraw.Side) -> Unit,
) {
    val r = if (side == BracketDraw.Side.A) m.a else m.b
    val won = m.winner == side
    val lost = m.winner != null && !won
    val text = slotText(c, runtime, all, m, side)
    val opponent = slotText(c, runtime, all, m, if (side == BracketDraw.Side.A) BracketDraw.Side.B else BracketDraw.Side.A)
    val locked = m.isPlayable && m.winner != null && draw.nextDecided(m)
    val spoken = stringResource(R.string.play_bracket_slot, text, opponent)
    val state = when {
        won -> stringResource(R.string.play_bracket_won)
        lost -> stringResource(R.string.play_bracket_lost)
        else -> ""
    }
    val hint = stringResource(R.string.play_bracket_make_winner)
    Row(
        Modifier
            .fillMaxWidth()
            .heightIn(min = 30.dp)
            .clickable(enabled = m.isPlayable, role = Role.Button, onClickLabel = if (locked) null else hint) { onTap(m, side) }
            .semantics(mergeDescendants = true) {
                contentDescription = spoken
                if (state.isNotEmpty()) stateDescription = state
            }
            .padding(horizontal = RenderTheme.Spacing.sm),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Text(
            text,
            Modifier.weight(1f),
            style = MaterialTheme.typography.bodyMedium.copy(fontWeight = if (won) FontWeight.Bold else FontWeight.Normal),
            color = if (r == null || lost) MaterialTheme.colorScheme.onSurfaceVariant else MaterialTheme.colorScheme.onSurface,
            maxLines = 2,
            overflow = TextOverflow.Ellipsis,
        )
        if (won) SymbolIcon("checkmark.circle", 16.dp, RenderTheme.colors.brandA)
    }
}

@Composable
private fun slotText(c: Component, runtime: DSLRuntime, all: Map<String, List<Record>>, m: BracketDraw.Match, side: BracketDraw.Side): String {
    val r = if (side == BracketDraw.Side.A) m.a else m.b
    return r?.let { BracketRules.name(c, runtime, all, it) }
        ?: stringResource(if (m.round == 1) R.string.play_bracket_bye else R.string.play_bracket_tbd)
}

@Composable
private fun roundTitleText(round: Int, total: Int): String = when (total - round) {
    0 -> stringResource(R.string.play_bracket_final)
    1 -> stringResource(R.string.play_bracket_semifinal)
    2 -> stringResource(R.string.play_bracket_quarterfinal)
    else -> stringResource(R.string.play_bracket_round, round)
}

/** トーナメント表の計算(描画と JVM テストで共用)。 */
object BracketRules {
    fun draw(c: Component, runtime: DSLRuntime, all: Map<String, List<Record>>, pending: Map<String, Int>): BracketDraw {
        val entity = runtime.entityModel(c.entity.orEmpty())
        return BracketDraw(
            PlayRecords.records(all, c.entity.orEmpty()),
            c.winsField.orEmpty(),
            c.seedField,
            startedField(c, entity),
            pending,
        )
    }

    /** 出場者の名前(titleField。無ければ id・空なら「—」)。 */
    fun name(c: Component, runtime: DSLRuntime, all: Map<String, List<Record>>, r: Record): String {
        val f = c.titleField?.takeIf { it.isNotEmpty() } ?: return r.id
        val entity = runtime.entityModel(c.entity.orEmpty())
        return PlayRecords.display(runtime, all, r[f], entity?.field(f)).ifEmpty { FieldCoding.UNAVAILABLE }
    }

    /** 勝ち数を保存できる field(integer・computed でない)。条件を満たさなければ null(表示だけ)。 */
    fun winsField(c: Component, entity: Entity?): String? {
        val id = c.winsField ?: return null
        val f = entity?.field(id) ?: return null
        return id.takeIf { !f.isComputed && f.type == FieldType.INTEGER }
    }

    /** 開始時刻を保存できる field(datetime・computed でない)。無ければ null(時刻の規則で固定)。 */
    fun startedField(c: Component, entity: Entity?): String? {
        val id = c.startedField ?: return null
        val f = entity?.field(id) ?: return null
        return id.takeIf { !f.isComputed && f.type == FieldType.DATETIME }
    }

    /** 保存の順: 勝ち数の小さい方(取り消し・元の勝者を戻す方)から。同じなら id 順。 */
    fun writeOrder(wins: Map<String, Int>): List<Pair<String, Int>> =
        wins.entries.sortedWith(compareBy<Map.Entry<String, Int>> { it.value }.thenBy { it.key }).map { it.key to it.value }

    /**
     * 保存する順と中身(recordId → data)。開始時刻を書く時は先に書いてから勝ち数、消す時は勝ち数を戻してから消す。
     * 同じ人の開始時刻は勝ち数と一緒に送る。
     */
    fun writePlan(
        wins: Map<String, Int>,
        starts: Map<String, JSONValue>,
        winsField: String,
        startedField: String?,
    ): List<Pair<String, Map<String, JSONValue>>> {
        val winWrites = writeOrder(wins).map { (id, w) ->
            val d = linkedMapOf<String, JSONValue>(winsField to JSONValue.Num(w.toDouble()))
            if (startedField != null) starts[id]?.let { d[startedField] = it }
            id to (d as Map<String, JSONValue>)
        }
        if (startedField == null || starts.isEmpty()) return winWrites
        val only = starts.keys.filter { it !in wins }.sorted().map { it to mapOf(startedField to starts.getValue(it)) }
        val setting = starts.values.any { !it.isNull }
        return if (setting) only + winWrites else winWrites + only
    }

    /** 決勝・準決勝・準々決勝、それより前は「n 回戦」(描画の木の値なので日本語)。 */
    fun roundTitle(round: Int, total: Int): String = when (total - round) {
        0 -> "決勝"
        1 -> "準決勝"
        2 -> "準々決勝"
        else -> "$round 回戦"
    }
}
