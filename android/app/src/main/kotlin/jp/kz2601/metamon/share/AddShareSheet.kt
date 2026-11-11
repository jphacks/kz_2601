package jp.kz2601.metamon.share

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.material3.Button
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.rememberModalBottomSheetState
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
import androidx.compose.ui.unit.dp
import jp.kz2601.metamon.R
import jp.kz2601.metamon.renderer.a11y.rememberAnnouncer
import jp.kz2601.metamon.services.sharing.PendingAdd
import jp.kz2601.metamon.services.sharing.ShareOpenFailure
import kotlinx.coroutines.launch

/**
 * 記録を共有しないアプリのリンクを開いた時の、取り込む前の確認(iOS の `AddShareSheet`)。
 * 確かめられなかった時(preview なし)は名前なしで聞く。
 * @param onAdd 追加する。失敗したら理由(シートは開いたまま)、うまくいけば null
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
internal fun AddShareSheet(
    add: PendingAdd,
    onAdd: suspend () -> ShareOpenFailure?,
    onDismiss: () -> Unit,
) {
    val scope = rememberCoroutineScope()
    val announce = rememberAnnouncer()
    var busy by remember(add) { mutableStateOf(false) }
    var failure by remember(add) { mutableStateOf<ShareOpenFailure?>(null) }
    val name = add.preview?.name?.takeIf { it.isNotEmpty() }
    val title = if (name != null) stringResource(R.string.share_add_title, name) else stringResource(R.string.share_add_title_unnamed)

    ModalBottomSheet(
        onDismissRequest = { if (!busy) onDismiss() },
        sheetState = rememberModalBottomSheetState(skipPartiallyExpanded = true, confirmValueChange = { !busy }),
        modifier = Modifier.testTag("addShare.sheet"),
    ) {
        Column(
            Modifier.padding(horizontal = 16.dp).padding(bottom = 24.dp),
            verticalArrangement = Arrangement.spacedBy(16.dp),
        ) {
            Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
                Text(title, style = MaterialTheme.typography.titleLarge, modifier = Modifier.testTag("addShare.title"))
                add.preview?.description?.takeIf { it.isNotBlank() }?.let {
                    Text(it, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
                }
            }
            Text(
                stringResource(R.string.share_add_note),
                style = MaterialTheme.typography.bodyMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            failure?.let {
                val text = stringResource(it.textRes)
                LaunchedEffect(it) { announce(text) } // 追加できなかった理由を TalkBack にも読ませる(H8)
                Text(
                    text,
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.error,
                    modifier = Modifier.testTag("addShare.error"),
                )
            }
            Button(
                onClick = {
                    scope.launch {
                        busy = true
                        failure = null
                        try {
                            failure = onAdd()
                        } finally {
                            busy = false
                        }
                    }
                },
                enabled = !busy,
                modifier = Modifier.fillMaxWidth().height(48.dp).testTag("addShare.confirm"),
            ) {
                Row(verticalAlignment = Alignment.CenterVertically) {
                    if (busy) {
                        CircularProgressIndicator(Modifier.size(18.dp), strokeWidth = 2.dp)
                        Spacer(Modifier.size(8.dp))
                    }
                    Text(stringResource(R.string.share_add_confirm))
                }
            }
            TextButton(
                onClick = onDismiss,
                enabled = !busy,
                modifier = Modifier.align(Alignment.CenterHorizontally).testTag("addShare.cancel"),
            ) { Text(stringResource(R.string.share_add_cancel)) }
        }
    }
}
