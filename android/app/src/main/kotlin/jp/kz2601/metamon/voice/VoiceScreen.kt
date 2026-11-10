package jp.kz2601.metamon.voice

import androidx.compose.foundation.Canvas
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.KeyboardActions
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Button
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.FilterChip
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedCard
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.SuggestionChip
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.geometry.CornerRadius
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.geometry.Size
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.drawscope.Stroke
import androidx.compose.ui.platform.LocalConfiguration
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.compose.LifecycleEventEffect
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import jp.kz2601.metamon.R
import jp.kz2601.metamon.permissions.PermissionCenter
import jp.kz2601.metamon.permissions.rememberPermissionRequest
import jp.kz2601.metamon.services.ChatOption
import jp.kz2601.metamon.services.ChatTurn
import jp.kz2601.metamon.services.permissions.AppPermission
import java.time.ZoneId
import java.time.format.DateTimeFormatter
import java.util.Calendar
import java.util.Locale

/** 音声の画面(iOS の `VoiceInputView`)。マイクの権限を確かめてから [VoiceViewModel] に渡す。 */
@Composable
fun VoiceRoute(vm: VoiceViewModel, modifier: Modifier = Modifier) {
    val state by vm.state.collectAsStateWithLifecycle()
    val context = LocalContext.current
    // 権限の窓口(#890)に寄せる。理由のダイアログは出さず、断られたら今までどおり状態カードで「設定を開く」
    val askMic = rememberPermissionRequest(AppPermission.MICROPHONE, explain = false) { granted ->
        if (granted) vm.onMicTap() else vm.onPermissionDenied()
    }
    val onMic = {
        if (state.phase == VoicePhase.LISTENING || !state.voiceAvailable) vm.onMicTap() else askMic()
    }
    // 裏に回ったら録音を捨てる(マイクが点いたまま残らないように)
    LifecycleEventEffect(Lifecycle.Event.ON_STOP) { vm.onScreenHidden() }
    VoiceScreen(
        state = state,
        onMic = onMic,
        vm = vm,
        onOpenSettings = { PermissionCenter.get(context).openSettings(context, AppPermission.MICROPHONE) },
        modifier = modifier,
    )
}

@Composable
fun VoiceScreen(
    state: VoiceUiState,
    onMic: () -> Unit,
    vm: VoiceViewModel,
    onOpenSettings: () -> Unit,
    modifier: Modifier = Modifier,
) {
    var typed by rememberSaveable { mutableStateOf("") }
    val send = { text: String ->
        vm.submit(text)
        typed = ""
    }
    Column(modifier.fillMaxSize().imePadding().padding(horizontal = 20.dp)) {
        TopBar(state, onRestart = vm::restartChat)
        if (state.isChatting) {
            ClarifyChat(state, vm, Modifier.weight(1f))
            ErrorCardSlot(state, vm, onOpenSettings)
        } else {
            Column(
                Modifier.weight(1f).fillMaxWidth().verticalScroll(rememberScrollState()),
                horizontalAlignment = Alignment.CenterHorizontally,
                verticalArrangement = Arrangement.spacedBy(16.dp),
            ) {
                Spacer(Modifier.height(16.dp))
                Heading(state)
                Spacer(Modifier.height(8.dp))
                MicButton(state, size = if (state.isInitial) 120.dp else 88.dp, onClick = onMic)
                Text(
                    statusText(state),
                    style = MaterialTheme.typography.bodyMedium,
                    color = if (state.phase == VoicePhase.LISTENING) MaterialTheme.colorScheme.error else MaterialTheme.colorScheme.onSurfaceVariant,
                    textAlign = TextAlign.Center,
                )
                if (!state.voiceAvailable) {
                    Text(
                        stringResource(R.string.voice_unavailable_hint),
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                        textAlign = TextAlign.Center,
                    )
                }
                TranscriptCard(state)
                ErrorCardSlot(state, vm, onOpenSettings)
            }
            if (state.phase.canStart && state.error == null) Examples(state, onPick = send)
        }
        Composer(
            text = typed,
            onText = { typed = it },
            placeholder = stringResource(
                when {
                    state.isChatting -> R.string.voice_reply_placeholder
                    state.voiceAvailable -> R.string.voice_placeholder_text
                    else -> R.string.voice_placeholder_text_first
                },
            ),
            enabled = !state.isGenerating && !state.chatPending,
            onSend = send,
            onMic = if (state.isChatting) onMic else null,
            state = state,
        )
    }
}

