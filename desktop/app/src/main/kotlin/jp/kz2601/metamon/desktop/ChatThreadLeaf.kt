package jp.kz2601.metamon.desktop

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.itemsIndexed
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.input.key.Key
import androidx.compose.ui.input.key.KeyEventType
import androidx.compose.ui.input.key.isShiftPressed
import androidx.compose.ui.input.key.key
import androidx.compose.ui.input.key.onPreviewKeyEvent
import androidx.compose.ui.input.key.type
import androidx.compose.ui.semantics.clearAndSetSemantics
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.unit.dp
import jp.kz2601.metamon.desktop.i18n.Strings
import jp.kz2601.metamon.desktop.store.SessionPersistence
import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.records.ChatThread
import jp.kz2601.metamon.dsl.records.JSONValue
import jp.kz2601.metamon.dsl.records.Record
import jp.kz2601.metamon.services.local.LocalRecord
import kotlinx.coroutines.delay
import java.time.Instant
import java.time.LocalDate
import java.time.ZoneId
import java.time.format.DateTimeFormatter
import java.util.Locale

// chatThread(#2214・iOS の ChatThreadLeaf・Android の components/social/ChatThreadLeaf.kt)。
// 吹き出し・送る・自分の判定は :dsl の ChatThread(iOS・Android と同じ規則)。
// リアルタイムの購読は #1752 で決めるので、共有の会話は開いている間だけ records/query を取り直す。

/** 会話の並び。時刻の無い行(端末から読んだ旧データ)は先頭に保存した順のまま、他は古い順(最新が一番下)。 */
object ChatRows {
    fun ordered(records: List<Record>): List<Record> =
        records.withIndex()
            .sortedWith(compareBy<IndexedValue<Record>, Instant?>(nullsFirst()) { ChatThread.date(it.value.createdAt) }.thenBy { it.index })
            .map { it.value }

    /** 今日なら「10:05」、それ以前は「10/24 10:05」(Android の ChatThreadSend.time と同じ)。 */
    fun time(createdAt: String?, zone: ZoneId = ZoneId.systemDefault(), today: LocalDate = LocalDate.now(zone)): String? {
        val at = ChatThread.date(createdAt) ?: return null
        val local = at.atZone(zone)
        val pattern = if (local.toLocalDate() == today) "H:mm" else "M/d H:mm"
        return local.format(DateTimeFormatter.ofPattern(pattern, Locale.ROOT))
    }

    /** 送るかどうか(文字が空なら送らない)。 */
    fun canSend(sending: Boolean, text: String): Boolean = !sending && text.isNotBlank()
}

/** 取り直しの間隔(ミリ秒)。 */
object ChatPolling {
    /** ウィンドウが前にある時。 */
    const val FOCUSED_MILLIS = 5_000L

    /** ウィンドウが後ろにある時(間隔を広げる)。 */
    const val BACKGROUND_MILLIS = 30_000L

    /** 続けて読めない時の上限。 */
    const val MAX_MILLIS = 120_000L

    /** 読めなかった回数 [failures] だけ倍にする(上限 [MAX_MILLIS])。 */
    fun interval(focused: Boolean, failures: Int): Long {
        val base = if (focused) FOCUSED_MILLIS else BACKGROUND_MILLIS
        val shift = failures.coerceIn(0, 6)
        return (base shl shift).coerceAtMost(MAX_MILLIS)
    }
}

/** サーバーの記録と、送れた自分の作成の id の対応(テストで作り物に替える)。 */
interface ChatRemote {
    /** records/query。読めなければ null か例外。 */
    suspend fun fetch(entity: String): List<Record>?

    /** 送れた自分の作成: サーバーの id → 画面の仮 id(`local-…`)。 */
    suspend fun aliases(entity: String): Map<String, String>
}

/** 本物: 端末の保存(SessionPersistence)の送り先から読む。 */
class PersistenceChatRemote(private val p: SessionPersistence) : ChatRemote {
    override suspend fun fetch(entity: String): List<Record>? = p.fetchRemote(entity)

