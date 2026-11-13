package jp.kz2601.metamon.desktop

import jp.kz2601.metamon.desktop.i18n.Strings
import androidx.compose.foundation.Canvas
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.aspectRatio
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.geometry.Size
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.google.zxing.EncodeHintType
import com.google.zxing.WriterException
import com.google.zxing.qrcode.decoder.ErrorCorrectionLevel
import com.google.zxing.qrcode.encoder.Encoder
import jp.kz2601.metamon.dsl.Action
import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.ComponentMode
import jp.kz2601.metamon.dsl.ComponentType
import jp.kz2601.metamon.dsl.Notification
import jp.kz2601.metamon.dsl.expr.DSLClock
import jp.kz2601.metamon.dsl.expr.ExprEvaluator
import jp.kz2601.metamon.dsl.logic.BracketDraw
import jp.kz2601.metamon.dsl.logic.FlashcardDeck
import jp.kz2601.metamon.dsl.logic.RandomPick
import jp.kz2601.metamon.dsl.records.Entity
import jp.kz2601.metamon.dsl.records.FieldCoding
import jp.kz2601.metamon.dsl.records.FieldType
import jp.kz2601.metamon.dsl.records.HiddenFields
import jp.kz2601.metamon.dsl.records.JSONValue
import jp.kz2601.metamon.dsl.records.Record
import jp.kz2601.metamon.dsl.records.RecordPermission
import jp.kz2601.metamon.dsl.records.RecordQuery
import jp.kz2601.metamon.dsl.records.Settlement
import jp.kz2601.metamon.dsl.records.filters
import jp.kz2601.metamon.renderer.form.FormDraft
import jp.kz2601.metamon.renderer.navigation.Route
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonNull
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import java.awt.GraphicsEnvironment
import java.util.Timer
import java.util.TimerTask
import java.util.concurrent.ConcurrentHashMap
import kotlin.concurrent.schedule

// 計算・遊びの部品(#1745): settlement・bracket・flashcards・randomPicker・timerDisplay・qrCode。
// 規則は :dsl の Settlement・BracketDraw・FlashcardDeck・RandomPick(Android と同じソース)。
// Android の BracketRules・FlashcardRules・QrMatrix・QrContent は android.* を読むファイルにあるため、同じ規則をここに写す。

object PlayLeaves {
    val types: Set<ComponentType> = setOf(
        ComponentType.BRACKET,
        ComponentType.FLASHCARDS,
        ComponentType.QR_CODE,
        ComponentType.RANDOM_PICKER,
        ComponentType.SETTLEMENT,
        ComponentType.TIMER_DISPLAY,
    )

    /** 適合テストの props(iOS の RenderTree・Android の PlayNodes と同じキー)。[raw] は computed を足す前の保存した記録。 */
    fun props(c: Component, data: LeafData, raw: (String) -> List<Record>, form: FormDraft? = null): Map<String, JsonElement> {
        if (c.type !in types) return emptyMap()
        val p = LinkedHashMap<String, JsonElement>()
        c.mode?.let { p["mode"] = JsonPrimitive(it.wire) }
        c.seconds?.let { p["seconds"] = JsonPrimitive(it.toDouble()) }
        c.title?.let { p["title"] = JsonPrimitive(it) }
        c.label?.let { p["label"] = JsonPrimitive(it) }
        when (c.type) {
            ComponentType.SETTLEMENT -> {
                p["title"] = JsonPrimitive(c.title ?: "精算")
                p["transfers"] = JsonArray(
                    PlayRules.transfers(c, data).map {
                        JsonObject(mapOf("from" to JsonPrimitive(it.from), "to" to JsonPrimitive(it.to), "amount" to JsonPrimitive(it.amount)))
                    },
                )
            }
            ComponentType.FLASHCARDS -> {
                val entity = data.entity(c.entity)
                val toggle = PlayRules.toggleField(c, entity)
                val cards = PlayRules.cards(c, data, emptyMap())
                val (position, total) = FlashcardDeck().progress(cards)
                p["cards"] = JsonArray(
                    cards.map { r ->
                        JsonObject(
                            buildMap {
                                put("recordId", JsonPrimitive(r.id))
                                put("front", JsonPrimitive(PlayRules.display(data, entity, r, c.front)))
                                put("back", JsonPrimitive(PlayRules.display(data, entity, r, c.back)))
                                if (toggle != null) put("learned", JsonPrimitive(FlashcardDeck.isLearned(r, toggle)))
                            },
                        )
                    },
                )
                p["progress"] = JsonPrimitive("$position / $total")
            }
            ComponentType.BRACKET -> {
                val draw = PlayRules.draw(c, data, raw(c.entity.orEmpty()), emptyMap())
                val entity = data.entity(c.entity)
                fun entrant(r: Record?): JsonElement = r?.let {
                    JsonObject(mapOf("recordId" to JsonPrimitive(it.id), "title" to JsonPrimitive(PlayRules.display(data, entity, it, c.titleField))))
                } ?: JsonNull
                p["rounds"] = JsonArray(
                    draw.rounds.mapIndexed { i, matches ->
                        JsonObject(
                            mapOf(
                                "title" to JsonPrimitive(PlayRules.roundTitle(i + 1, draw.rounds.size)),
                                "matches" to JsonArray(
                                    matches.map { m ->
                                        JsonObject(
                                            buildMap {
                                                put("a", entrant(m.a))
                                                put("b", entrant(m.b))
                                                put("winner", m.winner?.let { JsonPrimitive(if (it == BracketDraw.Side.A) "a" else "b") } ?: JsonNull)
                                                if (m.bye) put("bye", JsonPrimitive(true))
                                            },
                                        )
                                    },
                                ),
                            ),
                        )
                    },
                )
                p["champion"] = entrant(draw.champion)
            }
            ComponentType.QR_CODE -> p["text"] = JsonPrimitive(QrContent.resolve(c, form, data.evaluator))
            else -> Unit
        }
        return p
    }
}

