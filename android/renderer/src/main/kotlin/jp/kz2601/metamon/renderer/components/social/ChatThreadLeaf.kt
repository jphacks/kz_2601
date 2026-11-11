package jp.kz2601.metamon.renderer.components.social

import androidx.compose.foundation.ExperimentalFoundationApi
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.combinedClickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
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
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Send
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.clearAndSetSemantics
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.unit.dp
import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.records.ChatThread
import jp.kz2601.metamon.dsl.records.Entity
import jp.kz2601.metamon.dsl.records.JSONValue
import jp.kz2601.metamon.dsl.records.Record
import jp.kz2601.metamon.renderer.DSLRuntime
import jp.kz2601.metamon.renderer.R
import jp.kz2601.metamon.renderer.components.PhotoValue
import jp.kz2601.metamon.renderer.components.PhotoView
import jp.kz2601.metamon.renderer.components.lists.EmptyPlaceholder
import jp.kz2601.metamon.renderer.components.lists.rememberLeafData
import jp.kz2601.metamon.renderer.registry.RenderContext
import jp.kz2601.metamon.renderer.theme.RenderTheme
import kotlinx.coroutines.launch
import java.time.Instant
import java.time.LocalDate
import java.time.ZoneId
import java.time.format.DateTimeFormatter
import java.util.Locale

/**
 * `chatThread`(iOS の ChatThreadLeaf・#875)。吹き出しを古い順に上から下へ積み(最新が一番下)、自分は右・相手は左に送信者名付き。
 * 下の入力欄で送ると [DSLRuntime.addRecord] で `entity` に 1 件保存する(ローカル DB に即反映・同期キューが送る)。
 * 他の人のメッセージはホットリロード(#1064)の records の知らせで runtime が取り直し、[DSLRuntime.recordsFlow] から届く。
 * 共有アプリへの初めての投稿は [AuthorNameHost] が名前を聞く。他人の吹き出しの長押しで通報・ブロック([ModerationHost])。
 * 写真を添えて送るのは子 Issue(届いた写真は出す)。
 */
@Composable
fun ChatThreadLeaf(c: Component, ctx: RenderContext, modifier: Modifier = Modifier) {
    val rt = ctx.runtime
    val entity = c.entity?.let(rt::entityModel)
    val textField = c.textField
    if (entity == null || textField == null) {
        Text(stringResource(R.string.renderer_unavailable_here), modifier, style = MaterialTheme.typography.labelSmall)
        return
    }
    val scope = rememberCoroutineScope()
    val data = rememberLeafData(rt)
    val rows = remember(data, entity.id) { ChatThread.ordered(data.viewRecords(entity.id)) }
    val uid = rt.currentUid()

    var draft by remember { mutableStateOf("") }
    var sending by remember { mutableStateOf(false) }
    var asking by remember { mutableStateOf<ChatThreadSend.Pending?>(null) }

    LaunchedEffect(entity.id) { rt.loadRecords(setOf(entity.id)) }

    fun post(p: ChatThreadSend.Pending, name: ChatThreadSend.NameAnswer?) {
        sending = true
        scope.launch {
            val ok = ChatThreadSend.post(rt, entity.id, p.data, name)
            if (!ok && draft.isEmpty()) draft = p.text // 送れなかった文を入力欄に戻す
            sending = false
        }
    }

    fun send() {
        if (sending) return
        val text = draft
        when (val plan = ChatThreadSend.plan(rt, entity, text, textField, c.photoField)) {
            ChatThreadSend.Plan.Empty -> Unit
            is ChatThreadSend.Plan.AskName -> {
                draft = ""
                asking = plan.pending
            }
            is ChatThreadSend.Plan.Send -> {
                draft = ""
                post(plan.pending, null)
            }
        }
    }

    Column(modifier.fillMaxWidth(), verticalArrangement = Arrangement.spacedBy(RenderTheme.Spacing.sm)) {
        if (rows.isEmpty()) {
            EmptyPlaceholder(
                "bubble.left.and.bubble.right",
                c.emptyText?.takeIf { it.isNotEmpty() } ?: stringResource(R.string.social_chat_empty),
                Modifier.heightIn(min = 200.dp),
            )
        } else {
            val list = rememberLazyListState()
            val last = rows.last().id
            LaunchedEffect(last) { list.animateScrollToItem(rows.size - 1) }
            LazyColumn(
                Modifier.fillMaxWidth().heightIn(min = 200.dp, max = 560.dp).testTag("chatThread.messages"),
                state = list,
                verticalArrangement = Arrangement.spacedBy(RenderTheme.Spacing.xs),
            ) {
                itemsIndexed(rows, key = { _, r -> r.id }) { i, row ->
                    val mine = ChatThread.isMine(row, entity, uid)
                    val showSender = ChatThread.showsSender(row, rows.getOrNull(i - 1), entity, uid)
                    ChatBubble(
                        record = row,
                        text = row[textField].stringValue.orEmpty(),
                        photo = c.photoField?.let { row[it].stringValue },
                        mine = mine,
                        sender = if (showSender) row.authorName ?: stringResource(R.string.social_anonymous) else null,
                        moderation = (rt as? ModerationHost)?.takeIf { it.canReport(entity.id, row) },
                        entity = entity.id,
                    )
                }
            }
        }
        Row(verticalAlignment = Alignment.Bottom, horizontalArrangement = Arrangement.spacedBy(RenderTheme.Spacing.sm)) {
            OutlinedTextField(
                value = draft,
                onValueChange = { draft = it },
                modifier = Modifier.weight(1f).testTag("chatThread.input"),
                placeholder = { Text(c.placeholder?.takeIf { it.isNotEmpty() } ?: stringResource(R.string.social_chat_placeholder)) },
                maxLines = 4,
            )
            IconButton(onClick = ::send, enabled = !sending && draft.isNotBlank(), modifier = Modifier.testTag("chatThread.send")) {
                Icon(Icons.Filled.Send, contentDescription = stringResource(R.string.social_chat_send))
            }
        }
    }

    asking?.let { p ->
        AuthorNameDialog(
            onPost = { name ->
                asking = null
                post(p, ChatThreadSend.NameAnswer(name))
            },
            onDismiss = {
                // 閉じただけなら送らずに入力欄へ戻す(名前を聞いたことにもしない)
                asking = null
                if (draft.isEmpty()) draft = p.text
            },
        )
    }
}

