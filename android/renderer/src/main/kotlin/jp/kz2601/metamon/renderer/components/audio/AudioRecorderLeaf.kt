package jp.kz2601.metamon.renderer.components.audio

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.defaultMinSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableDoubleStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.clearAndSetSemantics
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.semantics.testTag
import androidx.compose.ui.unit.dp
import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.records.JSONValue
import jp.kz2601.metamon.renderer.R
import jp.kz2601.metamon.renderer.components.actions.ActionButton
import jp.kz2601.metamon.renderer.components.actions.ActionButtonKind
import jp.kz2601.metamon.renderer.components.inputs.NeedsForm
import jp.kz2601.metamon.renderer.form.FormDraft
import jp.kz2601.metamon.renderer.registry.RenderContext
import jp.kz2601.metamon.renderer.theme.RenderTheme
import jp.kz2601.metamon.renderer.theme.colors
import kotlinx.coroutines.delay

/**
 * `audioRecorder`(iOS の `AudioRecorderLeaf`): 録音・停止・再生・録り直し・削除。録音中は経過時間 / 上限と音量メーター。
 * 録音は端末内に保存し、field には `"audio:<id>|<秒>"`([AudioValue])を入れる。
 * 画面にある間はイヤホンの 再生 / 一時停止 ボタンで録音を開始・停止できる([RemoteRecorderCommands])。
 */
@Composable
fun AudioRecorderLeaf(c: Component, ctx: RenderContext, modifier: Modifier = Modifier) {
    val form = ctx.form
    val field = c.field
    if (form == null || field == null) return NeedsForm(c, modifier)
    AudioRecorderPanel(c, form, field, modifier)
}

@Composable
private fun AudioRecorderPanel(c: Component, form: FormDraft, field: String, modifier: Modifier) {
    val context = LocalContext.current
    val scope = rememberCoroutineScope()
    val value = AudioValue.parse(form.values[field])
    val recorder = remember(field) { AudioRecorderController(context.applicationContext, c.maxDuration, value != null, scope) }
    val title = c.label?.takeIf { it.isNotEmpty() } ?: stringResource(R.string.renderer_audio_recorder_title)
    val requestMic = rememberMicrophoneRequest()
    val start = { requestMic { granted -> if (granted) recorder.start() else recorder.denied() } }

    recorder.onRecorded = { v -> form.values[field] = JSONValue.Str(v.encoded) }
    LaunchedEffect(value?.id) { recorder.sync(value != null) }
    DisposableEffect(recorder, title) {
        RemoteRecorderCommands.attach(context, recorder.owner, title) { command ->
            val v = AudioValue.parse(form.values[field])
            when (recorder.machine.remoteAction(command, AudioPlayback.isPlaying(v?.id))) {
                AudioRecorderMachine.RemoteAction.START_RECORDING -> start()
                AudioRecorderMachine.RemoteAction.STOP_RECORDING -> recorder.stop()
                AudioRecorderMachine.RemoteAction.STOP_PLAYBACK -> AudioPlayback.stop()
                null -> Unit
            }
        }
        onDispose {
            recorder.stop() // 録れた分は残す(フォームを閉じたら保存はされない)
            if (AudioPlayback.isPlaying(AudioValue.parse(form.values[field])?.id)) AudioPlayback.stop()
            RemoteRecorderCommands.detach(recorder.owner)
        }
    }

    Column(
        modifier.fillMaxWidth().semantics { testTag = "audioRecorder.$field" },
        verticalArrangement = Arrangement.spacedBy(RenderTheme.Spacing.sm),
    ) {
        Text(
            title,
            style = MaterialTheme.typography.labelMedium,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
            modifier = Modifier.padding(start = 2.dp),
        )
        when {
            recorder.isRecording -> Recording(recorder)
            value != null -> Recorded(value, title, onRerecord = start) {
                recorder.discard()
                form.values[field] = JSONValue.Null
            }
            else -> Idle(recorder, start)
        }
        when (recorder.failure) {
            AudioRecorderController.Failure.FAILED -> Message(stringResource(R.string.renderer_audio_record_failed), error = true)
            AudioRecorderController.Failure.DENIED -> Message(stringResource(R.string.renderer_audio_mic_denied), error = true)
            null -> Unit
        }
    }
}

@Composable
private fun Idle(recorder: AudioRecorderController, start: () -> Unit) {
    Column(Modifier.fillMaxWidth(), verticalArrangement = Arrangement.spacedBy(RenderTheme.Spacing.xs), horizontalAlignment = Alignment.CenterHorizontally) {
        ActionButton(
            stringResource(R.string.renderer_audio_record),
            ActionButtonKind.PRIMARY,
            onClick = start,
            modifier = Modifier.semantics { testTag = "audioRecorder.record" },
            icon = "mic",
        )
        Text(
            stringResource(R.string.renderer_audio_recorder_hint, AudioRecorderPolicy.elapsedText(recorder.maxDuration.toDouble())),
            style = MaterialTheme.typography.labelSmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant.copy(alpha = 0.7f),
        )
    }
}

