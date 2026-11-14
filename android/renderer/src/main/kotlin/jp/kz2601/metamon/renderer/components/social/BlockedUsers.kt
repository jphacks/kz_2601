package jp.kz2601.metamon.renderer.components.social

import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import jp.kz2601.metamon.renderer.R
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.launch

/** ブロック中のユーザーの一覧と解除(iOS の `BlockedUsersSettingsSection`・#321)。 */
interface BlockedUsersHost {
    /** uid → 表示名。null = 通報・ブロックが使えない(設定に出さない)。 */
    val blockedUsers: StateFlow<Map<String, String>>?

    suspend fun unblock(uid: String)
}

/** 設定の「ブロック中のユーザー」。通報・ブロックが使える時だけ出す。 */
@Composable
fun BlockedUsersSection(host: BlockedUsersHost, modifier: Modifier = Modifier) {
    val flow = host.blockedUsers ?: return
    val blocked = flow.collectAsState().value
    val scope = rememberCoroutineScope()
    val anonymous = stringResource(R.string.social_anonymous)
    Column(modifier.fillMaxWidth().testTag("settings.blockedUsers")) {
        Text(
            stringResource(R.string.social_blocked_users_header),
            style = MaterialTheme.typography.titleSmall,
            modifier = Modifier.padding(horizontal = 16.dp, vertical = 8.dp),
        )
        if (blocked.isEmpty()) {
            Text(
                stringResource(R.string.social_blocked_users_empty),
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                modifier = Modifier.padding(horizontal = 16.dp, vertical = 8.dp),
            )
        } else {
            blocked.entries.sortedBy { it.value }.forEach { (uid, name) ->
                Row(Modifier.fillMaxWidth().padding(start = 16.dp, end = 4.dp), verticalAlignment = Alignment.CenterVertically) {
                    Text(name.ifEmpty { anonymous }, modifier = Modifier.weight(1f))
                    TextButton(onClick = { scope.launch { host.unblock(uid) } }) { Text(stringResource(R.string.social_unblock)) }
                }
            }
        }
        Text(
            stringResource(R.string.social_blocked_users_footer),
            style = MaterialTheme.typography.bodySmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
            modifier = Modifier.padding(horizontal = 16.dp, vertical = 8.dp),
        )
    }
}