/** 送るかどうか・名前を聞くか(iOS の ChatThreadLeaf.send と AppRuntime.postRecord)。描画から切り離して JVM で試す。 */
object ChatThreadSend {
    data class Pending(val text: String, val data: Map<String, JSONValue>)

    /** 名前の答え。[name] が null なら「匿名で投稿」。 */
    data class NameAnswer(val name: String?)

    sealed interface Plan {
        data object Empty : Plan
        data class AskName(val pending: Pending) : Plan
        data class Send(val pending: Pending) : Plan
    }

    fun plan(rt: DSLRuntime, entity: Entity, text: String, textField: String, photoField: String?, photo: String? = null): Plan {
        val skip = listOfNotNull(textField, photoField)
        val data = ChatThread.message(text, photo, textField, photoField, ChatThread.defaultData(entity, skip)) ?: return Plan.Empty
        val pending = Pending(text, data)
        return if ((rt as? AuthorNameHost)?.needsAuthorName(entity.id) == true) Plan.AskName(pending) else Plan.Send(pending)
    }

    /** 名前を聞いた後なら答えを先に残してから 1 件足す(名前は送る行に付く)。 */
    suspend fun post(rt: DSLRuntime, entity: String, data: Map<String, JSONValue>, answer: NameAnswer?): Boolean {
        if (answer != null) (rt as? AuthorNameHost)?.answerAuthorName(answer.name)
        return rt.addRecord(entity, data)
    }

    /** 今日なら「10:05」、それ以前は「10/24 10:05」。 */
    fun time(createdAt: String?, zone: ZoneId = ZoneId.systemDefault(), today: LocalDate = LocalDate.now(zone)): String? {
        val at: Instant = ChatThread.date(createdAt) ?: return null
        val local = at.atZone(zone)
        val pattern = if (local.toLocalDate() == today) "H:mm" else "M/d H:mm"
        return local.format(DateTimeFormatter.ofPattern(pattern, Locale.ROOT))
    }
}

