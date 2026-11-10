package jp.kz2601.metamon.account

import android.app.Activity
import android.widget.Toast
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import jp.kz2601.metamon.R
import jp.kz2601.metamon.auth.AuthSession
import jp.kz2601.metamon.auth.CredentialManagerGoogleSignIn
import jp.kz2601.metamon.auth.GoogleWebClientId
import jp.kz2601.metamon.auth.LinkProblem
import jp.kz2601.metamon.services.AuthUser
import kotlinx.coroutines.launch

/**
 * 匿名の人に出す「Google / Apple で続けて引き継ぐ」(#1101・iOS の `AppleLinkButton` と設定のフッター)。
 * Firebase の link なので uid は変わらず、作ったアプリとデータはそのまま。既に別のアカウントにある資格情報なら切り替えるかを確認する。
 * 切り替えると uid が変わり、匿名の時の端末のデータは持ち主の見張り(`AccountGuard`)が消す。
 */
@Composable
fun AccountLinkSection(session: AuthSession) {
    val context = LocalContext.current
    val activity = context as? Activity
    val scope = rememberCoroutineScope()
    val webClientId = remember(context) { GoogleWebClientId.resolve(context) }
    val busy by session.isBusy.collectAsStateWithLifecycle()
    val conflict by session.linkConflict.collectAsStateWithLifecycle()
    val problem by session.linkError.collectAsStateWithLifecycle()
    val doneGoogle = stringResource(R.string.account_link_done_google)
    val doneApple = stringResource(R.string.account_link_done_apple)

    Column(
        modifier = Modifier.padding(horizontal = 16.dp, vertical = 4.dp),
        verticalArrangement = Arrangement.spacedBy(8.dp),
    ) {
        Text(stringResource(R.string.account_link_status_anonymous), style = MaterialTheme.typography.bodyMedium)
        // Web クライアント ID が無いビルドでは Google のボタンを出さない(サインイン画面と同じ)
        if (activity != null && webClientId != null) {
            Button(
                onClick = {
                    scope.launch {
                        if (session.linkWithGoogle(CredentialManagerGoogleSignIn(activity, webClientId))) {
                            Toast.makeText(context, doneGoogle, Toast.LENGTH_LONG).show()
                        }
                    }
                },
                enabled = !busy,
                modifier = Modifier.fillMaxWidth().testTag("settings.account.linkGoogle"),
            ) { Text(stringResource(R.string.account_link_google)) }
        }
        if (activity != null) {
            OutlinedButton(
                onClick = {
                    scope.launch {
                        if (session.linkWithApple(activity)) Toast.makeText(context, doneApple, Toast.LENGTH_LONG).show()
                    }
                },
                enabled = !busy,
                modifier = Modifier.fillMaxWidth().testTag("settings.account.linkApple"),
            ) { Text(stringResource(R.string.account_link_apple)) }
        }
        problem?.let {
            Text(linkProblemText(it), color = MaterialTheme.colorScheme.error, style = MaterialTheme.typography.bodySmall)
        }
        Text(
            stringResource(R.string.account_link_footer),
            style = MaterialTheme.typography.bodySmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
    }

    conflict?.let { provider ->
        val apple = provider == AuthUser.APPLE
        AlertDialog(
            onDismissRequest = session::dismissLinkConflict,
            title = {
                Text(stringResource(if (apple) R.string.account_link_conflict_apple_title else R.string.account_link_conflict_google_title))
            },
            text = {
                Text(stringResource(if (apple) R.string.account_link_conflict_apple_message else R.string.account_link_conflict_google_message))
            },
            confirmButton = {
                TextButton(onClick = { scope.launch { session.switchToExistingAccount() } }) {
                    Text(stringResource(R.string.account_link_switch), color = MaterialTheme.colorScheme.error)
                }
            },
            dismissButton = {
                TextButton(onClick = session::dismissLinkConflict) { Text(stringResource(R.string.account_cancel)) }
            },
        )
    }
}

@Composable
private fun linkProblemText(problem: LinkProblem): String = when (problem) {
    LinkProblem.AlreadyLinked -> stringResource(R.string.account_link_already_linked)
    LinkProblem.NoPendingAccount -> stringResource(R.string.account_link_no_pending)
    LinkProblem.Failed -> stringResource(R.string.account_link_failed)
}