/** 計算・遊びの規則(描画と適合テストで共用)。Android の BracketRules・FlashcardRules・settlementTransfers・randomPicker* と同じ。 */
object PlayRules {
    /** computed を合成し hidden を落とした記録(filter なし)。 */
    fun visible(data: LeafData, entity: String): List<Record> =
        HiddenFields.redact(data.evaluator.viewRecords(entity), data.entity(entity))

    /** 表示用の文字列。field が無ければ空。 */
    fun display(data: LeafData, entity: Entity?, r: Record, field: String?): String {
        if (field.isNullOrEmpty()) return ""
        return FieldCoding.display(r[field], entity?.field(field), data.locale)
    }

    // MARK: settlement

    /** 名前は nameField(無ければ entity の見出し・"name")、残高は balanceField(既定 "balance")。 */
    fun transfers(c: Component, data: LeafData): List<Settlement.Transfer> {
        val entityId = c.entity.orEmpty()
        val entity = data.entity(entityId)
        val nameField = c.nameField ?: entity?.displayTitleField ?: "name"
        val balanceField = c.balanceField ?: "balance"
        val balances = visible(data, entityId).mapNotNull { r ->
            val b = r[balanceField].doubleValue ?: return@mapNotNull null
            display(data, entity, r, nameField) to b
        }
        return Settlement.transfers(balances)
    }

    // MARK: bracket

    fun draw(c: Component, data: LeafData, records: List<Record>, pending: Map<String, Int>): BracketDraw =
        BracketDraw(records, c.winsField.orEmpty(), c.seedField, startedField(c, data.entity(c.entity)), pending)

    /** 出場者の名前(titleField。無ければ id・空なら「—」)。 */
    fun entrantName(c: Component, data: LeafData, r: Record): String {
        val f = c.titleField?.takeIf { it.isNotEmpty() } ?: return r.id
        return display(data, data.entity(c.entity), r, f).ifEmpty { FieldCoding.UNAVAILABLE }
    }

    /** 勝ち数を保存できる field(integer・computed でない)。 */
    fun winsField(c: Component, entity: Entity?): String? {
        val id = c.winsField ?: return null
        val f = entity?.field(id) ?: return null
        return id.takeIf { !f.isComputed && f.type == FieldType.INTEGER }
    }

    /** 開始時刻を保存できる field(datetime・computed でない)。 */
    fun startedField(c: Component, entity: Entity?): String? {
        val id = c.startedField ?: return null
        val f = entity?.field(id) ?: return null
        return id.takeIf { !f.isComputed && f.type == FieldType.DATETIME }
    }

    /** 保存の順と中身。勝ち数の小さい方(取り消す方)から、開始時刻は付ける時は先・消す時は後。 */
    fun writePlan(wins: Map<String, Int>, starts: Map<String, JSONValue>, winsField: String, startedField: String?): List<Pair<String, Map<String, JSONValue>>> {
        val winWrites = wins.entries.sortedWith(compareBy<Map.Entry<String, Int>> { it.value }.thenBy { it.key }).map { (id, w) ->
            val d = linkedMapOf<String, JSONValue>(winsField to JSONValue.Num(w.toDouble()))
            if (startedField != null) starts[id]?.let { d[startedField] = it }
            id to (d as Map<String, JSONValue>)
        }
        if (startedField == null || starts.isEmpty()) return winWrites
        val only = starts.keys.filter { it !in wins }.sorted().map { it to mapOf(startedField to starts.getValue(it)) }
        return if (starts.values.any { !it.isNull }) only + winWrites else winWrites + only
    }

