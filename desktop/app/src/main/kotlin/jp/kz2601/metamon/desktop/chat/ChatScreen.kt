package jp.kz2601.metamon.desktop.chat

import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.FilterChip
import androidx.compose.material3.LinearProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.focus.FocusRequester
import androidx.compose.ui.focus.focusRequester
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.input.key.Key
import androidx.compose.ui.input.key.onPreviewKeyEvent
import androidx.compose.ui.text.TextRange
import androidx.compose.ui.text.input.TextFieldValue
import androidx.compose.ui.unit.dp
import jp.kz2601.metamon.desktop.HostOs
import jp.kz2601.metamon.desktop.KeyChord
import jp.kz2601.metamon.desktop.KeyPress
import jp.kz2601.metamon.desktop.Keymap
import jp.kz2601.metamon.desktop.i18n.Strings
import jp.kz2601.metamon.services.ChatTurn
import jp.kz2601.metamon.services.http.FailureKind
import kotlinx.coroutines.launch
import java.time.Instant
import java.time.ZoneId
import java.time.format.DateTimeFormatter
import java.time.format.FormatStyle

/** 会話の入力欄のキー(#2381)。Enter で送る・Shift+Enter で改行・Esc で閉じる。 */
enum class ChatKey(val key: String, val chord: KeyChord) {
    SEND("desktop_chat_keys_send", KeyChord(Key.Enter)),
    NEWLINE("desktop_chat_keys_newline", KeyChord(Key.Enter, shift = true)),
    CLOSE("desktop_chat_keys_close", KeyChord(Key.Escape)),
    ;

    val title: String get() = Strings[key]

    companion object {
        /** 押したキーの役目。Ctrl / ⌘ / Alt と組んだ Enter は何もしない(入力欄に任せる)。 */
        fun of(p: KeyPress): ChatKey? {
            if (p.ctrl || p.meta || p.alt) return null
            return when (p.key) {
                Key.Enter, Key.NumPadEnter -> if (p.shift) NEWLINE else SEND
                Key.Escape -> CLOSE.takeIf { !p.shift }
                else -> null
            }
        }

        /** F1 の一覧に出す (役目, 表記)。 */
        fun help(os: HostOs): List<Pair<String, String>> = entries.map { it.title to Keymap.label(it.chord, os) }
    }
}

/** 失敗の案内の文言(Android の VoiceErrorCopy.request と同じキー)。 */
data class ChatErrorCopy(val titleKey: String, val bodyKey: String, val retry: Boolean, val resetAt: Instant? = null) {
    val title: String get() = Strings[titleKey]

    /** 今日の上限は戻る時刻(分からなければ「明日」)を入れる。 */
    fun body(zone: ZoneId = ZoneId.systemDefault()): String {
        if (bodyKey != "voice_rate_body") return Strings[bodyKey]
        val whenText = resetAt?.let {
            DateTimeFormatter.ofLocalizedDateTime(FormatStyle.SHORT).withLocale(Strings.locale).withZone(zone).format(it)
        } ?: Strings["voice_tomorrow"]
        return Strings.format(bodyKey, whenText)
    }

    companion object {
        fun of(kind: FailureKind): ChatErrorCopy = when (kind) {
            FailureKind.Offline -> ChatErrorCopy("voice_offline_title", "voice_offline_body", retry = true)
            FailureKind.Timeout -> ChatErrorCopy("voice_timeout_title", "voice_timeout_body", retry = true)
            is FailureKind.Busy -> ChatErrorCopy("voice_busy_title", "voice_busy_body", retry = true)
            FailureKind.SessionExpired -> ChatErrorCopy("voice_session_title", "voice_session_body", retry = false)
            FailureKind.AppCheckFailed -> ChatErrorCopy("voice_chat_failed_title", "voice_app_check_body", retry = true)
            FailureKind.AIPaused -> ChatErrorCopy("voice_ai_paused_title", "voice_ai_paused_body", retry = false)
            is FailureKind.FreePlanPaused -> ChatErrorCopy("voice_free_paused_title", "voice_free_paused_body", retry = false)
            is FailureKind.ProRequiredKind -> ChatErrorCopy("voice_plan_limit_title", "voice_pro_required_body", retry = false)
            is FailureKind.PlanLimitReached -> ChatErrorCopy("voice_plan_limit_title", "voice_plan_limit_body", retry = false)
            FailureKind.AppStopped -> ChatErrorCopy("voice_app_stopped_title", "voice_app_stopped_body", retry = false)
            is FailureKind.RateLimited -> ChatErrorCopy("voice_rate_title", "voice_rate_body", retry = false, resetAt = kind.info.resetAt)
            is FailureKind.Conflict -> ChatErrorCopy("voice_chat_failed_title", "voice_conflict_body", retry = true)
            is FailureKind.Rejected -> ChatErrorCopy("voice_rejected_title", "voice_rejected_body", retry = false)
            FailureKind.Cancelled, is FailureKind.Other -> ChatErrorCopy("voice_chat_failed_title", "voice_other_body", retry = true)
        }
    }
}

