package jp.kz2601.metamon.renderer.components.audio

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.defaultMinSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
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
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalFocusManager
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.semantics.testTag
import androidx.compose.ui.unit.dp
import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.renderer.R
import jp.kz2601.metamon.renderer.components.actions.ActionButton
import jp.kz2601.metamon.renderer.components.actions.ActionButtonKind
import jp.kz2601.metamon.renderer.components.inputs.LabeledInput
import jp.kz2601.metamon.renderer.components.inputs.NeedsForm
import jp.kz2601.metamon.renderer.form.FormDraft
import jp.kz2601.metamon.renderer.form.TextInputRules
import jp.kz2601.metamon.renderer.form.setText
import jp.kz2601.metamon.renderer.form.text
import jp.kz2601.metamon.renderer.registry.RenderContext
import jp.kz2601.metamon.renderer.theme.RenderTheme
import kotlinx.coroutines.launch
import java.util.UUID

/** 話して入力の段階。STARTING = 録音を始めている間(二重に始めない)。 */
enum class DictationPhase { IDLE, STARTING, LISTENING, TRANSCRIBING }

/**
 * `transcribeField`(iOS の `TranscribeFieldLeaf`): 押して話す → 端末の中で文字にして string / text の field の後ろに足す。
 * 声を文字にするのはホスト([DictationHost]・本体の音声入力と同じ認識器)。声は端末の外に出さず、録音も残さない。
 * 画面を離れた・マイク FAB の音声入力が出たら録音を捨てる(マイクを取り合わない)。
 */
@Composable
fun TranscribeFieldLeaf(c: Component, ctx: RenderContext, modifier: Modifier = Modifier) {
    val form = ctx.form
    val field = c.field
    if (form == null || field == null) return NeedsForm(c, modifier)
    TranscribeFieldPanel(c, form, field, ctx.runtime.dictation, modifier)
}

@Composable
private fun TranscribeFieldPanel(c: Component, form: FormDraft, field: String, host: DictationHost?, modifier: Modifier) {
    val context = LocalContext.current
    val scope = rememberCoroutineScope()
    val focus = LocalFocusManager.current
    val requestMic = rememberMicrophoneRequest()
    val owner = remember { "dictation-" + UUID.randomUUID() }
    var phase by remember { mutableStateOf(DictationPhase.IDLE) }
    var failure by remember { mutableStateOf<DictationFailure?>(null) }
    val lines = TextInputRules.editorLines(c.minLines)

    fun cancel() {
        if (phase != DictationPhase.LISTENING && phase != DictationPhase.STARTING) return
        host?.cancelDictation()
        AudioArbiter.release(owner)
        phase = DictationPhase.IDLE
    }

    fun finish() {
        val h = host ?: return
        if (phase != DictationPhase.LISTENING) return
        phase = DictationPhase.TRANSCRIBING
        scope.launch {
            try {
                val text = h.finishDictation()
                form.setText(field, DictationText.appending(text, form.text(field)))
            } catch (e: DictationException) {
                failure = e.failure
            } finally {
                AudioArbiter.release(owner)
                phase = DictationPhase.IDLE
            }
        }
    }

    fun start() {
        val h = host ?: return
        if (phase != DictationPhase.IDLE) return
        phase = DictationPhase.STARTING
        failure = null
        focus.clearFocus()
        requestMic { granted ->
            if (!granted) {
                failure = DictationFailure.MIC_DENIED
                phase = DictationPhase.IDLE
                return@requestMic
            }
            AudioArbiter.claim(context, owner, AudioClaims.Use.DICTATION) { cancel() }
            scope.launch {
                try {
                    h.startDictation()
                    // 始めている間に捨てられた(画面を離れた・FAB の音声入力)
                    if (phase == DictationPhase.STARTING) phase = DictationPhase.LISTENING else h.cancelDictation()
                } catch (e: DictationException) {
                    failure = e.failure
                    AudioArbiter.release(owner)
                    phase = DictationPhase.IDLE
                }
            }
        }
    }

    if (host != null) {
        val voiceActive by host.isVoiceInputActive.collectAsState()
        val listening by host.listening.collectAsState()
        LaunchedEffect(voiceActive) { if (voiceActive) cancel() }
        // 認識器が無音で自分から止まったら、そこまでを文字にする
        LaunchedEffect(listening) { if (!listening && phase == DictationPhase.LISTENING) finish() }
    }
    DisposableEffect(Unit) { onDispose { cancel() } }

    LabeledInput(c.label, modifier.semantics { testTag = "transcribeField.$field" }) {
        Column(verticalArrangement = Arrangement.spacedBy(RenderTheme.Spacing.sm)) {
            OutlinedTextField(
                value = form.text(field),
                onValueChange = { form.setText(field, it) },
                modifier = Modifier.fillMaxWidth().semantics { testTag = "field.$field" },
                placeholder = { Text(c.placeholder ?: stringResource(R.string.renderer_audio_transcribe_placeholder)) },
                minLines = lines.first,
                maxLines = lines.last,
                isError = form.isFlagged(field),
                shape = RoundedCornerShape(RenderTheme.Radius.control),
            )
            MicRow(host, phase, onStart = ::start, onFinish = ::finish)
            failure?.let { Message(stringResource(failureText(it)), error = it == DictationFailure.MIC_DENIED) }
        }
    }
}