    /** 描画の木の値(iOS の日本語と突き合わせるので日本語のまま。Android の BracketRules と同じ)。画面は [PlayTexts.roundTitle]。 */
    fun roundTitle(round: Int, total: Int): String = when (total - round) {
        0 -> "決勝"
        1 -> "準決勝"
        2 -> "準々決勝"
        else -> "$round 回戦"
    }

    // MARK: flashcards

    /** toggle に使える field(boolean・computed でない)。 */
    fun toggleField(c: Component, entity: Entity?): String? {
        val id = c.toggle ?: return null
        val f = entity?.field(id) ?: return null
        return id.takeIf { !f.isComputed && f.type == FieldType.BOOLEAN }
    }

    /** 出題するカード。toggle を比べる filter は「値の無いカード = まだ」で評価するため、ほかの filter だけを当てる。 */
    fun cards(c: Component, data: LeafData, pending: Map<String, Boolean>): List<Record> {
        val filters = c.filters
        val learned = filters.firstNotNullOfOrNull { FlashcardDeck.learnedFilter(it, c.toggle) }
        val rest = if (learned != null) filters.filter { FlashcardDeck.learnedFilter(it, c.toggle) == null } else filters
        val rows = RecordQuery.filtered(c, data.entity(c.entity), data.evaluator, data.uid, fs = rest)
        return FlashcardDeck.cards(rows, c.toggle, learned, pending)
    }

    // MARK: randomPicker

    fun pickTitle(c: Component, data: LeafData, r: Record): String? {
        val entity = data.entity(c.entity)
        return display(data, entity, r, c.titleField ?: entity?.displayTitleField).ifEmpty { null }
    }

    fun pickSubtitle(c: Component, data: LeafData, r: Record): String? {
        val f = c.subtitleField?.takeIf { it.isNotEmpty() } ?: return null
        return display(data, data.entity(c.entity), r, f).takeUnless { it.isEmpty() || it == FieldCoding.UNAVAILABLE }
    }
}

/** 部品から記録を書き換える(差分を重ねる。null の値は消す)。変更できない人なら false。 */
fun DesktopSession.updateRecord(entity: String, id: String, patch: Map<String, JSONValue>): Boolean {
    val row = store.find(entity, id) ?: return false
    val record = records(entity).firstOrNull { it.id == id } ?: return false
    if (!RecordPermission.canEdit(record, entityModel(entity), uid)) {
        message = Strings["runtime_read_only"]
        return false
    }
    val next = row.data.toMutableMap()
    patch.forEach { (k, v) -> if (v.isNull) next.remove(k) else next[k] = MockStore.cell(v) }
    replaceStore(store.save(entity, id, next))
    return true
}

/** 部品を描く。[perform] は押した行の recordId 付きで画面のアクションを呼ぶ。 */
@Composable
fun PlayLeaf(c: Component, session: DesktopSession, route: Route, form: FormDraft?, data: LeafData, perform: (Action, String?) -> Unit) {
    when (c.type) {
        ComponentType.SETTLEMENT -> SettlementLeaf(c, data)
        ComponentType.BRACKET -> BracketLeaf(c, session, data)
        ComponentType.FLASHCARDS -> FlashcardsLeaf(c, session, data)
        ComponentType.RANDOM_PICKER -> RandomPickerLeaf(c, data, perform)
        ComponentType.TIMER_DISPLAY -> TimerDisplayLeaf(c, route, form)
        ComponentType.QR_CODE -> QrCodeLeaf(c, form, data)
        else -> Unit
    }
}