@Composable
private fun TopBar(state: VoiceUiState, onRestart: () -> Unit) {
    Row(Modifier.fillMaxWidth().padding(top = 8.dp, bottom = 4.dp), verticalAlignment = Alignment.CenterVertically) {
        if (state.isInitial) {
            Text(stringResource(R.string.app_name), style = MaterialTheme.typography.titleMedium, fontWeight = FontWeight.Bold)
        }
        Spacer(Modifier.weight(1f))
        if (state.isChatting) {
            TextButton(onClick = onRestart, enabled = !state.isGenerating) { Text(stringResource(R.string.voice_restart)) }
        }
    }
}

@Composable
private fun Heading(state: VoiceUiState) {
    val hour = remember { Calendar.getInstance().get(Calendar.HOUR_OF_DAY) }
    val title = if (state.isInitial) R.string.voice_title_first else R.string.voice_title_editing
    val subtitle = if (state.isInitial) {
        stringResource(R.string.voice_subtitle_first, stringResource(Greeting.timeOfDay(hour)))
    } else {
        stringResource(R.string.voice_subtitle_editing)
    }
    Column(horizontalAlignment = Alignment.CenterHorizontally, verticalArrangement = Arrangement.spacedBy(8.dp)) {
        Text(stringResource(title), style = MaterialTheme.typography.headlineSmall, fontWeight = FontWeight.Bold, textAlign = TextAlign.Center)
        Text(subtitle, style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant, textAlign = TextAlign.Center)
    }
}

@Composable
private fun statusText(state: VoiceUiState): String = stringResource(
    when (state.phase) {
        VoicePhase.LISTENING -> R.string.voice_status_listening
        VoicePhase.TRANSCRIBING -> R.string.voice_status_transcribing
        VoicePhase.SENDING, VoicePhase.GENERATING ->
            if (state.chatPending) R.string.voice_status_thinking_chat else R.string.voice_status_thinking_app
        else -> if (state.voiceAvailable) R.string.voice_status_tap_to_speak else R.string.voice_status_not_ready
    },
)

/** マイク(iOS の `MicButton`)。タップで始め、もう一度タップで止める。長押しの変身一覧は android-061。 */
@Composable
fun MicButton(
    state: VoiceUiState,
    size: Dp,
    onClick: () -> Unit,
    modifier: Modifier = Modifier,
    /** 読み上げの説明を差し替える(マイク FAB =「このアプリを変える」) */
    description: String? = null,
) {
    val listening = state.phase == VoicePhase.LISTENING
    val busy = state.isBusy
    val colors = MaterialTheme.colorScheme
    val background = when {
        listening -> colors.error
        busy || !state.voiceAvailable -> colors.surfaceVariant
        else -> colors.primary
    }
    val foreground = if (busy || !state.voiceAvailable) colors.onSurfaceVariant else colors.onPrimary
    val label = description ?: stringResource(
        when {
            listening -> R.string.voice_mic_a11y_listening
            busy -> R.string.voice_mic_a11y_transcribing
            !state.voiceAvailable -> R.string.voice_mic_a11y_not_ready
            else -> R.string.voice_mic_a11y_idle
        },
    )
    Box(
        modifier
            .size(size)
            .clip(CircleShape)
            .background(background)
            .clickable(enabled = listening || (state.phase.canStart && !state.chatPending), onClick = onClick)
            .semantics { contentDescription = label },
        contentAlignment = Alignment.Center,
    ) {
        if (state.phase == VoicePhase.TRANSCRIBING) {
            CircularProgressIndicator(Modifier.size(size * 0.4f), color = foreground, strokeWidth = 3.dp)
        } else {
            MicGlyph(foreground, listening, Modifier.size(size * 0.42f))
        }
    }
}

/** マイクの形(アイコンのライブラリを足さないよう手で描く)。聞いている間は四角(停止)。 */
@Composable
private fun MicGlyph(color: Color, stop: Boolean, modifier: Modifier = Modifier) {
    Canvas(modifier) {
        val w = size.width
        val h = size.height
        if (stop) {
            drawRoundRect(color, topLeft = Offset(w * 0.2f, h * 0.2f), size = Size(w * 0.6f, h * 0.6f), cornerRadius = CornerRadius(w * 0.1f))
            return@Canvas
        }
        val stroke = w * 0.09f
        drawRoundRect(color, topLeft = Offset(w * 0.32f, 0f), size = Size(w * 0.36f, h * 0.62f), cornerRadius = CornerRadius(w * 0.18f))
        drawArc(
            color,
            startAngle = 0f,
            sweepAngle = 180f,
            useCenter = false,
            topLeft = Offset(w * 0.18f, h * 0.22f),
            size = Size(w * 0.64f, h * 0.56f),
            style = Stroke(stroke),
        )
        drawLine(color, Offset(w / 2, h * 0.78f), Offset(w / 2, h * 0.95f), strokeWidth = stroke)
        drawLine(color, Offset(w * 0.32f, h * 0.96f), Offset(w * 0.68f, h * 0.96f), strokeWidth = stroke)
    }
}

