package jp.kz2601.metamon.renderer.components.importing

import android.content.res.Resources
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.LinearProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import jp.kz2601.metamon.renderer.R
import jp.kz2601.metamon.renderer.components.actions.ActionButton
import jp.kz2601.metamon.renderer.components.actions.ActionButtonKind
import jp.kz2601.metamon.renderer.theme.RenderTheme
import kotlin.math.roundToInt

/** 文字起こし中の進み具合とキャンセル(iOS の `FileImportAudioProgress`)。前面にある間だけ進むことを出す。 */
@Composable
internal fun FileImportAudioProgress(progress: Double, cancel: () -> Unit) {
    Column(verticalArrangement = Arrangement.spacedBy(RenderTheme.Spacing.xs)) {
        Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.SpaceBetween) {
            Text(stringResource(R.string.renderer_import_audio_progress), style = MaterialTheme.typography.bodyMedium)
            Text(stringResource(R.string.renderer_import_audio_percent, (progress * 100).roundToInt()), style = MaterialTheme.typography.bodyMedium)
        }
        LinearProgressIndicator(progress = { progress.toFloat() }, modifier = Modifier.fillMaxWidth().testTag("fileImport.audioProgress"))
        Text(
            stringResource(R.string.renderer_import_audio_foreground),
            style = MaterialTheme.typography.bodySmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
        ActionButton(
            label = stringResource(R.string.renderer_import_audio_cancel),
            kind = ActionButtonKind.SECONDARY,
            onClick = cancel,
            modifier = Modifier.testTag("fileImport.audioCancel"),
        )
    }
}

/**
 * 録音ファイルを文字にする音声モデルの具合(用意できていない時だけ出す・iOS の `FileImportAudioModelRow`)。
 * 落とす前に大きさを示して確かめる(モバイル回線で勝手に 60MB を落とさない)。
 */
@Composable
internal fun FileImportAudioModelRow(host: AudioFileTranscriptionHost?) {
    val note: @Composable (String) -> Unit = {
        Text(it, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
    }
    if (host == null) {
        note(stringResource(R.string.renderer_import_audio_no_host))
        return
    }
    val readiness by host.audioFileReadiness.collectAsState()
    var confirming by remember { mutableStateOf(false) }
    when (val r = readiness) {
        AudioModelReadiness.Ready -> Unit
        AudioModelReadiness.Unavailable -> note(stringResource(R.string.renderer_import_audio_unsupported))
        is AudioModelReadiness.Preparing -> {
            val fraction = r.fraction
            if (fraction == null) {
                note(stringResource(R.string.renderer_import_audio_preparing))
                LinearProgressIndicator(Modifier.fillMaxWidth().testTag("fileImport.audioModelProgress"))
            } else {
                note(stringResource(R.string.renderer_import_audio_downloading, (fraction * 100).roundToInt()))
                LinearProgressIndicator(progress = { fraction }, modifier = Modifier.fillMaxWidth().testTag("fileImport.audioModelProgress"))
            }
        }
        AudioModelReadiness.NeedsModel -> ActionButton(
            label = stringResource(R.string.renderer_import_audio_prepare),
            kind = ActionButtonKind.SECONDARY,
            onClick = { confirming = true },
            modifier = Modifier.testTag("fileImport.audioPrepare"),
        )
        is AudioModelReadiness.Failed -> {
            note(failureText(r.reason, host.modelMegabytes))
            ActionButton(
                label = stringResource(R.string.renderer_import_audio_retry),
                kind = ActionButtonKind.SECONDARY,
                onClick = { confirming = true },
                modifier = Modifier.testTag("fileImport.audioPrepare"),
            )
        }
    }
    if (confirming) {
        AlertDialog(
            onDismissRequest = { confirming = false },
            title = { Text(stringResource(R.string.renderer_import_audio_download_title)) },
            text = { Text(stringResource(R.string.renderer_import_audio_download_message, host.modelMegabytes)) },
            confirmButton = {
                TextButton(
                    onClick = {
                        confirming = false
                        host.prepareAudioFileTranscription()
                    },
                    modifier = Modifier.testTag("fileImport.audioDownload"),
                ) { Text(stringResource(R.string.renderer_import_audio_download)) }
            },
            dismissButton = { TextButton(onClick = { confirming = false }) { Text(stringResource(R.string.renderer_import_cancel)) } },
        )
    }
}

@Composable
private fun failureText(reason: AudioModelFailure, megabytes: Int): String = when (reason) {
    AudioModelFailure.NETWORK -> stringResource(R.string.renderer_import_audio_failed_network)
    AudioModelFailure.STORAGE -> stringResource(R.string.renderer_import_audio_failed_storage, megabytes)
    AudioModelFailure.CHECKSUM -> stringResource(R.string.renderer_import_audio_failed_checksum)
    AudioModelFailure.OTHER -> stringResource(R.string.renderer_import_audio_failed_other)
}

/** 「12 分 30 秒」のような長さ(iOS の `AudioFileImport.durationText`)。 */
internal fun durationText(res: Resources, seconds: Double): String {
    val (m, s) = AudioFileImport.minutesAndSeconds(seconds)
    return if (m > 0) res.getString(R.string.renderer_import_audio_minutes_seconds, m, s) else res.getString(R.string.renderer_import_audio_seconds, s)
}

/** 下見の問題 → 文。 */
internal fun audioProblemText(res: Resources, p: AudioFileImport.Problem): String = when (p) {
    AudioFileImport.Problem.TOO_LARGE -> res.getString(R.string.renderer_import_problem_too_large, (AudioFileImport.MAX_BYTES / 1_000_000).toInt())
    AudioFileImport.Problem.TOO_LONG -> {
        val minutes = (AudioFileImport.MAX_SECONDS / 60).toInt()
        res.getQuantityString(R.plurals.renderer_import_audio_too_long, minutes, minutes)
    }
    AudioFileImport.Problem.UNREADABLE, AudioFileImport.Problem.EMPTY -> res.getString(R.string.renderer_import_audio_unreadable)
}