@Composable
private fun PlayCard(content: @Composable () -> Unit) {
    Card(Modifier.fillMaxWidth()) { Column(Modifier.padding(12.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) { content() } }
}

@Composable
private fun Note(text: String) = Text(text, color = Color.Gray, style = MaterialTheme.typography.bodyMedium)

/** 画面に出す遊びの部品の文言(Android と同じキー・#1759)。描画の木の値は [PlayRules] の日本語のまま。 */
object PlayTexts {
    fun roundTitle(round: Int, total: Int): String = when (total - round) {
        0 -> Strings["play_bracket_final"]
        1 -> Strings["play_bracket_semifinal"]
        2 -> Strings["play_bracket_quarterfinal"]
        else -> Strings.format("play_bracket_round", round)
    }

    /** 組み合わせがまだ無い時。1 人だけなら 2 人以上で表ができる、0 人なら追加を促す。 */
    fun bracketEmpty(entrants: Int): String = if (entrants == 1) Strings["play_bracket_need_two"] else Strings["play_bracket_add_entrants"]

    /** 相手のいない枠。1 回戦は不戦勝、それより後は未定。 */
    fun emptySlot(round: Int): String = if (round == 1) Strings["play_bracket_bye"] else Strings["play_bracket_tbd"]
}

// MARK: - settlement

@Composable
private fun SettlementLeaf(c: Component, data: LeafData) {
    val transfers = PlayRules.transfers(c, data)
    PlayCard {
        Text(c.title ?: Strings["play_settlement_title"], style = MaterialTheme.typography.titleMedium)
        HorizontalDivider()
        when {
            PlayRules.visible(data, c.entity.orEmpty()).isEmpty() -> Note(Strings["play_settlement_empty"])
            transfers.isEmpty() -> Note(Strings["play_settlement_none"])
            else -> transfers.forEach { t ->
                Row(Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    Text(t.from, maxLines = 1, overflow = TextOverflow.Ellipsis)
                    Text("→", color = Color.Gray)
                    Text(t.to, Modifier.weight(1f), maxLines = 1, overflow = TextOverflow.Ellipsis)
                    Text(FieldCoding.currency(t.amount, data.locale), fontWeight = FontWeight.SemiBold)
                }
            }
        }
    }
}

// MARK: - bracket

/** 試合の 1 人を押すとその人を勝者に(もう一度で取り消し)、winsField に保存する。 */
@Composable
private fun BracketLeaf(c: Component, session: DesktopSession, data: LeafData) {
    val entityId = c.entity.orEmpty()
    val entity = data.entity(entityId)
    val records = session.records(entityId)
    val draw = PlayRules.draw(c, data, records, emptyMap())

    fun tap(m: BracketDraw.Match, side: BracketDraw.Side) {
        val winsField = PlayRules.winsField(c, entity) ?: return
        when (val result = draw.tap(m, side, winsField)) {
            BracketDraw.TapResult.NotPlayable -> Unit
            BracketDraw.TapResult.Locked -> session.message = Strings["play_bracket_locked"]
            is BracketDraw.TapResult.Update -> {
                val started = PlayRules.startedField(c, entity)
                val starts = draw.startWrites(result.wins, records, winsField, started, now = FieldCoding.formatDatetime(DSLClock.now()))
                // 下げる方(元の勝者)が先。途中で失敗したら残りは送らない
                for ((id, patch) in PlayRules.writePlan(result.wins, starts, winsField, started)) {
                    if (!session.updateRecord(entityId, id, patch)) break
                }
            }
        }
    }

    PlayCard {
        Row(Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
            Text(c.title.orEmpty(), Modifier.weight(1f), style = MaterialTheme.typography.titleSmall, color = Color.Gray)
            draw.champion?.let { Text(Strings.format("play_bracket_champion", PlayRules.entrantName(c, data, it)), fontWeight = FontWeight.SemiBold, color = MaterialTheme.colorScheme.primary) }
        }
        if (draw.rounds.isEmpty()) {
            Note(PlayTexts.bracketEmpty(draw.entrants.size))
        } else {
            Row(Modifier.horizontalScroll(rememberScrollState()), horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                draw.rounds.forEachIndexed { i, matches ->
                    val round = i + 1
                    Column(Modifier.width(160.dp)) {
                        Text(PlayTexts.roundTitle(round, draw.rounds.size), Modifier.height(20.dp), style = MaterialTheme.typography.labelMedium, color = Color.Gray)
                        matches.forEach { m ->
                            // 後の回戦ほど 2 倍ずつ高くして、上がる先の試合が真ん中に来る
                            Column(Modifier.height(76.dp * (1 shl i)), verticalArrangement = Arrangement.Center) { MatchCard(c, data, m, ::tap) }
                        }
                    }
                }
            }
        }
        if (draw.lateEntrants > 0) Text(Strings.plural("play_bracket_late", draw.lateEntrants, draw.lateEntrants), style = MaterialTheme.typography.bodySmall, color = Color.Gray)
    }
}

@Composable
private fun MatchCard(c: Component, data: LeafData, m: BracketDraw.Match, onTap: (BracketDraw.Match, BracketDraw.Side) -> Unit) {
    val shape = RoundedCornerShape(10.dp)
    Column(Modifier.fillMaxWidth().padding(vertical = 4.dp).border(1.dp, Color(0x33000000), shape)) {
        listOf(BracketDraw.Side.A, BracketDraw.Side.B).forEachIndexed { i, side ->
            if (i > 0) HorizontalDivider()
            val r = if (side == BracketDraw.Side.A) m.a else m.b
            val won = m.winner == side
            val lost = m.winner != null && !won
            Row(
                Modifier.fillMaxWidth().heightIn(min = 30.dp).clickable(enabled = m.isPlayable) { onTap(m, side) }.padding(horizontal = 8.dp),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                Text(
                    r?.let { PlayRules.entrantName(c, data, it) } ?: PlayTexts.emptySlot(m.round),
                    Modifier.weight(1f),
                    fontWeight = if (won) FontWeight.Bold else FontWeight.Normal,
                    color = if (r == null || lost) Color.Gray else Color.Unspecified,
                    maxLines = 2,
                    overflow = TextOverflow.Ellipsis,
                )
                if (won) Text("✓", color = MaterialTheme.colorScheme.primary)
            }
        }
    }
}

// MARK: - flashcards

/** 表を見て押すと裏。「覚えた」「まだ」で次へ。toggle があれば保存する。進み具合は保存しない。 */
@Composable
private fun FlashcardsLeaf(c: Component, session: DesktopSession, data: LeafData) {
    val deck = remember(c.id) { FlashcardDeck() }
    var tick by remember { mutableIntStateOf(0) } // deck を変えたら描き直す
    val entityId = c.entity.orEmpty()
    val entity = data.entity(entityId)
    val toggle = PlayRules.toggleField(c, entity)
    val cards = tick.let { PlayRules.cards(c, data, deck.pending) }
    val current = deck.current(cards)
    val (position, total) = deck.progress(cards)

    fun answer(card: Record, learned: Boolean) {
        val patch = deck.answer(card, learned, toggle)
        if (patch != null) {
            session.updateRecord(entityId, card.id, patch)
            deck.settle(card.id, learned)
        }
        tick++
    }

    PlayCard {
        Row(Modifier.fillMaxWidth()) {
            Text(c.title.orEmpty(), Modifier.weight(1f), style = MaterialTheme.typography.titleSmall, color = Color.Gray)
            if (total > 0 && current != null) Text("$position / $total", color = Color.Gray)
        }
        when {
            cards.isEmpty() && !deck.isDone(cards) -> Note(Strings["play_flashcards_empty"])
            current == null -> {
                Note(Strings["play_flashcards_done"])
                OutlinedButton(onClick = { deck.restart(); tick++ }) { Text(Strings["play_flashcards_again"]) }
            }
            else -> {
                val face = if (deck.flipped) c.back else c.front
                Box(
                    Modifier.fillMaxWidth().heightIn(min = 140.dp).border(1.dp, Color(0x33000000), RoundedCornerShape(12.dp))
                        .clickable { deck.flip(); tick++ }.padding(16.dp),
                    contentAlignment = Alignment.Center,
                ) {
                    Text(
                        PlayRules.display(data, entity, current, face).ifEmpty { "—" },
                        style = MaterialTheme.typography.headlineSmall,
                        textAlign = TextAlign.Center,
                    )
                }
                Text(if (deck.flipped) Strings["desktop_flashcards_back_hint"] else Strings["desktop_flashcards_front_hint"], style = MaterialTheme.typography.bodySmall, color = Color.Gray)
                Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.spacedBy(12.dp)) {
                    OutlinedButton(onClick = { answer(current, false) }, Modifier.weight(1f)) { Text(Strings["play_flashcards_not_yet"]) }
                    Button(onClick = { answer(current, true) }, Modifier.weight(1f)) { Text(Strings["play_flashcards_learned"]) }
                }
            }
        }
    }
}

