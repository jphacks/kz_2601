package jp.kz2601.metamon.retention

import androidx.compose.foundation.layout.Row
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalResources
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import jp.kz2601.metamon.R
import jp.kz2601.metamon.language.AppLocales
import jp.kz2601.metamon.runtime.AppRuntime
import jp.kz2601.metamon.runtime.RetentionPrompt
import kotlinx.coroutines.delay

/** 今のアプリの保存期間の確認が待っているか(完成の後のお知らせの順番に入れる・#1271)。 */
@Composable
fun retentionPending(runtime: AppRuntime): Boolean = currentRetentionPrompt(runtime) != null

@Composable
private fun currentRetentionPrompt(runtime: AppRuntime): RetentionPrompt? {
    val retention = runtime.retention ?: return null
    val prompt by retention.prompt.collectAsStateWithLifecycle()
    val runtimeState by runtime.state.collectAsStateWithLifecycle()
    return prompt?.takeIf { it.appId == runtimeState.document.appId }
}

/**
 * 記録の保存期間の確認(#482・iOS の `RetentionPromptHost`)。「自動で消す」/「自動で消すのをやめる」/「あとで」。
 * 完成の後のお知らせの順番(要約 → 保存期間 → 直し方の例・[jp.kz2601.metamon.voice.stage.PostBuildNotice])で自分の番の時だけ、
 * 生成中・音声のシートの間を避けて、少し待ってから出す(#1271)。
 *
 * @param busy 生成中・音声のシートを開いている
 * @param turn お知らせの順番で保存期間の番(前の要約が閉じている)
 * @param onStop 「自動で消すのをやめる」の指示文を送る(声で言うのと同じ既存の修正)
 */
@Composable
fun RetentionPromptHost(runtime: AppRuntime, busy: Boolean, turn: Boolean, onStop: (String) -> Unit) {
    val retention = runtime.retention ?: return
    var shown by remember { mutableStateOf(false) }
    val current = currentRetentionPrompt(runtime)
    LaunchedEffect(current?.id, busy, turn) {
        shown = false
        if (current == null || busy || !turn) return@LaunchedEffect
        delay(PROMPT_DELAY_MILLIS)
        shown = true
    }
    if (!shown || current == null) return
    val context = LocalContext.current
    val resources = LocalResources.current
    val english = AppLocales.languageCode(context) == "en"
    AlertDialog(
        onDismissRequest = retention::defer,
        modifier = Modifier.testTag("retention.prompt"),
        title = { Text(stringResource(R.string.retention_title)) },
        text = { Text(current.message(english) { id, args -> resources.getString(id, *args) }) },
        confirmButton = {
            TextButton(
                onClick = {
                    retention.confirm()
                    runtime.sweepRetention()
                },
                modifier = Modifier.testTag("retention.confirm"),
            ) { Text(stringResource(R.string.retention_confirm), color = MaterialTheme.colorScheme.error) }
        },
        dismissButton = {
            Row {
                TextButton(onClick = {
                    retention.dismiss()
                    onStop(resources.getString(R.string.retention_stop_instruction)) // 声で言うのと同じ(iOS の `removeRetention`)
                }) { Text(stringResource(R.string.retention_stop)) }
                TextButton(onClick = retention::defer) { Text(stringResource(R.string.retention_later)) }
            }
        },
    )
}

private const val PROMPT_DELAY_MILLIS = 900L