@Composable
private fun MicRow(host: DictationHost?, phase: DictationPhase, onStart: () -> Unit, onFinish: () -> Unit) {
    if (host == null) return Message(stringResource(R.string.renderer_audio_transcribe_no_host))
    val readiness by host.readiness.collectAsState()
    when {
        phase == DictationPhase.LISTENING -> MicButton(stringResource(R.string.renderer_audio_transcribe_listening), ActionButtonKind.PRIMARY, "xmark", onClick = onFinish)
        phase == DictationPhase.STARTING -> MicButton(stringResource(R.string.renderer_audio_transcribe_mic), ActionButtonKind.SECONDARY, "mic", enabled = false) {}
        phase == DictationPhase.TRANSCRIBING -> Progress(stringResource(R.string.renderer_audio_transcribe_working))
        readiness == DictationReadiness.READY -> MicButton(stringResource(R.string.renderer_audio_transcribe_mic), ActionButtonKind.SECONDARY, "mic", onClick = onStart)
        readiness == DictationReadiness.PREPARING -> Progress(stringResource(R.string.renderer_audio_transcribe_preparing))
        readiness == DictationReadiness.NEEDS_MODEL ->
            MicButton(stringResource(R.string.renderer_audio_transcribe_get_model), ActionButtonKind.SECONDARY, "arrow.down.right") { host.prepareDictation() }
        else -> Message(stringResource(R.string.renderer_audio_transcribe_unavailable))
    }
}

@Composable
private fun MicButton(label: String, kind: ActionButtonKind, icon: String, enabled: Boolean = true, onClick: () -> Unit) {
    ActionButton(label, kind, onClick = onClick, modifier = Modifier.semantics { testTag = "transcribeField.mic" }, icon = icon, enabled = enabled)
}

@Composable
private fun Progress(label: String) {
    Row(
        Modifier.fillMaxWidth().defaultMinSize(minHeight = 44.dp),
        horizontalArrangement = Arrangement.spacedBy(RenderTheme.Spacing.sm, Alignment.CenterHorizontally),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        CircularProgressIndicator(Modifier.size(18.dp), strokeWidth = 2.dp)
        Text(label, style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
    }
}

internal fun failureText(f: DictationFailure): Int = when (f) {
    DictationFailure.MIC_DENIED -> R.string.renderer_audio_mic_denied
    DictationFailure.NOT_READY -> R.string.renderer_audio_dictation_not_ready
    DictationFailure.NOT_HEARD -> R.string.renderer_audio_dictation_not_heard
    DictationFailure.UNAVAILABLE -> R.string.renderer_audio_transcribe_unavailable
    DictationFailure.FAILED -> R.string.renderer_audio_dictation_failed
}