// MARK: - randomPicker

/** 押すと候補から 1 件を選ぶ(約 0.7 秒のシャッフル)。選んだ id は部品の状態だけに持ち、保存しない。 */
@Composable
private fun RandomPickerLeaf(c: Component, data: LeafData, perform: (Action, String?) -> Unit) {
    val candidates = data.filtered(c)
    var pickedId by remember(c.id) { mutableStateOf<String?>(null) }
    var spinning by remember { mutableStateOf<Record?>(null) }
    var job by remember { mutableStateOf<Job?>(null) }
    val scope = rememberCoroutineScope()
    val picked = pickedId?.let { id -> candidates.firstOrNull { it.id == id } }
    val shown = spinning ?: picked
    val isFinal = spinning == null

    PlayCard {
        c.title?.takeIf { it.isNotEmpty() }?.let { Text(it, style = MaterialTheme.typography.titleSmall, color = Color.Gray) }
        Box(Modifier.fillMaxWidth().heightIn(min = 72.dp), contentAlignment = Alignment.Center) {
            when {
                shown != null -> {
                    val tap = c.onItemTap
                    Column(
                        Modifier.fillMaxWidth().then(if (isFinal && tap != null) Modifier.clickable { perform(tap, shown.id) } else Modifier),
                        horizontalAlignment = Alignment.CenterHorizontally,
                    ) {
                        Text(
                            PlayRules.pickTitle(c, data, shown) ?: Strings["desktop_untitled"],
                            fontSize = 32.sp,
                            fontWeight = FontWeight.Bold,
                            color = if (isFinal) MaterialTheme.colorScheme.primary else Color.Gray,
                            textAlign = TextAlign.Center,
                            maxLines = 2,
                        )
                        if (isFinal) PlayRules.pickSubtitle(c, data, shown)?.let { Text(it, color = Color.Gray, maxLines = 1) }
                    }
                }
                candidates.isEmpty() -> Note(Strings["desktop_picker_empty"])
                else -> Note(Strings["desktop_picker_hint"])
            }
        }
        Button(
            onClick = {
                val winner = RandomPick.pick(candidates, pickedId) ?: return@Button
                job?.cancel()
                val frames = RandomPick.reel(candidates, winner, frames = 9)
                job = scope.launch {
                    frames.forEachIndexed { i, f ->
                        spinning = f
                        delay(40L + i * 10L) // だんだん遅くなる
                    }
                    spinning = null
                    pickedId = winner.id
                }
            },
            enabled = candidates.isNotEmpty() && spinning == null,
            modifier = Modifier.fillMaxWidth(),
        ) { Text(c.label?.takeIf { it.isNotEmpty() } ?: Strings["desktop_picker_pick"], fontWeight = FontWeight.SemiBold) }
    }
}