/**
 * 会話の画面(#2381・#2162 の 2)。[ChatSession] を描く。作る([ChatBuild])は [onBuild] に渡す(作る流れは #2163)。
 * 吹き出し・質問のチップ(複数選択は「決定」)・案のカード(複数選択は「これでいく(N 件)」)・ready の「これでいく」・失敗の案内。
 */
@OptIn(ExperimentalLayoutApi::class)
@Composable
fun ChatScreen(session: ChatSession, onBuild: (ChatBuild) -> Unit, onClose: () -> Unit, modifier: Modifier = Modifier) {
    val state by session.state.collectAsState()
    val scope = rememberCoroutineScope()
    var input by remember { mutableStateOf(TextFieldValue("")) }
    val focus = remember { FocusRequester() }
    val list = rememberLazyListState()
    LaunchedEffect(Unit) { runCatching { focus.requestFocus() } }
    LaunchedEffect(state.bubbles.size) { if (state.bubbles.isNotEmpty()) list.animateScrollToItem(state.bubbles.lastIndex) }

    fun submit() {
        val text = input.text
        if (text.isBlank() || !state.canSend) return
        input = TextFieldValue("")
        scope.launch { session.send(text) }
    }

    Column(modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
        Row(Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
            Text(Strings["desktop_chat_new_app"], style = MaterialTheme.typography.titleMedium, modifier = Modifier.weight(1f))
            TextButton(onClick = { scope.launch { session.restart() } }, enabled = state.bubbles.isNotEmpty()) { Text(Strings["voice_restart"]) }
            TextButton(onClick = onClose) { Text(Strings["desktop_close"]) }
        }
        LazyColumn(Modifier.weight(1f).fillMaxWidth(), state = list, verticalArrangement = Arrangement.spacedBy(6.dp)) {
            items(state.bubbles) { b -> Bubble(b) }
        }
        if (state.pending) LinearProgressIndicator(Modifier.fillMaxWidth())
        state.error?.let { kind ->
            val copy = ChatErrorCopy.of(kind)
            Card(Modifier.fillMaxWidth()) {
                Column(Modifier.padding(12.dp), verticalArrangement = Arrangement.spacedBy(4.dp)) {
                    Text(copy.title, style = MaterialTheme.typography.titleSmall)
                    Text(copy.body())
                    Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                        if (copy.retry) Button(onClick = { scope.launch { session.retry() } }) { Text(Strings["voice_retry"]) }
                        TextButton(onClick = session::dismissError) { Text(Strings["desktop_close"]) }
                    }
                }
            }
        }
        if (state.suggestions.isNotEmpty() && state.kind != ChatTurn.Kind.PROPOSAL) {
            if (state.isMultiSelect) Text(Strings["voice_multi_select_hint"], style = MaterialTheme.typography.labelSmall, color = Color.Gray)
            FlowRow(horizontalArrangement = Arrangement.spacedBy(6.dp)) {
                state.suggestions.forEach { chip ->
                    if (state.isMultiSelect) {
                        FilterChip(selected = chip in state.selected, onClick = { session.toggle(chip) }, label = { Text(chip) })
                    } else {
                        OutlinedButton(onClick = { scope.launch { session.send(chip) } }) { Text(chip) }
                    }
                }
                if (state.isMultiSelect) {
                    Button(onClick = { scope.launch { session.submitSelection() } }, enabled = state.selected.isNotEmpty()) {
                        Text(Strings["voice_answer_selected"])
                    }
                }
            }
        }
        if (state.options.isNotEmpty()) {
            if (state.isMultiSelect) Text(Strings["voice_multi_select_hint"], style = MaterialTheme.typography.labelSmall, color = Color.Gray)
            state.options.forEach { o ->
                val on = o.optionId in state.selected
                Card(
                    Modifier.fillMaxWidth().clickable { if (state.isMultiSelect) session.toggle(o.optionId) },
                ) {
                    Row(Modifier.padding(12.dp), verticalAlignment = Alignment.CenterVertically) {
                        Column(Modifier.weight(1f)) {
                            Text(o.title, style = MaterialTheme.typography.titleSmall)
                            if (o.detail.isNotBlank()) Text(o.detail, style = MaterialTheme.typography.bodySmall)
                        }
                        if (state.isMultiSelect) {
                            FilterChip(selected = on, onClick = { session.toggle(o.optionId) }, label = { Text(if (on) "✓" else " ") })
                        } else {
                            Button(onClick = { session.choose(o)?.let(onBuild) }) { Text(Strings["voice_build_button"]) }
                        }
                    }
                }
            }
            if (state.isMultiSelect) {
                val n = state.options.count { it.optionId in state.selected }
                Button(onClick = { session.chooseSelected()?.let(onBuild) }, enabled = n > 0) { Text(Strings.format("voice_choose_selected", n)) }
            }
        }
        if (state.isReady) {
            Card(Modifier.fillMaxWidth()) {
                Row(Modifier.padding(12.dp), verticalAlignment = Alignment.CenterVertically) {
                    Text(Strings["voice_ready_title"], Modifier.weight(1f), style = MaterialTheme.typography.titleSmall)
                    Button(onClick = { session.build()?.let(onBuild) }) { Text(Strings["voice_build_button"]) }
                }
            }
        }
        Row(Modifier.fillMaxWidth(), verticalAlignment = Alignment.Bottom, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            OutlinedTextField(
                value = input,
                onValueChange = { input = it },
                placeholder = { Text(Strings["desktop_chat_placeholder"]) },
                maxLines = 6,
                modifier = Modifier.weight(1f).focusRequester(focus).onPreviewKeyEvent { e ->
                    when (KeyPress.of(e)?.let(ChatKey::of)) {
                        ChatKey.SEND -> true.also { submit() }
                        ChatKey.NEWLINE -> true.also { input = insertNewline(input) }
                        ChatKey.CLOSE -> true.also { onClose() }
                        null -> false
                    }
                },
            )
            Button(onClick = ::submit, enabled = state.canSend && input.text.isNotBlank()) { Text(Strings["voice_send"]) }
        }
    }
}