@Composable
private fun Recording(recorder: AudioRecorderController) {
    val elapsed = AudioRecorderPolicy.elapsedText(recorder.elapsed)
    val a11y = stringResource(R.string.renderer_audio_recording_a11y, elapsed)
    Column(Modifier.fillMaxWidth(), verticalArrangement = Arrangement.spacedBy(RenderTheme.Spacing.sm), horizontalAlignment = Alignment.CenterHorizontally) {
        Row(
            Modifier.clearAndSetSemantics { contentDescription = a11y },
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(6.dp),
        ) {
            Box(Modifier.size(10.dp).clip(CircleShape).background(Color.Red))
            Text(elapsed, style = RenderTheme.Fonts.number)
            Text(
                "/ " + AudioRecorderPolicy.elapsedText(recorder.maxDuration.toDouble()),
                style = MaterialTheme.typography.bodyMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
        AudioLevelMeter(recorder.level)
        ActionButton(
            stringResource(R.string.renderer_audio_stop),
            ActionButtonKind.DESTRUCTIVE,
            onClick = recorder::stop,
            modifier = Modifier.semantics { testTag = "audioRecorder.stop" },
            icon = "xmark",
        )
    }
}

@Composable
private fun Recorded(value: AudioValue, title: String, onRerecord: () -> Unit, onDelete: () -> Unit) {
    val context = LocalContext.current
    val exists = remember(value.id) { LocalAudioStore.of(context).exists(value.id) }
    val colors = RenderTheme.colors
    Column(Modifier.fillMaxWidth(), verticalArrangement = Arrangement.spacedBy(RenderTheme.Spacing.sm)) {
        Row(
            Modifier
                .fillMaxWidth()
                .defaultMinSize(minHeight = 44.dp)
                .background(colors.brandSoft, RoundedCornerShape(RenderTheme.Radius.inner))
                .padding(horizontal = RenderTheme.Spacing.sm),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            AudioPlayButton(value, enabled = exists)
            Spacer(Modifier.weight(1f))
            if (!exists) {
                Text(
                    stringResource(R.string.renderer_audio_not_on_device),
                    style = MaterialTheme.typography.labelSmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
        }
        val deleteA11y = stringResource(R.string.renderer_audio_delete_a11y, title)
        Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.spacedBy(RenderTheme.Spacing.sm)) {
            ActionButton(
                stringResource(R.string.renderer_audio_rerecord),
                ActionButtonKind.SECONDARY,
                onClick = onRerecord,
                modifier = Modifier.weight(1f).semantics { testTag = "audioRecorder.rerecord" },
                icon = "mic",
            )
            ActionButton(
                stringResource(R.string.renderer_audio_delete),
                ActionButtonKind.DESTRUCTIVE,
                onClick = onDelete,
                modifier = Modifier.weight(1f).semantics { contentDescription = deleteA11y },
                icon = "trash",
            )
        }
    }
}

/** 録音の再生 / 停止と長さ(再生中は位置)。 */
@Composable
fun AudioPlayButton(value: AudioValue, enabled: Boolean = true) {
    val context = LocalContext.current
    val playing = AudioPlayback.isPlaying(value.id)
    var position by remember { mutableDoubleStateOf(0.0) }
    LaunchedEffect(playing) {
        position = 0.0
        while (playing) {
            position = AudioPlayback.position()
            delay(250)
        }
    }
    val seconds = if (playing) position else value.seconds.toDouble()
    ActionButton(
        (if (playing) stringResource(R.string.renderer_audio_stop) else stringResource(R.string.renderer_audio_play)) +
            "  " + AudioRecorderPolicy.elapsedText(seconds),
        ActionButtonKind.PLAIN,
        onClick = { if (playing) AudioPlayback.stop() else AudioPlayback.play(context.applicationContext, value.id) },
        enabled = enabled,
        icon = if (playing) "xmark" else "play",
        fillWidth = false,
    )
}

/** 録音中の音量メーター(0...1)。細い棒 20 本・音量で左から点く。 */
@Composable
fun AudioLevelMeter(level: Double) {
    val colors = RenderTheme.colors
    val bars = 20
    Row(
        Modifier.fillMaxWidth().height(16.dp).clearAndSetSemantics {},
        horizontalArrangement = Arrangement.spacedBy(3.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        repeat(bars) { i ->
            val on = i.toDouble() / bars < level
            Box(
                Modifier
                    .weight(1f)
                    .height((6 + (i % 4) * 3).dp)
                    .clip(CircleShape)
                    .background(if (on) colors.brandA else MaterialTheme.colorScheme.onSurfaceVariant.copy(alpha = 0.2f)),
            )
        }
    }
}

@Composable
internal fun Message(text: String, error: Boolean = false) {
    Text(
        text,
        style = MaterialTheme.typography.labelSmall,
        color = if (error) MaterialTheme.colorScheme.error else MaterialTheme.colorScheme.onSurfaceVariant,
    )
}