// MARK: - timerDisplay

/**
 * 走っている時間(iOS の `TimerRun`)。経過は開始時刻から都度出すので、画面を離れても正しい時間を出す。
 * @param base 一時停止までの経過秒 @param startedAtMillis 走行中なら開始(再開)時刻
 */
data class TimerRun(val base: Int = 0, val startedAtMillis: Long? = null) {
    val isRunning: Boolean get() = startedAtMillis != null

    fun elapsed(nowMillis: Long, total: Int? = null): Int {
        val raw = base + (startedAtMillis?.let { maxOf(((nowMillis - it) / 1000).toInt(), 0) } ?: 0)
        return total?.let { minOf(raw, it) } ?: raw
    }

    fun endsAtMillis(total: Int): Long? = startedAtMillis?.let { it + (total - base) * 1000L }

    companion object {
        /** 「05:03」、1 時間を超えたら「1:05:03」。 */
        fun format(seconds: Int): String {
            val h = seconds / 3600
            val m = (seconds % 3600) / 60
            val s = seconds % 60
            return if (h > 0) "%d:%02d:%02d".format(h, m, s) else "%02d:%02d".format(m, s)
        }
    }
}

/**
 * タイマーの走行状態(アプリが動いている間だけ)。画面を移っても止まらない。ウィンドウを閉じてもトレイに残っていれば止まらない(#1761)。
 * countdown の終わりは、その部品が画面に出ていなければトレイの通知で知らせる(Android の「前面で見えている間は出さない」と同じ)。
 * 走っている物はトレイのメニュー・ツールチップに出す([active])。
 */
object DesktopTimers {
    private val runs = ConcurrentHashMap<String, TimerRun>()
    private val meta = ConcurrentHashMap<String, Pair<String, Int?>>()
    private val visible = ConcurrentHashMap.newKeySet<String>()
    private val alarms = HashMap<String, TimerTask>()
    private val clock by lazy { Timer("timerDisplay", true) }

    fun get(key: String): TimerRun = runs[key] ?: TimerRun()

    @Synchronized
    fun set(key: String, run: TimerRun, name: String, countdownTotal: Int?) {
        runs[key] = run
        meta[key] = name to countdownTotal
        alarms.remove(key)?.cancel()
        val end = countdownTotal?.let { run.endsAtMillis(it) } ?: return
        val wait = end - System.currentTimeMillis()
        if (wait <= 0) return
        alarms[key] = clock.schedule(wait) {
            if (key !in visible && !GraphicsEnvironment.isHeadless()) {
                Notifier.Tray.schedule(Notification(id = "timer-$key", title = name, body = Strings["desktop_timer_done"]))
            }
        }
    }