/** 選んでいる所を改行に置き換える(Shift+Enter)。 */
internal fun insertNewline(v: TextFieldValue): TextFieldValue {
    val start = minOf(v.selection.start, v.selection.end)
    val end = maxOf(v.selection.start, v.selection.end)
    val text = v.text.substring(0, start) + "\n" + v.text.substring(end)
    return TextFieldValue(text, TextRange(start + 1))
}

@Composable
private fun Bubble(b: ChatBubble) {
    Box(Modifier.fillMaxWidth(), contentAlignment = if (b.fromUser) Alignment.CenterEnd else Alignment.CenterStart) {
        Surface(
            color = if (b.fromUser) MaterialTheme.colorScheme.primaryContainer else MaterialTheme.colorScheme.surfaceVariant,
            shape = MaterialTheme.shapes.medium,
            modifier = Modifier.widthIn(max = 480.dp),
        ) {
            Text(b.text, Modifier.padding(horizontal = 12.dp, vertical = 8.dp))
        }
    }
}

/** 会話の画面を今のウィンドウの上に重ねる(コマンドパレットと同じ置き方)。 */
@Composable
fun ChatOverlay(session: ChatSession, onBuild: (ChatBuild) -> Unit, onClose: () -> Unit) {
    Box(Modifier.fillMaxSize().background(Color(0x66000000)), contentAlignment = Alignment.Center) {
        Surface(
            Modifier.widthIn(max = 640.dp).fillMaxWidth(0.92f).fillMaxHeight(0.9f),
            shape = MaterialTheme.shapes.large,
            tonalElevation = 8.dp,
        ) {
            ChatScreen(session, onBuild, onClose, Modifier.fillMaxSize())
        }
    }
}
