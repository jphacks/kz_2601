package jp.kz2601.metamon.renderer.components.ai

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
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.selection.SelectionContainer
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Send
import androidx.compose.material3.Button
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalConfiguration
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.ai.AiChat
import jp.kz2601.metamon.renderer.R
import jp.kz2601.metamon.renderer.a11y.TalkBack
import jp.kz2601.metamon.renderer.a11y.rememberAnnouncer
import jp.kz2601.metamon.renderer.components.audio.DictationException
import jp.kz2601.metamon.renderer.components.audio.DictationFailure
import jp.kz2601.metamon.renderer.components.play.rememberRecords
import jp.kz2601.metamon.renderer.registry.RenderContext
import jp.kz2601.metamon.renderer.theme.RenderTheme
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.launch
import java.util.UUID

/**
 * `aiChat`(iOS の AiChatLeaf)。クラウドの AI と何往復も話す。吹き出しは古い順に上から下へ(自分は右・AI は左)、下に入力欄。
 * 送る・保存は [AiChatSend]。saveTo の記録の案は最新の返事の下にカード([AiChatSuggestion])。
 * 話して入力・読み上げは [LocalAiChatVoice](無ければマイクとスピーカーを出さない)。
 */
@Composable
fun AiChatLeaf(c: Component, ctx: RenderContext, modifier: Modifier = Modifier) {
    val spec = remember(c) { AiChatSend.Spec.of(c) }
    if (spec == null || ctx.runtime.entityModel(spec.entity) == null) {
        Text(stringResource(R.string.renderer_unavailable_here), modifier, style = MaterialTheme.typography.labelSmall)
        return
    }
    val rt = ctx.runtime
    val scope = rememberCoroutineScope()
    val locale = if (LocalConfiguration.current.locales[0]?.language == "en") "en" else "ja"
    val voice = LocalAiChatVoice.current
    val records by rememberRecords(rt)
    val turns = remember(records, spec) { AiChatSend.turns(rt, spec) }
    val online = rt.ai?.supportsOnlineText == true
    val owner = remember { UUID.randomUUID().toString() }
    val noFlow = remember { MutableStateFlow<String?>(null) }
    val speaking by (voice?.speakingOwner ?: noFlow).collectAsState()

    var draft by remember { mutableStateOf("") }
    var sending by remember { mutableStateOf(false) }
    var pending by remember { mutableStateOf<String?>(null) }
    var error by remember { mutableStateOf<AiMessage?>(null) }
    var suggestion by remember { mutableStateOf<AiChatSuggestion?>(null) }
    var phase by remember { mutableStateOf(DictationPhase.IDLE) }
    var visible by remember { mutableStateOf(true) }
    val context = LocalContext.current
    val announce = rememberAnnouncer()
    val announceQueue = remember { AiChatAnnounceQueue() }

    LaunchedEffect(spec.entity) { rt.loadRecords(setOf(spec.entity)) }
    // 録音中に届いた返事は録音が終わってから TalkBack に読ませる
    LaunchedEffect(phase) {
        announceQueue.phaseChanged(phase, visible, TalkBack.isRunning(context))?.let(announce)
    }
    DisposableEffect(owner) {
        visible = true
        onDispose {
            visible = false
            announceQueue.clear()
            speaking?.takeIf { it.startsWith("$owner/") }?.let { voice?.stop(it) }
            if (phase == DictationPhase.LISTENING) voice?.cancelDictation()
        }
    }

    fun send() {
        val text = draft.trim()
        if (text.isEmpty() || sending) return
        sending = true
        error = null
        suggestion = null // 前の案は閉じる(言い直した内容で新しい案が来る)
        draft = ""
        scope.launch {
            try {
                when (val out = AiChatSend.send(rt, spec, text, locale) { pending = it }) {
                    is AiChatSend.Outcome.Sent -> {
                        suggestion = out.suggestion
                        val id = out.turnId
                        if (id != null) {
                            // 保存できた返事だけ。TalkBack の時は端末の声と重ねず TalkBack に読ませる
                            val mode = AiChatAutoRead.mode(c.speak, visible, phase != DictationPhase.IDLE, TalkBack.isRunning(context))
                            when (mode) {
                                AiChatAutoRead.SPEAK -> voice?.speak(out.reply, "$owner/$id")
                                AiChatAutoRead.ANNOUNCE -> announce(out.reply)
                                AiChatAutoRead.QUEUE -> announceQueue.received(out.reply, mode)
                                AiChatAutoRead.NONE -> Unit
                            }
                        }
                    }
                    is AiChatSend.Outcome.Failed -> {
                        error = out.message
                        if (out.restore && draft.isEmpty()) draft = text // 送れなかった文を入力欄に戻す
                    }
                }
            } finally {
                sending = false
            }
        }
    }

    Column(modifier.fillMaxWidth(), verticalArrangement = Arrangement.spacedBy(RenderTheme.Spacing.sm)) {
        if (turns.isEmpty() && pending == null && !sending) {
            Text(
                c.emptyText?.takeIf { it.isNotEmpty() } ?: stringResource(R.string.ai_chat_empty),
                Modifier.fillMaxWidth().heightIn(min = 120.dp).padding(RenderTheme.Spacing.md),
                style = MaterialTheme.typography.bodyMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        } else {
            val list = rememberLazyListState()
            val count = turns.size + (if (pending != null) 1 else 0) + (if (sending) 1 else 0) + (if (suggestion != null) 1 else 0)
            LaunchedEffect(count) { if (count > 0) list.animateScrollToItem(count - 1) }
            LazyColumn(
                Modifier.fillMaxWidth().heightIn(min = 200.dp, max = 480.dp).testTag("aiChat.messages"),
                state = list,
                verticalArrangement = Arrangement.spacedBy(RenderTheme.Spacing.xs),
            ) {
                turns.forEach { t ->
                    item(key = t.id) {
                        if (t.role == AiChat.Role.USER) {
                            Bubble(t.text, mine = true)
                        } else {
                            val id = "$owner/${t.id}"
                            Bubble(
                                t.text,
                                mine = false,
                                speaking = speaking == id,
                                canSpeak = AiChatMic.canSpeak(phase),
                                onSpeak = voice?.let { v -> { if (speaking == id) v.stop(id) else v.speak(t.text, id) } },
                            )
                        }
                    }
                    val s = suggestion
                    if (s != null && s.turnId == t.id) {
                        item(key = "suggest") {
                            SuggestCard(
                                s,
                                onAdd = {
                                    if (s.state != AiChatSuggestion.SaveState.Saving && s.state != AiChatSuggestion.SaveState.Saved) {
                                        suggestion = s.copy(state = AiChatSuggestion.SaveState.Saving)
                                        scope.launch {
                                            val msg = AiChatSend.addSuggestion(rt, s)
                                            if (suggestion?.turnId == s.turnId) {
                                                suggestion = s.copy(
                                                    state = msg?.let { AiChatSuggestion.SaveState.Failed(it) } ?: AiChatSuggestion.SaveState.Saved,
                                                )
                                            }
                                        }
                                    }
                                },
                                // 「直す」: カードを閉じて入力欄へ(会話で言い直すと AI が新しい案を出す)
                                onFix = { suggestion = null },
                            )
                        }
                    }
                }
                pending?.let { p -> item(key = "pending") { Bubble(p, mine = true) } }
                if (sending) item(key = "thinking") { Thinking() }
            }
        }
        Footer(error, online, c.inputEntity?.let { rt.entityModel(it)?.name })
        Row(verticalAlignment = Alignment.Bottom, horizontalArrangement = Arrangement.spacedBy(RenderTheme.Spacing.sm)) {
            OutlinedTextField(
                value = draft,
                onValueChange = { draft = it },
                modifier = Modifier.weight(1f).testTag("aiChat.input"),
                placeholder = { Text(c.placeholder?.takeIf { it.isNotEmpty() } ?: stringResource(R.string.ai_chat_placeholder)) },
                maxLines = 4,
            )
            if (voice != null) {
                MicButton(voice, phase, online, onCancelled = { phase = DictationPhase.IDLE }) { action ->
                    scope.launch {
                        try {
                            when (action) {
                                MicAction.START -> {
                                    if (phase != DictationPhase.IDLE) return@launch // 二重に始めない
                                    phase = DictationPhase.STARTING
                                    error = null
                                    voice.startDictation()
                                    // 始める間に画面を離れた・マイク FAB が始まった
                                    phase = if (visible && !voice.voiceInputActive.value) {
                                        DictationPhase.LISTENING
                                    } else {
                                        voice.cancelDictation()
                                        DictationPhase.IDLE
                                    }
                                }
                                MicAction.FINISH -> {
                                    if (phase != DictationPhase.LISTENING) return@launch
                                    phase = DictationPhase.TRANSCRIBING
                                    draft = appendDictation(voice.finishDictation(), draft)
                                    phase = DictationPhase.IDLE
                                }
                            }
                        } catch (e: CancellationException) {
                            throw e
                        } catch (e: Exception) {
                            val denied = (e as? DictationException)?.failure == DictationFailure.MIC_DENIED
                            error = AiMessage(if (denied) R.string.renderer_audio_mic_denied else R.string.ai_chat_mic_failed)
                            phase = DictationPhase.IDLE
                        }
                    }
                }
            }
            val canSend = AiChatMic.canSend(online, sending, draft, phase)
            IconButton(onClick = ::send, enabled = canSend, modifier = Modifier.testTag("aiChat.send")) {
                Icon(Icons.Filled.Send, contentDescription = stringResource(R.string.ai_chat_send))
            }
        }
    }
}

private enum class MicAction { START, FINISH }

/** マイクのボタン。録音の開始・終了は [onAction]、モデルの用意はその場で頼む。 */
@Composable
private fun MicButton(
    voice: AiChatVoice,
    phase: DictationPhase,
    online: Boolean,
    onCancelled: () -> Unit,
    onAction: (MicAction) -> Unit,
) {
    val readiness by voice.readiness.collectAsState()
    val active by voice.voiceInputActive.collectAsState()
    val mic = AiChatMic.state(phase, readiness, active, online)
    // 録音中にマイク FAB の音声入力が始まったら録音を捨てる
    LaunchedEffect(active) {
        if (active && phase == DictationPhase.LISTENING) {
            voice.cancelDictation()
            onCancelled()
        }
    }
    if (mic == AiChatMic.HIDDEN) return
    val label = stringResource(
        when (mic) {
            AiChatMic.STOP -> R.string.ai_chat_mic_stop
            AiChatMic.TRANSCRIBING, AiChatMic.STARTING -> R.string.ai_chat_mic_transcribing
            AiChatMic.PREPARE -> R.string.ai_chat_mic_prepare
            AiChatMic.BUSY -> R.string.ai_chat_mic_busy
            else -> R.string.ai_chat_mic
        },
    )
    TextButton(
        onClick = {
            when (mic) {
                AiChatMic.START -> onAction(MicAction.START)
                AiChatMic.STOP -> onAction(MicAction.FINISH)
                AiChatMic.PREPARE -> voice.prepareDictation()
                else -> Unit
            }
        },
        enabled = mic.enabled,
        modifier = Modifier.heightIn(min = 48.dp).semantics { contentDescription = label }.testTag("aiChat.mic"),
    ) {
        if (mic == AiChatMic.TRANSCRIBING || mic == AiChatMic.STARTING) {
            CircularProgressIndicator(Modifier.size(18.dp), strokeWidth = 2.dp)
        } else {
            Text(label, style = MaterialTheme.typography.labelMedium)
        }
    }
}

@Composable
private fun Bubble(
    text: String,
    mine: Boolean,
    speaking: Boolean = false,
    canSpeak: Boolean = true,
    onSpeak: (() -> Unit)? = null,
) {
    val who = stringResource(if (mine) R.string.ai_chat_me else R.string.ai_chat_ai)
    Row(Modifier.fillMaxWidth(), horizontalArrangement = if (mine) Arrangement.End else Arrangement.Start) {
        if (mine) Spacer(Modifier.size(56.dp))
        Column(horizontalAlignment = if (mine) Alignment.End else Alignment.Start, modifier = Modifier.weight(1f, fill = false)) {
            if (!mine) Text(who, style = MaterialTheme.typography.labelSmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
            val shape = RoundedCornerShape(18.dp)
            val bubble = if (mine) {
                Modifier.background(MaterialTheme.colorScheme.primary, shape)
            } else {
                Modifier.background(MaterialTheme.colorScheme.surfaceVariant, shape)
                    .border(1.dp, MaterialTheme.colorScheme.outlineVariant, shape)
            }
            SelectionContainer {
                Text(
                    text,
                    modifier = bubble.padding(horizontal = 14.dp, vertical = 9.dp).widthIn(max = 520.dp)
                        .semantics { contentDescription = "$who, $text" }
                        .testTag(if (mine) "aiChat.message.mine" else "aiChat.message.ai"),
                    color = if (mine) MaterialTheme.colorScheme.onPrimary else MaterialTheme.colorScheme.onSurface,
                    style = MaterialTheme.typography.bodyLarge,
                )
            }
            if (onSpeak != null) {
                TextButton(onClick = onSpeak, enabled = canSpeak || speaking, modifier = Modifier.testTag("aiChat.speak")) {
                    Text(stringResource(if (speaking) R.string.ai_chat_speak_stop else R.string.ai_chat_speak), style = MaterialTheme.typography.labelSmall)
                }
            }
        }
        if (!mine) Spacer(Modifier.size(56.dp))
    }
}

@Composable
private fun Thinking() {
    Row(
        Modifier.background(MaterialTheme.colorScheme.surfaceVariant, RoundedCornerShape(18.dp))
            .padding(horizontal = 14.dp, vertical = 9.dp).testTag("aiChat.thinking"),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(RenderTheme.Spacing.sm),
    ) {
        CircularProgressIndicator(Modifier.size(16.dp), strokeWidth = 2.dp)
        Text(stringResource(R.string.ai_chat_thinking), style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
    }
}

@Composable
private fun Footer(error: AiMessage?, online: Boolean, inputName: String?) {
    val style = MaterialTheme.typography.labelSmall
    when {
        error != null -> Text(
            error.raw ?: if (error.arg != null) stringResource(error.res, error.arg) else stringResource(error.res),
            style = style,
            color = MaterialTheme.colorScheme.error,
            modifier = Modifier.testTag("aiChat.error"),
        )
        !online -> Text(stringResource(R.string.ai_chat_offline), style = style, color = MaterialTheme.colorScheme.onSurfaceVariant)
        // inputEntity: 何を AI に渡すかを入力欄の上に出しておく
        inputName != null -> Text(
            stringResource(R.string.ai_chat_input_note, inputName),
            style = style,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
            modifier = Modifier.testTag("aiChat.inputNote"),
        )
    }
}

@Composable
private fun SuggestCard(s: AiChatSuggestion, onAdd: () -> Unit, onFix: () -> Unit) {
    val shape = RoundedCornerShape(14.dp)
    Column(
        Modifier.padding(end = 56.dp).fillMaxWidth()
            .background(MaterialTheme.colorScheme.surfaceVariant, shape)
            .border(1.dp, MaterialTheme.colorScheme.outlineVariant, shape)
            .padding(horizontal = 14.dp, vertical = 10.dp)
            .testTag("aiChat.suggest"),
        verticalArrangement = Arrangement.spacedBy(RenderTheme.Spacing.sm),
    ) {
        Text(stringResource(R.string.ai_chat_suggest_title), style = MaterialTheme.typography.labelMedium)
        s.rows.forEach { (name, value) ->
            Row(horizontalArrangement = Arrangement.spacedBy(RenderTheme.Spacing.sm)) {
                Text(name, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
                Text(value, style = MaterialTheme.typography.bodySmall)
            }
        }
        when (val st = s.state) {
            AiChatSuggestion.SaveState.Saved -> Text(
                stringResource(R.string.ai_chat_suggest_saved),
                style = MaterialTheme.typography.bodySmall,
                modifier = Modifier.testTag("aiChat.suggest.saved"),
            )
            else -> {
                if (st is AiChatSuggestion.SaveState.Failed) {
                    Text(stringResource(st.message.res), style = MaterialTheme.typography.labelSmall, color = MaterialTheme.colorScheme.error)
                }
                val saving = st == AiChatSuggestion.SaveState.Saving
                Row(horizontalArrangement = Arrangement.spacedBy(RenderTheme.Spacing.sm)) {
                    Button(onClick = onAdd, enabled = !saving, modifier = Modifier.weight(1f).testTag("aiChat.suggest.add")) {
                        Text(stringResource(R.string.ai_chat_suggest_add))
                    }
                    OutlinedButton(onClick = onFix, enabled = !saving, modifier = Modifier.weight(1f).testTag("aiChat.suggest.fix")) {
                        Text(stringResource(R.string.ai_chat_suggest_fix))
                    }
                }
            }
        }
    }
}