    /** 走っている・途中で止めてあるタイマー(リセット済み・終わった countdown は出さない)。 */
    fun active(nowMillis: Long = System.currentTimeMillis()): List<TimerStatus> = runs.entries.sortedBy { it.key }.mapNotNull { (key, run) ->
        val (name, total) = meta[key] ?: return@mapNotNull null
        val elapsed = run.elapsed(nowMillis, total)
        if (!run.isRunning && run.base == 0) return@mapNotNull null
        if (total != null && elapsed >= total) return@mapNotNull null
        TimerStatus(key, name, if (total != null) total - elapsed else elapsed, total != null, run.isRunning)
    }

    /** テスト用: 全部消す。 */
    internal fun clear() {
        runs.clear()
        meta.clear()
        synchronized(this) { alarms.values.forEach { it.cancel() }; alarms.clear() }
    }

    fun visible(key: String, on: Boolean) {
        if (on) visible += key else visible -= key
    }
}

@Composable
private fun TimerDisplayLeaf(c: Component, route: Route, form: FormDraft?) {
    val isCountdown = c.mode == ComponentMode.COUNTDOWN
    val total = maxOf(c.seconds ?: 0, 0)
    val key = "${route.screen}/${c.id.orEmpty()}"
    val name = c.label?.takeIf { it.isNotEmpty() } ?: Strings["desktop_timer_name"]
    var run by remember(key) { mutableStateOf(DesktopTimers.get(key)) }
    var elapsed by remember(key) { mutableIntStateOf(run.elapsed(System.currentTimeMillis(), if (isCountdown) total else null)) }
    val shown = if (isCountdown) maxOf(total - elapsed, 0) else elapsed
    val finished = isCountdown && elapsed >= total

    fun update(next: TimerRun) {
        run = next
        DesktopTimers.set(key, next, name, if (isCountdown) total else null)
    }

    fun save() {
        val field = c.field ?: return
        form?.values?.set(field, JSONValue.Num(elapsed.toDouble()))
    }

    DisposableEffect(key) {
        DesktopTimers.visible(key, true)
        onDispose { DesktopTimers.visible(key, false) }
    }
    LaunchedEffect(run, key) {
        while (run.isRunning) {
            elapsed = run.elapsed(System.currentTimeMillis(), if (isCountdown) total else null)
            if (isCountdown && elapsed >= total) {
                update(TimerRun(base = total))
                save()
                break
            }
            delay(250)
        }
    }

    PlayCard {
        c.label?.takeIf { it.isNotEmpty() }?.let { Text(it, style = MaterialTheme.typography.labelMedium, color = Color.Gray) }
        Text(
            TimerRun.format(shown),
            Modifier.fillMaxWidth(),
            fontSize = 44.sp,
            fontWeight = FontWeight.SemiBold,
            textAlign = TextAlign.Center,
            color = if (finished) Color(0xFFE5484D) else MaterialTheme.colorScheme.primary,
        )
        Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.spacedBy(12.dp)) {
            Button(
                onClick = {
                    val now = System.currentTimeMillis()
                    if (run.isRunning) {
                        elapsed = run.elapsed(now, if (isCountdown) total else null)
                        update(TimerRun(base = elapsed))
                        save()
                    } else {
                        update(TimerRun(base = elapsed, startedAtMillis = now))
                    }
                },
                enabled = !finished,
                modifier = Modifier.weight(1f),
            ) {
                Text(
                    when {
                        run.isRunning -> Strings["desktop_timer_pause"]
                        elapsed > 0 && !finished -> Strings["desktop_timer_resume"]
                        else -> Strings["desktop_timer_start"]
                    },
                )
            }
            OutlinedButton(
                onClick = {
                    update(TimerRun())
                    elapsed = 0
                },
                enabled = elapsed > 0 || run.isRunning,
                modifier = Modifier.weight(1f),
            ) { Text(Strings["desktop_timer_reset"]) }
        }
    }
}

// MARK: - qrCode

/** QR のモジュール(iOS の CoreImage と同じ誤り訂正 M・周りに 4 モジュールの余白)。[size] は余白込みの 1 辺。 */
class QrMatrix private constructor(val size: Int, private val dark: BooleanArray) {
    operator fun get(x: Int, y: Int): Boolean = dark[y * size + x]

    companion object {
        const val QUIET_ZONE = 4

        /** 空・容量超えは null。ASCII だけなら ECI を付けない。 */
        fun encode(text: String): QrMatrix? {
            if (text.isEmpty()) return null
            val hints = if (text.all { it.code < 0x80 }) emptyMap() else mapOf(EncodeHintType.CHARACTER_SET to "UTF-8")
            val code = try {
                Encoder.encode(text, ErrorCorrectionLevel.M, hints)
            } catch (_: WriterException) {
                return null
            }
            val m = code.matrix
            val size = m.width + QUIET_ZONE * 2
            val dark = BooleanArray(size * size)
            for (y in 0 until m.height) for (x in 0 until m.width) {
                if (m.get(x, y).toInt() == 1) dark[(y + QUIET_ZONE) * size + x + QUIET_ZONE] = true
            }
            return QrMatrix(size, dark)
        }
    }
}

