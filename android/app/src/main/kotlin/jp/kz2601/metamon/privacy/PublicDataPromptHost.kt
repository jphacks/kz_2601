package jp.kz2601.metamon.privacy

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
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import jp.kz2601.metamon.R
import jp.kz2601.metamon.language.AppLocales
import jp.kz2601.metamon.services.privacy.PublicDataPrompt
import kotlinx.coroutines.delay

/**
 * 生成直後の「個人の情報も公開されます」の確認(#253・#899・iOS の `PublicDataPromptHost`)。
 * ai が `meta.publicPersonalData` を返した時だけ出す。お知らせの順番(要約の後・[jp.kz2601.metamon.voice.stage.PostBuildNotice])で
 * 自分の番の時だけ、生成中・音声のシートの間を避けて、少し待ってから出す。
 *
 * @param onRemove 「この欄を消す」(英語の指示文にするか)
 * @param onPrivate 「自分だけにする」(英語の指示文にするか)
 * @param onKeep 「公開のままにする」・閉じる
 */
@Composable
fun PublicDataPromptHost(
    prompt: PublicDataPrompt?,
    busy: Boolean,
    turn: Boolean,
    onRemove: (Boolean) -> Unit,
    onPrivate: (Boolean) -> Unit,
    onKeep: () -> Unit,
) {
    var shown by remember { mutableStateOf(false) }
    LaunchedEffect(prompt, busy, turn) {
        shown = false
        if (prompt == null || busy || !turn) return@LaunchedEffect
        delay(PROMPT_DELAY_MILLIS)
        shown = true
    }
    if (!shown || prompt == null) return
    val english = AppLocales.languageCode(LocalContext.current) == "en"
    AlertDialog(
        onDismissRequest = onKeep,
        modifier = Modifier.testTag("publicData.prompt"),
        title = { Text(stringResource(R.string.public_data_title)) },
        text = { Text(stringResource(R.string.public_data_message, prompt.fieldNames(english))) },
        confirmButton = {
            TextButton(onClick = { onRemove(english) }, modifier = Modifier.testTag("publicData.remove")) {
                Text(stringResource(R.string.public_data_remove), color = MaterialTheme.colorScheme.error)
            }
        },
        dismissButton = {
            Row {
                TextButton(onClick = { onPrivate(english) }, modifier = Modifier.testTag("publicData.private")) {
                    Text(stringResource(R.string.public_data_private))
                }
                TextButton(onClick = onKeep, modifier = Modifier.testTag("publicData.keep")) { Text(stringResource(R.string.public_data_keep)) }
            }
        },
    )
}

/** 生成直後は音声のシートが閉じる途中なので少し待つ(iOS は 700ms) */
private const val PROMPT_DELAY_MILLIS = 700L