    override suspend fun aliases(entity: String): Map<String, String> = p.sentAliases(entity)
}

object ChatSync {
    /**
     * 取り直した記録を画面の行に混ぜる。
     * - サーバーの行は、送れた自分の作成なら画面の仮 id に戻す(二重に出さない・開いている物が迷子にならない)
     * - まだ送れていない自分の行(仮 id で対応が無い)は残す
     * - 送った直後で対応がまだ付いていない自分の分(同じ中身)は、サーバーの側を出さない(次の取り直しで対応が付く)
     * - それ以外でサーバーに無い行は消えた(消された)ので出さない
     * [selfUid] はサインインしている人(分からなければ null)。自分が書いた行は author を外して「自分」にする。
     */
    fun merge(current: List<Row>, remote: List<Record>, aliases: Map<String, String>, selfUid: String?): List<Row> {
        val byId = current.associateBy { it.id }
        val sent = aliases.values.toSet()
        val unsent = current.filter { it.id.startsWith(LocalRecord.TEMP_PREFIX) && it.id !in sent }
        val waiting = unsent.map { it.data }.toMutableList()
        val fromServer = remote.mapNotNull { r ->
            val screenId = aliases[r.id] ?: r.id
            val data = r.data.mapValues { MockStore.cell(it.value) }
            val mayBeMine = selfUid == null || r.author == selfUid
            if (screenId == r.id && byId[r.id] == null && mayBeMine && waiting.remove(data)) return@mapNotNull null
            val mine = screenId != r.id || (selfUid != null && r.author == selfUid)
            Row(
                id = screenId,
                data = data,
                createdAt = r.createdAt ?: byId[screenId]?.createdAt,
                author = if (mine) byId[screenId]?.author else r.author,
                authorName = r.authorName,
            )
        }
        return fromServer + unsent.filter { u -> fromServer.none { it.id == u.id } }
    }

    /** 1 回取り直して [records] に混ぜる。読めたら true。 */
    suspend fun pull(remote: ChatRemote, records: SharedRecords, entity: String, selfUid: String?): Boolean {
        val rows = runCatching { remote.fetch(entity) }.getOrNull() ?: return false
        val aliases = runCatching { remote.aliases(entity) }.getOrDefault(emptyMap())
        records.mergeRemote(entity) { current -> merge(current, rows, aliases, selfUid) }
        return true
    }
}

/** chatThread が画面から受け取るもの。 */
class ChatEnv(
    /** 1 通足す(フォームの保存と同じ道)。 */
    val send: (entity: String, data: Map<String, JSONValue>) -> Unit,
    /** 共有の会話を 1 回取り直す。送り先が無い(モック)なら null で取り直さない。 */
    val poll: (suspend (entity: String) -> Boolean)? = null,
    /** このウィンドウが前にあるか(後ろなら間隔を広げる)。 */
    val isActive: () -> Boolean = { true },
)