/** QR の中身(iOS の `QRCodeContent.resolve`): text(`{{id}}` を下書きの値に)> field > value(式)。 */
object QrContent {
    fun resolve(c: Component, form: FormDraft?, evaluator: ExprEvaluator): String {
        val row = form?.let { ExprEvaluator.Row(it.entity, Record(it.recordId ?: "draft", it.values.toMap()), memoizable = false) }
        val text = c.text
        if (!text.isNullOrEmpty()) {
            if (form == null || !text.contains("{{")) return text
            var s: String = text
            for (f in form.entity.fields) {
                val token = "{{${f.id}}}"
                if (s.contains(token)) s = s.replace(token, fieldText(f.id, form, row, evaluator))
            }
            return s
        }
        val field = c.field
        if (!field.isNullOrEmpty()) return form?.let { fieldText(field, it, row, evaluator) }.orEmpty()
        val value = RecordQuery.expr(c.value) ?: return ""
        return evaluator.evaluate(value, row)?.text.orEmpty()
    }

    private fun fieldText(id: String, form: FormDraft, row: ExprEvaluator.Row?, evaluator: ExprEvaluator): String {
        val f = form.entity.field(id)
        if (f != null && f.isComputed && row != null) {
            val v = evaluator.fieldValue(id, row)
            return if (f.type == FieldType.DATETIME) v?.json(f)?.displayString.orEmpty() else v?.text.orEmpty()
        }
        return (form.values[id] ?: JSONValue.Null).displayString
    }
}

/** 白地に黒(ダークモードでも読み取りやすさ優先)。下に label。「画像をコピー」でクリップボードへ PNG 相当の画像を入れる。 */
@Composable
private fun QrCodeLeaf(c: Component, form: FormDraft?, data: LeafData) {
    val content = QrContent.resolve(c, form, data.evaluator)
    val matrix = remember(content) { QrMatrix.encode(content) }
    Column(Modifier.fillMaxWidth(), horizontalAlignment = Alignment.CenterHorizontally, verticalArrangement = Arrangement.spacedBy(8.dp)) {
        c.title?.takeIf { it.isNotEmpty() }?.let { Text(it, Modifier.fillMaxWidth(), style = MaterialTheme.typography.titleSmall, color = Color.Gray) }
        when {
            content.isBlank() -> Note(Strings["renderer_qr_empty"])
            matrix == null -> Note(Strings["renderer_qr_too_long"])
            else -> {
                QrImage(matrix, Modifier.widthIn(max = 320.dp).fillMaxWidth())
                c.label?.takeIf { it.isNotEmpty() }?.let { Text(it, style = MaterialTheme.typography.bodySmall, color = Color.Gray) }
                TextButton(onClick = { QrClipboard.copy(matrix) }) { Text(Strings["desktop_qr_copy_image"]) }
            }
        }
    }
}

/** QR の絵(白地に黒)。共有のリンク(share/ShareLink.kt)も同じ描き方。 */
@Composable
internal fun QrImage(matrix: QrMatrix, modifier: Modifier = Modifier) {
    Canvas(modifier.aspectRatio(1f).background(Color.White)) {
        val cell = size.width / matrix.size
        for (y in 0 until matrix.size) for (x in 0 until matrix.size) {
            if (matrix[x, y]) drawRect(Color.Black, Offset(x * cell, y * cell), Size(cell + 0.5f, cell + 0.5f))
        }
    }
}

internal object QrClipboard {
    fun copy(m: QrMatrix, scale: Int = 8) {
        if (GraphicsEnvironment.isHeadless()) return
        val px = m.size * scale
        val img = java.awt.image.BufferedImage(px, px, java.awt.image.BufferedImage.TYPE_INT_RGB)
        for (y in 0 until px) for (x in 0 until px) img.setRGB(x, y, if (m[x / scale, y / scale]) 0x000000 else 0xFFFFFF)
        val t = object : java.awt.datatransfer.Transferable {
            override fun getTransferDataFlavors() = arrayOf(java.awt.datatransfer.DataFlavor.imageFlavor)
            override fun isDataFlavorSupported(f: java.awt.datatransfer.DataFlavor) = f == java.awt.datatransfer.DataFlavor.imageFlavor
            override fun getTransferData(f: java.awt.datatransfer.DataFlavor): Any = img
        }
        runCatching { java.awt.Toolkit.getDefaultToolkit().systemClipboard.setContents(t, null) }
    }
}