/** 途中の聞き取り / 送った依頼文。 */
@Composable
private fun TranscriptCard(state: VoiceUiState) {
    val text = state.partial.ifEmpty { state.transcript }
    if (text.isEmpty()) return
    OutlinedCard(Modifier.fillMaxWidth()) {
        Text("「$text」", Modifier.padding(16.dp), style = MaterialTheme.typography.bodyLarge)
    }
}

@Composable
private fun ErrorCardSlot(state: VoiceUiState, vm: VoiceViewModel, onOpenSettings: () -> Unit) {
    val error = state.error ?: return
    ErrorCard(VoiceErrorCopy.of(error), onRetry = vm::retry, onDismiss = vm::dismissError, onOpenSettings = onOpenSettings)
}

/** 状態カード(iOS の `ErrorCard`)。 */
@Composable
fun ErrorCard(copy: ErrorCardCopy, onRetry: () -> Unit, onDismiss: () -> Unit, onOpenSettings: () -> Unit, modifier: Modifier = Modifier) {
    val english = LocalConfiguration.current.locales[0].language == "en"
    val body = if (copy.body == R.string.voice_rate_body) {
        val whenText = copy.resetAt?.let {
            DateTimeFormatter.ofPattern(if (english) "MMM d, H:mm" else "M月d日 H:mm", if (english) Locale.US else Locale.JAPAN)
                .withZone(ZoneId.systemDefault())
                .format(it)
        } ?: stringResource(R.string.voice_tomorrow)
        stringResource(copy.body, whenText)
    } else {
        stringResource(copy.body)
    }
    OutlinedCard(modifier.fillMaxWidth().padding(vertical = 8.dp)) {
        Column(Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
            Text(stringResource(copy.title), style = MaterialTheme.typography.titleSmall, fontWeight = FontWeight.Bold)
            Text(body, style = MaterialTheme.typography.bodyMedium)
            Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                when {
                    copy.openSettings -> Button(onClick = onOpenSettings) { Text(stringResource(R.string.voice_open_settings)) }
                    copy.retry -> Button(onClick = onRetry) { Text(stringResource(R.string.voice_retry)) }
                }
                TextButton(onClick = onDismiss) { Text(stringResource(R.string.voice_dismiss)) }
            }
        }
    }
}