/** 1 通の吹き出し。自分は右にブランド色・相手は左に面の色(上に送信者名)、下に送った時刻。相手の分は長押しで通報・ブロック。 */
@OptIn(ExperimentalFoundationApi::class)
@Composable
private fun ChatBubble(
    record: Record,
    text: String,
    photo: String?,
    mine: Boolean,
    sender: String?,
    moderation: ModerationHost?,
    entity: String,
) {
    var menu by remember { mutableStateOf(false) }
    val time = remember(record.createdAt) { ChatThreadSend.time(record.createdAt) }
    val hasPhoto = PhotoValue.hasPhoto(photo)
    val who = if (mine) stringResource(R.string.social_chat_me) else sender ?: stringResource(R.string.social_chat_other)
    val body = if (text.isEmpty() && hasPhoto) stringResource(R.string.social_chat_photo) else text
    val label = listOfNotNull(who, body, time).joinToString(", ")
    Row(Modifier.fillMaxWidth(), horizontalArrangement = if (mine) Arrangement.End else Arrangement.Start) {
        if (mine) Spacer(Modifier.size(56.dp))
        Column(
            horizontalAlignment = if (mine) Alignment.End else Alignment.Start,
            verticalArrangement = Arrangement.spacedBy(3.dp),
            modifier = Modifier.weight(1f, fill = false)
                .clearAndSetSemantics { contentDescription = label }
                .testTag(if (mine) "chatThread.message.mine" else "chatThread.message.other"),
        ) {
            if (sender != null) {
                Text(
                    sender,
                    Modifier.padding(horizontal = 6.dp).padding(top = RenderTheme.Spacing.xs),
                    style = MaterialTheme.typography.labelMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
            val shape = RoundedCornerShape(18.dp)
            val bubble = if (mine) {
                Modifier.background(MaterialTheme.colorScheme.primary, shape)
            } else {
                Modifier.background(MaterialTheme.colorScheme.surfaceVariant, shape).border(1.dp, MaterialTheme.colorScheme.outlineVariant, shape)
            }
            Box {
                Column(
                    bubble
                        .combinedClickable(enabled = moderation != null, onClick = {}, onLongClick = { menu = true })
                        .padding(horizontal = 14.dp, vertical = 9.dp)
                        .widthIn(max = 520.dp),
                    verticalArrangement = Arrangement.spacedBy(RenderTheme.Spacing.sm),
                ) {
                    if (hasPhoto) PhotoView(photo, 160.dp, Modifier.widthIn(max = 220.dp))
                    if (text.isNotEmpty()) {
                        Text(
                            text,
                            color = if (mine) MaterialTheme.colorScheme.onPrimary else MaterialTheme.colorScheme.onSurface,
                            style = MaterialTheme.typography.bodyLarge,
                        )
                    }
                }
                if (moderation != null) {
                    DropdownMenu(expanded = menu, onDismissRequest = { menu = false }) {
                        DropdownMenuItem(
                            text = { Text(stringResource(R.string.social_report)) },
                            onClick = {
                                menu = false
                                moderation.reportRecord(entity, record)
                            },
                        )
                        DropdownMenuItem(
                            text = { Text(stringResource(R.string.social_block), color = MaterialTheme.colorScheme.error) },
                            onClick = {
                                menu = false
                                moderation.blockAuthor(entity, record)
                            },
                        )
                    }
                }
            }
            if (time != null) {
                Text(
                    time,
                    Modifier.padding(horizontal = 6.dp),
                    style = MaterialTheme.typography.labelSmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
        }
        if (!mine) Spacer(Modifier.size(56.dp))
    }
}

/** 共有アプリへの初めての投稿で表示名を聞く(iOS の AuthorNamePromptModifier)。 */
@Composable
private fun AuthorNameDialog(onPost: (String?) -> Unit, onDismiss: () -> Unit) {
    var name by remember { mutableStateOf("") }
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text(stringResource(R.string.social_author_name_title)) },
        text = {
            Column(verticalArrangement = Arrangement.spacedBy(RenderTheme.Spacing.sm)) {
                Text(stringResource(R.string.social_author_name_message), style = MaterialTheme.typography.bodyMedium)
                OutlinedTextField(
                    value = name,
                    onValueChange = { name = it.take(AUTHOR_NAME_MAX) },
                    singleLine = true,
                    placeholder = { Text(stringResource(R.string.social_author_name_placeholder)) },
                    modifier = Modifier.fillMaxWidth().testTag("chatThread.authorName"),
                )
            }
        },
        confirmButton = {
            TextButton(onClick = { onPost(name.trim().ifEmpty { null }) }, enabled = name.isNotBlank()) {
                Text(stringResource(R.string.social_author_name_post))
            }
        },
        dismissButton = {
            TextButton(onClick = { onPost(null) }) { Text(stringResource(R.string.social_author_name_anonymous)) }
        },
    )
}

/** 名前の長さの上限(:services の AuthorProfile.MAX_LENGTH と同じ・iOS も 20)。 */
private const val AUTHOR_NAME_MAX = 20