@Composable
fun ChatThreadLeaf(c: Component, data: LeafData, env: ChatEnv) {
    val entity = data.entity(c.entity)
    val textField = c.textField
    if (entity == null || textField == null) {
        Text(Strings["renderer_unavailable_here"], style = MaterialTheme.typography.labelSmall)
        return
    }
    val rows = remember(data, entity.id) { ChatRows.ordered(data.records(entity.id)) }
    val poll = env.poll
    if (poll != null && entity.isShared) {
        // 開いている間だけ(この部品が画面から外れると止まる)
        LaunchedEffect(entity.id) {
            var failures = 0
            while (true) {
                failures = if (poll(entity.id)) 0 else failures + 1
                delay(ChatPolling.interval(env.isActive(), failures))
            }
        }
    }
    var draft by remember { mutableStateOf("") }
    fun send() {
        if (!ChatRows.canSend(false, draft)) return
        val message = ChatThread.message(draft, null, textField, null, ChatThread.defaultData(entity, listOfNotNull(textField, c.photoField))) ?: return
        env.send(entity.id, message)
        draft = ""
    }
    Column(Modifier.fillMaxWidth(), verticalArrangement = Arrangement.spacedBy(8.dp)) {
        if (rows.isEmpty()) {
            Text(
                c.emptyText?.takeIf { it.isNotEmpty() } ?: Strings["social_chat_empty"],
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                modifier = Modifier.heightIn(min = 120.dp).padding(16.dp),
            )
        } else {
            val list = rememberLazyListState()
            LaunchedEffect(rows.last().id, rows.size) { list.animateScrollToItem(rows.size - 1) }
            LazyColumn(Modifier.fillMaxWidth().heightIn(min = 160.dp, max = 560.dp), state = list, verticalArrangement = Arrangement.spacedBy(4.dp)) {
                itemsIndexed(rows, key = { _, r -> r.id }) { i, row ->
                    val mine = ChatThread.isMine(row, entity, data.uid)
                    val sender = if (ChatThread.showsSender(row, rows.getOrNull(i - 1), entity, data.uid)) row.authorName ?: Strings["social_anonymous"] else null
                    val photo = c.photoField?.let { row[it] as? JSONValue.Str }?.value?.takeIf { it.isNotEmpty() }
                    val text = (row[textField] as? JSONValue.Str)?.value.orEmpty().ifEmpty { if (photo != null) Strings["social_chat_photo"] else "" }
                    ChatBubble(text, ChatRows.time(row.createdAt), mine, sender)
                }
            }
        }
        Row(verticalAlignment = Alignment.Bottom, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            OutlinedTextField(
                value = draft,
                onValueChange = { draft = it },
                // Enter で送る・Shift+Enter で改行
                modifier = Modifier.weight(1f).onPreviewKeyEvent { e ->
                    if (e.type == KeyEventType.KeyDown && e.key == Key.Enter && !e.isShiftPressed) {
                        send()
                        true
                    } else {
                        false
                    }
                },
                placeholder = { Text(c.placeholder?.takeIf { it.isNotEmpty() } ?: Strings["social_chat_placeholder"]) },
                maxLines = 4,
            )
            TextButton(onClick = ::send, enabled = ChatRows.canSend(false, draft)) { Text(Strings["social_chat_send"]) }
        }
    }
}

/** 1 通の吹き出し。自分は右にブランド色・相手は左に面の色(上に送信者名)、下に送った時刻。 */
@Composable
private fun ChatBubble(text: String, time: String?, mine: Boolean, sender: String?) {
    val who = if (mine) Strings["social_chat_me"] else sender ?: Strings["social_chat_other"]
    val label = listOfNotNull(who, text, time).joinToString(", ")
    Row(Modifier.fillMaxWidth(), horizontalArrangement = if (mine) Arrangement.End else Arrangement.Start) {
        if (mine) Spacer(Modifier.size(56.dp))
        Column(
            horizontalAlignment = if (mine) Alignment.End else Alignment.Start,
            verticalArrangement = Arrangement.spacedBy(3.dp),
            modifier = Modifier.weight(1f, fill = false).clearAndSetSemantics { contentDescription = label },
        ) {
            if (sender != null) {
                Text(sender, Modifier.padding(horizontal = 6.dp), style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
            }
            val shape = RoundedCornerShape(18.dp)
            val bubble = if (mine) {
                Modifier.background(MaterialTheme.colorScheme.primary, shape)
            } else {
                Modifier.background(MaterialTheme.colorScheme.surfaceVariant, shape).border(1.dp, MaterialTheme.colorScheme.outlineVariant, shape)
            }
            Text(
                text,
                modifier = bubble.padding(horizontal = 14.dp, vertical = 9.dp).widthIn(max = 520.dp),
                color = if (mine) MaterialTheme.colorScheme.onPrimary else MaterialTheme.colorScheme.onSurface,
                style = MaterialTheme.typography.bodyLarge,
            )
            if (time != null) {
                Text(time, Modifier.padding(horizontal = 6.dp), style = MaterialTheme.typography.labelSmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
            }
        }
        if (!mine) Spacer(Modifier.size(56.dp))
    }
}
