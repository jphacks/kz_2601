package jp.kz2601.metamon.voice

import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.Canvas
import androidx.compose.foundation.background
import androidx.compose.foundation.ExperimentalFoundationApi
import androidx.compose.foundation.clickable
import androidx.compose.foundation.combinedClickable
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.lazy.itemsIndexed
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.selection.toggleable
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.KeyboardActions
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.Checkbox
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.FilterChip
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedCard
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.SuggestionChip
import androidx.compose.material3.Surface
import androidx.compose.material3.Switch
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
import androidx.compose.ui.draw.alpha
import androidx.compose.ui.draw.clip
import androidx.compose.ui.geometry.CornerRadius
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.geometry.Size
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.drawscope.Stroke
import androidx.compose.ui.platform.LocalConfiguration
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.semantics.stateDescription
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.compose.LifecycleEventEffect
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import jp.kz2601.metamon.R
import jp.kz2601.metamon.ReviewBuild
import jp.kz2601.metamon.permissions.PermissionCenter
import jp.kz2601.metamon.permissions.rememberPermissionRequest
import jp.kz2601.metamon.purchase.PaywallReason
import jp.kz2601.metamon.renderer.layout.imePaddingExcludingBelow
import jp.kz2601.metamon.runtime.UiText
import jp.kz2601.metamon.runtime.resolve
import jp.kz2601.metamon.services.ChatMode
import jp.kz2601.metamon.services.ChatOption
import jp.kz2601.metamon.services.ChatTurn
import jp.kz2601.metamon.services.RemainingToday
import jp.kz2601.metamon.services.permissions.AppPermission
import jp.kz2601.metamon.ui.CraneMascot
import kotlinx.coroutines.delay
import java.time.Instant
import java.time.ZoneId
import java.time.format.DateTimeFormatter
import java.util.Calendar
import java.util.Locale

