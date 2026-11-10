package jp.kz2601.metamon.switcher

import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import jp.kz2601.metamon.R
import jp.kz2601.metamon.services.AppSummary

/**
 * アプリの切り替え(iOS の AppSwitcherSheet の最小版・#881)。名前の一覧をタップで切り替え、「+ 新しいアプリ」で初期画面へ。
 * 各行の「シェア」で共有シート(#901)。サムネ・並べ替え・名前の変更・削除は子 Issue。
 */
@Composable
fun AppSwitcherDialog(
    apps: List<AppSummary>,
    currentAppId: String,
    onSelect: (String) -> Unit,
    onNewApp: () -> Unit,
    onDismiss: () -> Unit,
    onShare: ((AppSummary) -> Unit)? = null,
) {
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text(stringResource(R.string.switcher_title)) },
        text = {
            if (apps.isEmpty()) {
                Text(stringResource(R.string.switcher_empty))
            } else {
                LazyColumn(Modifier.heightIn(max = 360.dp)) {
                    items(apps, key = { it.id }) { app ->
                        Row(Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
                            Column(
                                Modifier.weight(1f).clickable { onSelect(app.id) }.padding(vertical = 12.dp),
                            ) {
                                Text(app.name.ifEmpty { stringResource(R.string.switcher_untitled) }, style = MaterialTheme.typography.bodyLarge)
                                if (app.id == currentAppId) {
                                    Text(stringResource(R.string.switcher_current), style = MaterialTheme.typography.labelSmall)
                                }
                            }
                            // 共有シート(#901・iOS は各カードの「シェア」)。シェアが使えない構成では出さない
                            if (onShare != null) {
                                TextButton(onClick = { onShare(app) }, modifier = Modifier.testTag("switcher.share.${app.id}")) {
                                    Text(stringResource(R.string.share_action))
                                }
                            }
                        }
                    }
                }
            }
        },
        confirmButton = { TextButton(onClick = onNewApp) { Text(stringResource(R.string.switcher_new_app)) } },
        dismissButton = { TextButton(onClick = onDismiss) { Text(stringResource(R.string.switcher_close)) } },
    )
}