/** 例文(#274)。初期画面はカテゴリ → 例、生成後は変更の例。タップで送る(iOS と同じ)。 */
@Composable
private fun Examples(state: VoiceUiState, onPick: (String) -> Unit) {
    var category by rememberSaveable { mutableStateOf(Greeting.createCategories.first().id) }
    val examples = if (state.isInitial) {
        Greeting.createCategories.first { it.id == category }.examples
    } else {
        Greeting.editExamples
    }
    Column(Modifier.fillMaxWidth().padding(bottom = 8.dp), verticalArrangement = Arrangement.spacedBy(4.dp)) {
        Text(
            stringResource(if (state.isInitial) R.string.voice_examples_label else R.string.voice_edit_examples_label),
            style = MaterialTheme.typography.labelMedium,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
        if (state.isInitial) {
            Row(Modifier.horizontalScroll(rememberScrollState()), horizontalArrangement = Arrangement.spacedBy(6.dp)) {
                Greeting.createCategories.forEach { c ->
                    FilterChip(selected = c.id == category, onClick = { category = c.id }, label = { Text(stringResource(c.title)) })
                }
            }
        }
        Row(Modifier.horizontalScroll(rememberScrollState()), horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            examples.forEach { res ->
                val text = stringResource(res)
                SuggestionChip(onClick = { onPick(text) }, label = { Text(text) })
            }
        }
    }
}

/** 下に固定する文字入力(iOS の `ChatComposer`)。対話中は小さいマイクも並べる。 */
@Composable
private fun Composer(
    text: String,
    onText: (String) -> Unit,
    placeholder: String,
    enabled: Boolean,
    onSend: (String) -> Unit,
    onMic: (() -> Unit)?,
    state: VoiceUiState,
) {
    Row(Modifier.fillMaxWidth().padding(vertical = 8.dp), verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
        OutlinedTextField(
            value = text,
            onValueChange = onText,
            modifier = Modifier.weight(1f),
            placeholder = { Text(placeholder) },
            enabled = enabled,
            maxLines = 4,
            keyboardOptions = KeyboardOptions(imeAction = ImeAction.Send),
            keyboardActions = KeyboardActions(onSend = { if (text.isNotBlank()) onSend(text) }),
        )
        if (text.isBlank() && onMic != null) {
            MicButton(state, size = 48.dp, onClick = onMic)
        } else {
            TextButton(onClick = { onSend(text) }, enabled = enabled && text.isNotBlank()) { Text(stringResource(R.string.voice_send)) }
        }
    }
}

// MARK: - 相談(iOS の `ClarifyChatView` の基本だけ)

@Composable
private fun ClarifyChat(state: VoiceUiState, vm: VoiceViewModel, modifier: Modifier = Modifier) {
    val list = rememberLazyListState()
    LaunchedEffect(state.chat.size, state.chatPending) {
        val last = state.chat.size + (if (state.chatPending) 1 else 0) + 1
        list.animateScrollToItem(last - 1)
    }
    LazyColumn(modifier.fillMaxWidth(), state = list, verticalArrangement = Arrangement.spacedBy(8.dp)) {
        items(state.chat) { Bubble(it) }
        if (state.chatPending) {
            item { Bubble(ChatBubble(fromUser = false, text = stringResource(R.string.voice_status_thinking_chat))) }
        }
        item { TurnActions(state, vm) }
    }
}

@Composable
private fun Bubble(bubble: ChatBubble) {
    val colors = MaterialTheme.colorScheme
    Box(Modifier.fillMaxWidth(), contentAlignment = if (bubble.fromUser) Alignment.CenterEnd else Alignment.CenterStart) {
        Surface(
            shape = RoundedCornerShape(16.dp),
            color = if (bubble.fromUser) colors.primaryContainer else colors.surfaceVariant,
            modifier = Modifier.widthIn(max = 320.dp),
        ) {
            Text(bubble.text, Modifier.padding(horizontal = 14.dp, vertical = 10.dp), style = MaterialTheme.typography.bodyMedium)
        }
    }
}

/** 最後の返事に応じた操作: 質問 = チップ + 「このまま作る」/ 案 = カード / ready = 「これでいく」。 */
@Composable
private fun TurnActions(state: VoiceUiState, vm: VoiceViewModel) {
    val turn = state.chatTurn ?: return
    if (state.chatPending || state.chat.lastOrNull()?.fromUser == true) return
    val busy = state.isGenerating
    Column(Modifier.fillMaxWidth().padding(vertical = 4.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
        when (turn.kind) {
            ChatTurn.Kind.QUESTION -> {
                turn.suggestions.orEmpty().takeIf { it.isNotEmpty() }?.let { chips ->
                    Row(Modifier.horizontalScroll(rememberScrollState()), horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                        chips.forEach { c -> SuggestionChip(onClick = { vm.answer(c) }, enabled = !busy, label = { Text(c) }) }
                    }
                }
                OutlinedButton(onClick = vm::buildNow, enabled = !busy) { Text(stringResource(R.string.voice_build_now_button)) }
            }
            ChatTurn.Kind.PROPOSAL -> turn.options.orEmpty().forEach { OptionCard(it, enabled = !busy) { vm.choose(it) } }
            ChatTurn.Kind.READY -> OutlinedCard(Modifier.fillMaxWidth()) {
                Column(Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
                    Text(stringResource(R.string.voice_ready_title), style = MaterialTheme.typography.titleSmall, fontWeight = FontWeight.Bold)
                    turn.brief?.let { b ->
                        if (b.name.isNotEmpty()) Text(b.name, style = MaterialTheme.typography.bodyLarge)
                        if (b.purpose.isNotEmpty()) Text(b.purpose, style = MaterialTheme.typography.bodySmall)
                    }
                    if (state.autoBuildPending) {
                        Text(stringResource(R.string.voice_auto_building), style = MaterialTheme.typography.bodySmall)
                    }
                    Button(onClick = vm::buildFromChat, enabled = !busy) { Text(stringResource(R.string.voice_build_button)) }
                }
            }
            ChatTurn.Kind.ANSWER -> Unit
        }
    }
}

@Composable
private fun OptionCard(option: ChatOption, enabled: Boolean, onChoose: () -> Unit) {
    OutlinedCard(Modifier.fillMaxWidth()) {
        Column(Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(6.dp)) {
            Text(option.title, style = MaterialTheme.typography.titleSmall, fontWeight = FontWeight.Bold)
            if (option.detail.isNotEmpty()) Text(option.detail, style = MaterialTheme.typography.bodySmall)
            Button(onClick = onChoose, enabled = enabled) { Text(stringResource(R.string.voice_build_button)) }
        }
    }
}