/** 音声の画面(iOS の `VoiceInputView`)。マイクの権限を確かめてから [VoiceViewModel] に渡す。 */
@Composable
fun VoiceRoute(
    vm: VoiceViewModel,
    modifier: Modifier = Modifier,
    /** マイクの長押し = 変身一覧(iOS と同じ)。null なら長押ししない */
    onMicLongPress: (() -> Unit)? = null,
) {
    val state by vm.state.collectAsStateWithLifecycle()
    val context = LocalContext.current
    val returning = remember { LaunchVisits.isReturning(context) }
    // 権限の窓口(#890)に寄せる。理由のダイアログは出さず、断られたら今までどおり状態カードで「設定を開く」
    val askMic = rememberPermissionRequest(AppPermission.MICROPHONE, explain = false) { granted ->
        if (granted) vm.onMicTap() else vm.onPermissionDenied()
    }
    val onMic = {
        if (state.phase == VoicePhase.LISTENING || !state.voiceAvailable) vm.onMicTap() else askMic()
    }
    // 裏に回ったら録音を捨てる(マイクが点いたまま残らないように)
    LifecycleEventEffect(Lifecycle.Event.ON_STOP) {
        vm.onScreenHidden()
        vm.stopSpeech()
    }
    // 開いた時に「今日の残り」を読む(5 分は読み直さない)
    LaunchedEffect(Unit) { vm.refreshUsage() }
    VoiceScreen(
        state = state,
        onMic = onMic,
        onMicLongPress = onMicLongPress,
        launchIsReturning = returning,
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
    onMicLongPress: (() -> Unit)? = null,
    launchIsReturning: Boolean = false,
) {
    var typed by rememberSaveable { mutableStateOf("") }
    var showTips by rememberSaveable { mutableStateOf(false) }
    val send = { text: String ->
        vm.submit(text)
        typed = ""
    }
    val greeting = Greeting.copy(Greeting.context(state.homeEntry, state.isInitial, launchIsReturning))
    val hour = remember { Calendar.getInstance().get(Calendar.HOUR_OF_DAY) }
    val title = stringResource(greeting.title)
    val subtitle = if (greeting.subtitleHasHello) {
        stringResource(greeting.subtitle, stringResource(Greeting.timeOfDay(hour)))
    } else {
        stringResource(greeting.subtitle)
    }
    // #43 初期画面の挨拶を読み上げ(相談中・生成中・生成後のシートでは読まない・設定で OFF なら何もしない)
    LaunchedEffect(state.isInitial, state.showsChat, title) {
        delay(GREETING_SPEECH_DELAY_MILLIS)
        if (state.isInitial && !state.showsChat) vm.speakGreeting(title + "\n" + subtitle)
    }
    // 設定の「頼み方のコツ」で選んだ文(#1161)。入れるだけで送らない
    LaunchedEffect(state.draft) {
        state.draft?.let {
            typed = it
            vm.consumeDraft()
        }
    }
    if (showTips) {
        PromptTipsSheet(onDismiss = { showTips = false }) { text ->
            showTips = false
            typed = text
        }
    }
    Column(modifier.fillMaxSize().imePaddingExcludingBelow().padding(horizontal = 20.dp)) {
        TopBar(state, onRestart = vm::restartChat)
        if (state.restartConfirm) RestartConfirmDialog(vm)
        if (state.showsChat) {
            ClarifyChat(state, vm, Modifier.weight(1f))
            ErrorCardSlot(state, vm, onOpenSettings)
        } else {
            Column(
                Modifier.weight(1f).fillMaxWidth().verticalScroll(rememberScrollState()),
                horizontalAlignment = Alignment.CenterHorizontally,
                verticalArrangement = Arrangement.spacedBy(16.dp),
            ) {
                Spacer(Modifier.height(16.dp))
                // ツル(#1132・iOS と同じ 76dp・happy)。縦が狭い時は出さない
                val compactHeight = VoiceCrane.isCompactHeight(LocalConfiguration.current.screenHeightDp)
                VoiceCrane.pose(state, compactHeight)?.let { CraneMascot(it, size = 76.dp) }
                Heading(title, subtitle)
                Spacer(Modifier.height(8.dp))
                MicButton(state, size = if (state.isInitial) 120.dp else 88.dp, onClick = onMic, onLongClick = onMicLongPress)
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
            if (state.phase.canStart && state.error == null) Examples(state, onPick = send, onTips = { showTips = true })
        }
        RemainingTodayBadge(state)
        Composer(
            text = typed,
            onText = { typed = it },
            placeholder = stringResource(
                when {
                    state.showsChat && state.chatMode == ChatMode.EDIT -> R.string.voice_edit_reply_placeholder
                    state.showsChat -> R.string.voice_reply_placeholder
                    state.voiceAvailable -> R.string.voice_placeholder_text
                    else -> R.string.voice_placeholder_text_first
                },
            ),
            enabled = !state.isGenerating && !state.chatPending,
            onSend = send,
            onMic = if (state.showsChat) onMic else null,
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
        if (state.showsChat) {
            TextButton(onClick = onRestart, enabled = !state.isGenerating) { Text(stringResource(R.string.voice_restart)) }
        }
    }
}

/** 問いかけ(iOS の `Greeting.copy`: 初回 / 再訪 / 新しいアプリ / 改変で出し分ける) */
@Composable
private fun Heading(title: String, subtitle: String) {
    Column(horizontalAlignment = Alignment.CenterHorizontally, verticalArrangement = Arrangement.spacedBy(8.dp)) {
        Text(title, style = MaterialTheme.typography.headlineSmall, fontWeight = FontWeight.Bold, textAlign = TextAlign.Center)
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

/** マイク(iOS の `MicButton`)。タップで始め、もう一度タップで止める。長押し([onLongClick])= 変身一覧。 */
@OptIn(ExperimentalFoundationApi::class)
@Composable
fun MicButton(
    state: VoiceUiState,
    size: Dp,
    onClick: () -> Unit,
    modifier: Modifier = Modifier,
    /** 読み上げの説明を差し替える(マイク FAB =「このアプリを変える」) */
    description: String? = null,
    onLongClick: (() -> Unit)? = null,
) {
    val switcherLabel = stringResource(R.string.voice_fab_switcher)
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
            .combinedClickable(
                enabled = listening || (state.phase.canStart && !state.chatPending),
                onClick = onClick,
                onLongClick = onLongClick?.takeIf { !listening },
                onLongClickLabel = onLongClick?.let { switcherLabel },
            )
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
internal fun MicGlyph(color: Color, stop: Boolean, modifier: Modifier = Modifier) {
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
    ErrorCard(VoiceErrorCopy.of(error), onRetry = vm::retry, onDismiss = vm::dismissError, onOpenSettings = onOpenSettings, onUpgrade = vm.upgradeHandler())
}

/** 状態カードの本文。上限に達した時は戻る時刻(分からなければ「明日」)を入れる。TalkBack の読み上げにも使う。 */
fun ErrorCardCopy.bodyText(english: Boolean): UiText {
    if (body != R.string.voice_rate_body) return UiText.Res(body)
    val whenText = resetAt?.let {
        UiText.Raw(
            DateTimeFormatter.ofPattern(if (english) "MMM d, H:mm" else "M月d日 H:mm", if (english) Locale.US else Locale.JAPAN)
                .withZone(ZoneId.systemDefault())
                .format(it),
        )
    } ?: UiText.Res(R.string.voice_tomorrow)
    return UiText.Res(body, whenText)
}

/** 状態カード(iOS の `ErrorCard`)。 */
@Composable
fun ErrorCard(
    copy: ErrorCardCopy,
    onRetry: () -> Unit,
    onDismiss: () -> Unit,
    onOpenSettings: () -> Unit,
    modifier: Modifier = Modifier,
    /** 「Pro にする」(#1103)。null = 課金を持たない構成なので出さない */
    onUpgrade: ((PaywallReason) -> Unit)? = null,
) {
    val english = LocalConfiguration.current.locales[0].language == "en"
    val body = copy.bodyText(english).resolve()
    OutlinedCard(modifier.fillMaxWidth().padding(vertical = 8.dp)) {
        Column(Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
            Text(stringResource(copy.title), style = MaterialTheme.typography.titleSmall, fontWeight = FontWeight.Bold)
            Text(body, style = MaterialTheme.typography.bodyMedium)
            Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                val upgrade = copy.upgrade
                when {
                    copy.openSettings -> Button(onClick = onOpenSettings) { Text(stringResource(R.string.voice_open_settings)) }
                    upgrade != null && onUpgrade != null -> Button(onClick = { onUpgrade(upgrade) }, modifier = Modifier.testTag("errorCard.upgradePro")) {
                        Text(stringResource(R.string.voice_upgrade_pro))
                    }
                    copy.retry -> Button(onClick = onRetry) { Text(stringResource(R.string.voice_retry)) }
                }
                TextButton(onClick = onDismiss) { Text(stringResource(R.string.voice_dismiss)) }
            }
        }
    }
}

/** 例文(#274)。初期画面はカテゴリ → 例、生成後は変更の例。タップで送る(iOS と同じ)。 */
@Composable
private fun Examples(state: VoiceUiState, onPick: (String) -> Unit, onTips: () -> Unit) {
    var category by rememberSaveable { mutableStateOf(Greeting.createCategories.first().id) }
    val examples = if (state.isInitial) {
        Greeting.createCategories.first { it.id == category }.examples
    } else {
        Greeting.editExamples
    }
    Column(Modifier.fillMaxWidth().padding(bottom = 8.dp), verticalArrangement = Arrangement.spacedBy(4.dp)) {
        Row(Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
            Text(
                stringResource(if (state.isInitial) R.string.voice_examples_label else R.string.voice_edit_examples_label),
                style = MaterialTheme.typography.labelMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                modifier = Modifier.weight(1f),
            )
            // 頼み方のコツ(#415)。例をタップすると入力欄に入る
            TextButton(onClick = onTips, modifier = Modifier.testTag("voice.tips")) {
                Text(stringResource(R.string.voice_tips_title), style = MaterialTheme.typography.labelMedium, fontWeight = FontWeight.SemiBold)
            }
        }
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

// MARK: - 相談(iOS の `ClarifyChatView`)

/**
 * 吹き出しの一覧。上に閉じた会話と反映の区切り(#210)、下に今の会話。右 = 自分・左 = AI・待っている最新の質問だけ大きく。
 */
@Composable
private fun ClarifyChat(state: VoiceUiState, vm: VoiceViewModel, modifier: Modifier = Modifier) {
    val list = rememberLazyListState()
    val questionNumber = if (state.chatPending) null else QuestionProgress.number(state.chat, state.chatMode, state.chatTurn?.kind)
    LaunchedEffect(state.history.size, state.chat.size, state.chatPending) {
        val count = state.history.size + state.chat.size + (if (state.chatPending) 1 else 0) + 1
        list.animateScrollToItem(count - 1)
    }
    LazyColumn(modifier.fillMaxWidth(), state = list, verticalArrangement = Arrangement.spacedBy(8.dp)) {
        items(state.history, key = { it.id }) { HistoryRow(it) }
        itemsIndexed(state.chat) { i, bubble ->
            val latestQuestion = i == state.chat.lastIndex && questionNumber != null
            Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
                Bubble(bubble, large = latestQuestion)
                if (latestQuestion) {
                    Text(
                        stringResource(R.string.voice_question_progress, questionNumber!!, QuestionProgress.MAX_QUESTIONS),
                        style = MaterialTheme.typography.labelSmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                        modifier = Modifier.padding(start = 8.dp),
                    )
                }
            }
        }
        if (state.chatPending) {
            item { Bubble(ChatBubble(fromUser = false, text = stringResource(R.string.voice_status_thinking_chat))) }
        }
        item {
            // 無料枠切れ・402 で止めた作成(#1261)は、案や質問の代わりにカードを出す(iOS の ClarifyChatView と同じ)
            val held = state.heldBuild
            if (held != null) HeldBuildCard(held, busy = state.isGenerating, vm) else TurnActions(state, vm)
        }
    }
}

@Composable
private fun Bubble(bubble: ChatBubble, large: Boolean = false) {
    val colors = MaterialTheme.colorScheme
    Box(Modifier.fillMaxWidth(), contentAlignment = if (bubble.fromUser) Alignment.CenterEnd else Alignment.CenterStart) {
        Surface(
            shape = RoundedCornerShape(16.dp),
            color = if (bubble.fromUser) colors.primaryContainer else colors.surfaceVariant,
            modifier = Modifier.widthIn(max = 320.dp),
        ) {
            Text(
                bubble.text,
                Modifier.padding(horizontal = 14.dp, vertical = 10.dp),
                style = if (large) MaterialTheme.typography.titleMedium else MaterialTheme.typography.bodyMedium,
                fontWeight = if (large) FontWeight.SemiBold else null,
            )
        }
    }
}

/** 閉じた会話の 1 件(少し薄く)か、反映の区切り「ここで反映しました(v3)・〜」 */
@Composable
private fun HistoryRow(entry: ChatHistoryEntry) {
    when (entry.kind) {
        ChatHistoryEntry.Kind.APPLIED -> {
            val head = entry.version?.let { stringResource(R.string.voice_history_applied, it) }
                ?: stringResource(R.string.voice_history_applied_plain)
            val text = entry.text.trim()
            Row(Modifier.fillMaxWidth().padding(vertical = 4.dp), verticalAlignment = Alignment.CenterVertically) {
                HorizontalDivider(Modifier.weight(1f))
                Text(
                    if (text.isEmpty()) head else "$head・$text",
                    style = MaterialTheme.typography.labelSmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    textAlign = TextAlign.Center,
                    modifier = Modifier.padding(horizontal = 8.dp).widthIn(max = 260.dp),
                )
                HorizontalDivider(Modifier.weight(1f))
            }
        }
        else -> Box(Modifier.alpha(0.7f)) {
            Bubble(ChatBubble(fromUser = entry.kind == ChatHistoryEntry.Kind.USER, text = entry.text))
        }
    }
}

/**
 * 最後の返事に応じた操作(iOS の `ClarifyFlow.offer`): 質問 = チップ / 案 = カード / ready = 「これでいく」。
 * `multiSelect` の時はチップ・案をいくつでも選び、まとめて送る(#220)。下に副ボタン(「このまま作る」/「このまま反映」)。
 */
@Composable
private fun TurnActions(state: VoiceUiState, vm: VoiceViewModel) {
    val turn = state.chatTurn ?: return
    if (state.chatPending || state.chat.lastOrNull()?.fromUser == true) return
    val busy = state.isGenerating
    // 選んだもの。返事が変わったら(吹き出しの数で見る)空に戻す
    var pickedChips by remember(state.chat.size) { mutableStateOf(listOf<String>()) }
    var pickedOptions by remember(state.chat.size) { mutableStateOf(setOf<String>()) }
    val multi = turn.isMultiSelect
    Column(Modifier.fillMaxWidth().padding(vertical = 4.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
        when (turn.kind) {
            ChatTurn.Kind.QUESTION, ChatTurn.Kind.ANSWER -> {
                val chips = turn.suggestions.orEmpty().filter { it.isNotBlank() }.take(3)
                val multiChips = multi && turn.kind == ChatTurn.Kind.QUESTION
                if (chips.isNotEmpty()) {
                    if (multiChips) MultiSelectHint()
                    Row(Modifier.horizontalScroll(rememberScrollState()), horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                        chips.forEach { c ->
                            if (multiChips) {
                                FilterChip(
                                    selected = c in pickedChips,
                                    onClick = { pickedChips = if (c in pickedChips) pickedChips - c else pickedChips + c },
                                    enabled = !busy,
                                    label = { Text(c) },
                                )
                            } else {
                                SuggestionChip(onClick = { vm.answer(c) }, enabled = !busy, label = { Text(c) })
                            }
                        }
                    }
                    if (multiChips) {
                        Button(onClick = { vm.answerMany(pickedChips) }, enabled = !busy && pickedChips.isNotEmpty()) {
                            Text(stringResource(R.string.voice_answer_selected))
                        }
                    }
                }
            }
            ChatTurn.Kind.PROPOSAL -> {
                val options = turn.options.orEmpty().filter { it.instructionText.isNotBlank() }.take(3)
                if (multi) MultiSelectHint()
                options.forEach { option ->
                    val selected = if (multi) option.optionId in pickedOptions else null
                    OptionCard(option, enabled = !busy, selected = selected) {
                        if (multi) {
                            val id = option.optionId
                            pickedOptions = if (id in pickedOptions) pickedOptions - id else pickedOptions + id
                        } else {
                            vm.choose(option)
                        }
                    }
                }
                if (multi) {
                    val chosen = options.filter { it.optionId in pickedOptions }
                    Button(onClick = { vm.chooseMany(chosen) }, enabled = !busy && chosen.isNotEmpty()) {
                        Text(stringResource(R.string.voice_choose_selected, chosen.size))
                    }
                }
            }
            ChatTurn.Kind.READY -> OutlinedCard(Modifier.fillMaxWidth()) {
                Column(Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
                    val title = if (state.chatMode == ChatMode.EDIT) R.string.voice_edit_ready_title else R.string.voice_ready_title
                    Text(stringResource(title), style = MaterialTheme.typography.titleSmall, fontWeight = FontWeight.Bold)
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
        }
        when (ChatShortcut.of(turn, state.chatMode)) {
            ChatShortcut.BUILD_NOW ->
                OutlinedButton(onClick = vm::buildNow, enabled = !busy) { Text(stringResource(R.string.voice_build_now_button)) }
            ChatShortcut.APPLY_NOW ->
                OutlinedButton(onClick = vm::applyNow, enabled = !busy) { Text(stringResource(R.string.voice_apply_now_button)) }
            ChatShortcut.NONE -> Unit
        }
    }
}

@Composable
private fun MultiSelectHint() {
    Text(
        stringResource(R.string.voice_multi_select_hint),
        style = MaterialTheme.typography.labelMedium,
        fontWeight = FontWeight.SemiBold,
        color = MaterialTheme.colorScheme.onSurfaceVariant,
    )
}

/** 案 1 つ。[selected] が null でない時(複数選択)はカード全体で選び、「これでいく」は下にまとめて出す */
@Composable
private fun OptionCard(option: ChatOption, enabled: Boolean, selected: Boolean? = null, onChoose: () -> Unit) {
    val stateText = selected?.let { stringResource(if (it) R.string.voice_option_selected else R.string.voice_option_unselected) }
    val border = if (selected == true) BorderStroke(2.dp, MaterialTheme.colorScheme.primary) else CardDefaults.outlinedCardBorder(enabled)
    OutlinedCard(
        Modifier
            .fillMaxWidth()
            .then(if (selected != null) Modifier.clickable(enabled = enabled, onClick = onChoose) else Modifier)
            .semantics { if (stateText != null) stateDescription = stateText },
        border = border,
    ) {
        Row(Modifier.padding(16.dp), verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(12.dp)) {
            if (selected != null) Checkbox(checked = selected, onCheckedChange = null, enabled = enabled)
            Column(Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(6.dp)) {
                Text(option.title, style = MaterialTheme.typography.titleSmall, fontWeight = FontWeight.Bold)
                if (option.detail.isNotEmpty()) Text(option.detail, style = MaterialTheme.typography.bodySmall)
                if (selected == null) Button(onClick = onChoose, enabled = enabled) { Text(stringResource(R.string.voice_build_button)) }
            }
        }
    }
}

/** 「最初から」で、これまでの会話があるときの確認(iOS の M9) */
@Composable
private fun RestartConfirmDialog(vm: VoiceViewModel) {
    AlertDialog(
        onDismissRequest = vm::dismissRestartConfirm,
        title = { Text(stringResource(R.string.voice_restart_confirm_title)) },
        text = { Text(stringResource(R.string.voice_restart_confirm_body)) },
        confirmButton = {
            TextButton(onClick = vm::clearChatHistory) {
                Text(stringResource(R.string.voice_restart_clear_all), color = MaterialTheme.colorScheme.error)
            }
        },
        dismissButton = {
            Column(horizontalAlignment = Alignment.End) {
                TextButton(onClick = vm::restartLiveChat) { Text(stringResource(R.string.voice_restart_live_only)) }
                TextButton(onClick = vm::dismissRestartConfirm) { Text(stringResource(R.string.voice_cancel)) }
            }
        },
    )
}

/** 相談の ON / OFF(iOS の設定の「チャットで相談してから作る」)。設定の画面([jp.kz2601.metamon.settings.SettingsSheet])に出す */
@Composable
fun ClarifySection(settings: ClarifySettings, modifier: Modifier = Modifier) {
    var enabled by remember { mutableStateOf(settings.enabled) }
    var autoBuild by remember { mutableStateOf(settings.autoBuild) }
    Column(modifier.fillMaxWidth().testTag("settings.clarify")) {
        SettingSwitch(stringResource(R.string.voice_clarify_setting_title), enabled) {
            enabled = it
            settings.enabled = it
        }
        SettingFooter(stringResource(R.string.voice_clarify_setting_footer))
        if (enabled) {
            SettingSwitch(stringResource(R.string.voice_clarify_autobuild_title), autoBuild) {
                autoBuild = it
                settings.autoBuild = it
            }
            SettingFooter(stringResource(R.string.voice_clarify_autobuild_footer))
        }
    }
}

@Composable
private fun SettingSwitch(title: String, checked: Boolean, onChange: (Boolean) -> Unit) {
    Row(
        Modifier
            .fillMaxWidth()
            .toggleable(value = checked, role = Role.Switch, onValueChange = onChange)
            .padding(horizontal = 16.dp, vertical = 12.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Text(title, Modifier.weight(1f))
        Switch(checked = checked, onCheckedChange = null)
    }
}

@Composable
private fun SettingFooter(text: String) {
    Text(
        text,
        style = MaterialTheme.typography.bodySmall,
        color = MaterialTheme.colorScheme.onSurfaceVariant,
        modifier = Modifier.padding(horizontal = 16.dp, vertical = 4.dp),
    )
}

// MARK: - 今日の残り・頼み方のコツ・読み上げ(#892)

/** 入力欄の上の小さな「今日の残り N」(iOS の `RemainingTodayBadge`)。少ない時と 0 は目立たせる */
@Composable
private fun RemainingTodayBadge(state: VoiceUiState) {
    val display = RemainingTodayDisplay.of(state.remainingToday, Instant.now(), state.isPro)
    val (n, emphasized) = when (display) {
        RemainingTodayDisplay.Hidden -> return
        is RemainingTodayDisplay.Normal -> display.remaining to false
        is RemainingTodayDisplay.Low -> display.remaining to true
        is RemainingTodayDisplay.Exhausted -> 0 to true
    }
    val pro = when (display) {
        is RemainingTodayDisplay.Normal -> display.pro
        is RemainingTodayDisplay.Low -> display.pro
        else -> false
    }
    val label = stringResource(R.string.voice_remaining_today, n)
    val withPro = if (pro) stringResource(R.string.voice_remaining_a11y_pro, label) else label
    val a11y = if (display is RemainingTodayDisplay.Low) stringResource(R.string.voice_remaining_a11y_low, withPro) else withPro
    val hint = stringResource(
        R.string.voice_remaining_hint,
        RemainingToday.Weight.CREATE,
        RemainingToday.Weight.EDIT,
        RemainingToday.Weight.CHAT,
        resetText(state.remainingToday?.resetAt),
    )
    Row(
        Modifier
            .fillMaxWidth()
            .padding(top = 4.dp)
            .semantics(mergeDescendants = true) {
                contentDescription = a11y
                stateDescription = hint
            }
            .testTag("voice.remainingToday"),
        horizontalArrangement = Arrangement.spacedBy(6.dp, Alignment.End),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        if (pro) {
            Surface(shape = RoundedCornerShape(50), color = MaterialTheme.colorScheme.primary) {
                Text(
                    "Pro",
                    style = MaterialTheme.typography.labelSmall,
                    fontWeight = FontWeight.Bold,
                    color = MaterialTheme.colorScheme.onPrimary,
                    modifier = Modifier.padding(horizontal = 6.dp, vertical = 1.dp),
                )
            }
        }
        Text(
            label,
            style = MaterialTheme.typography.labelMedium,
            fontWeight = if (emphasized) FontWeight.SemiBold else null,
            color = if (emphasized) MaterialTheme.colorScheme.error else MaterialTheme.colorScheme.onSurfaceVariant,
        )
    }
}

/** 戻る時刻(上限のカードと同じ書き方)。分からなければ「明日」 */
@Composable
internal fun resetText(at: Instant?): String {
    val english = LocalConfiguration.current.locales[0].language == "en"
    return at?.let {
        DateTimeFormatter.ofPattern(if (english) "MMM d, H:mm" else "M月d日 H:mm", if (english) Locale.US else Locale.JAPAN)
            .withZone(ZoneId.systemDefault())
            .format(it)
    } ?: stringResource(R.string.voice_tomorrow)
}

/** 「頼み方のコツ」のシート(iOS の `PromptTipsSheet`)。例をタップすると [onPick] に文を渡す(送らない) */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun PromptTipsSheet(onDismiss: () -> Unit, onPick: (String) -> Unit) {
    ModalBottomSheet(onDismissRequest = onDismiss, modifier = Modifier.testTag("tips.list")) {
        val pickHint = stringResource(R.string.voice_tips_pick_hint)
        LazyColumn(Modifier.fillMaxWidth().padding(horizontal = 20.dp), verticalArrangement = Arrangement.spacedBy(4.dp)) {
            item {
                Text(stringResource(R.string.voice_tips_title), style = MaterialTheme.typography.titleMedium, fontWeight = FontWeight.Bold)
                Text(
                    stringResource(R.string.voice_tips_intro),
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    modifier = Modifier.padding(vertical = 8.dp),
                )
            }
            PromptTips.categories(PromptTips.available(ReviewBuild.enabled)).forEach { category ->
                item(key = "cat-" + category.id) {
                    Text(
                        stringResource(category.title),
                        style = MaterialTheme.typography.labelLarge,
                        fontWeight = FontWeight.SemiBold,
                        color = MaterialTheme.colorScheme.primary,
                        modifier = Modifier.padding(top = 12.dp, bottom = 4.dp).testTag("tips.category.${category.id}"),
                    )
                }
                items(category.tips, key = { it.id }) { tip ->
                    val text = stringResource(tip.text)
                    Text(
                        text,
                        style = MaterialTheme.typography.bodyMedium,
                        modifier = Modifier
                            .fillMaxWidth()
                            .clickable(onClickLabel = pickHint) { onPick(text) }
                            .padding(vertical = 10.dp)
                            .testTag("tips.example.${tip.id}"),
                    )
                }
            }
            item { Spacer(Modifier.height(24.dp)) }
        }
    }
}

/** 読み上げの ON / OFF(iOS / Mac の設定の「読み上げ」)。設定の画面に出す */
@Composable
fun SpeechSection(settings: SpeechSettings, modifier: Modifier = Modifier) {
    var enabled by remember { mutableStateOf(settings.enabled) }
    Column(modifier.fillMaxWidth().testTag("settings.speech")) {
        Text(
            stringResource(R.string.voice_speech_section),
            style = MaterialTheme.typography.labelLarge,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
            modifier = Modifier.padding(horizontal = 16.dp, vertical = 4.dp),
        )
        SettingSwitch(stringResource(R.string.voice_speech_setting_title), enabled) {
            enabled = it
            settings.enabled = it
        }
        SettingFooter(stringResource(R.string.voice_speech_setting_footer))
    }
}

/** 画面が出てから挨拶を読むまで(iOS と同じ 0.4 秒) */
private const val GREETING_SPEECH_DELAY_MILLIS = 400L
