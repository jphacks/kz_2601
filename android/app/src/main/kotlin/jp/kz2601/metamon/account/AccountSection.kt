package jp.kz2601.metamon.account

import android.app.Activity
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.pluralStringResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import androidx.compose.ui.window.Dialog
import androidx.compose.ui.window.DialogProperties
import jp.kz2601.metamon.AppContainer
import jp.kz2601.metamon.R
import jp.kz2601.metamon.auth.CredentialManagerGoogleSignIn
import jp.kz2601.metamon.auth.GoogleWebClientId
import jp.kz2601.metamon.runtime.UiText
import jp.kz2601.metamon.runtime.resolve
import jp.kz2601.metamon.services.AuthUser
import jp.kz2601.metamon.services.account.SignOutFlow
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.launch

/** 開いているダイアログ。 */
private sealed interface AccountDialog {
    data class SignOut(val prompt: SignOutFlow.Prompt) : AccountDialog
    data class Unsent(val count: Int) : AccountDialog
    data object Delete : AccountDialog
    data class Failed(val title: Int, val message: UiText) : AccountDialog
}

/**
 * 設定の画面(#1144)の「アカウント」(#841・iOS の `AccountSettingsSection`)。ログアウトとアカウント削除。
 * @param busy 生成中(iOS の `canDelete` と同じく削除させない)
 */
@Composable
fun AccountSection(container: AppContainer, user: AuthUser, busy: Boolean) {
    val context = LocalContext.current
    val activity = context as? Activity
    val scope = rememberCoroutineScope()
    // ログアウト・削除は画面より長く生きる scope で回す(サインアウトで本体の画面が消えても途中で止めない)
    val work = container.accountScope
    val session = container.session
    val recordSync = container.recordSync
    val guard = container.accountGuard
    var dialog by remember { mutableStateOf<AccountDialog?>(null) }
    var working by remember { mutableStateOf<Int?>(null) } // 進行中の文言

    fun signOut(choice: SignOutFlow.Choice) {
        dialog = null
        working = R.string.account_signing_out
        work.launch {
            try {
                when (val outcome = session.signOutAndWipe(choice, recordSync, wipe = guard::wipeForSignOut)) {
                    SignOutFlow.Outcome.SignedOut -> Unit
                    is SignOutFlow.Outcome.Unsent -> dialog = AccountDialog.Unsent(outcome.count)
                }
            } catch (e: CancellationException) {
                throw e
            } catch (e: Exception) {
                dialog = AccountDialog.Failed(R.string.account_sign_out_failed, AccountFailureText.of(e))
            } finally {
                working = null
            }
        }
    }

    suspend fun reauthenticate(): Boolean {
        val act = activity ?: return false
        return when (user.provider) {
            AuthUser.GOOGLE -> {
                val clientId = GoogleWebClientId.resolve(act) ?: return false
                session.service.reauthenticateWithGoogle(CredentialManagerGoogleSignIn(act, clientId).idToken())
                true
            }
            AuthUser.APPLE -> {
                session.service.reauthenticateWithApple(act)
                true
            }
            else -> false
        }
    }

    fun deleteAccount() {
        dialog = null
        working = R.string.account_deleting
        work.launch {
            try {
                session.deleteAccount(container.account, reauthenticate = ::reauthenticate, wipe = guard::wipeForSignOut)
            } catch (e: CancellationException) {
                throw e
            } catch (e: Exception) {
                dialog = AccountDialog.Failed(R.string.account_delete_failed, AccountFailureText.of(e))
            } finally {
                working = null
            }
        }
    }

    Text(
        stringResource(R.string.account_menu),
        style = MaterialTheme.typography.titleSmall,
        modifier = Modifier.padding(horizontal = 16.dp, vertical = 8.dp),
    )
    // #1101: uid を保ったまま Google / Apple に引き継ぐ(作ったアプリとデータはそのまま)
    if (user.isAnonymous) AccountLinkSection(session, enabled = working == null)
    TextButton(
        onClick = { scope.launch { dialog = AccountDialog.SignOut(session.signOutPrompt(recordSync)) } },
        enabled = working == null && !busy, // 引き継ぎ・切り替えの最中にサインアウトを重ねない
        modifier = Modifier.padding(horizontal = 4.dp).testTag("settings.account.signOut"),
    ) { Text(stringResource(R.string.account_sign_out)) }
    TextButton(
        onClick = { dialog = AccountDialog.Delete },
        enabled = working == null && !busy,
        modifier = Modifier.padding(horizontal = 4.dp).testTag("settings.account.delete"),
    ) {
        Text(
            stringResource(R.string.account_delete),
            color = if (busy) MaterialTheme.colorScheme.onSurfaceVariant else MaterialTheme.colorScheme.error,
        )
    }

    when (val d = dialog) {
        is AccountDialog.SignOut -> SignOutDialog(d.prompt, user.isAnonymous, onChoose = ::signOut, onDismiss = { dialog = null })
        is AccountDialog.Unsent -> AlertDialog(
            onDismissRequest = { dialog = null },
            title = { Text(stringResource(R.string.account_unsent_title)) },
            text = { Text(pluralStringResource(R.plurals.account_unsent_after_flush, d.count, d.count)) },
            confirmButton = {
                TextButton(onClick = { signOut(SignOutFlow.Choice.DISCARD_AND_WIPE) }) {
                    Text(stringResource(R.string.account_sign_out_without_sending), color = MaterialTheme.colorScheme.error)
                }
            },
            dismissButton = { TextButton(onClick = { dialog = null }) { Text(stringResource(R.string.account_cancel)) } },
        )
        AccountDialog.Delete -> AlertDialog(
            onDismissRequest = { dialog = null },
            title = { Text(stringResource(R.string.account_delete_title)) },
            text = { Text(stringResource(R.string.account_delete_message)) },
            confirmButton = {
                TextButton(onClick = { if (!busy) deleteAccount() }) {
                    Text(stringResource(R.string.account_delete_confirm), color = MaterialTheme.colorScheme.error)
                }
            },
            dismissButton = { TextButton(onClick = { dialog = null }) { Text(stringResource(R.string.account_cancel)) } },
        )
        is AccountDialog.Failed -> AlertDialog(
            onDismissRequest = { dialog = null },
            title = { Text(stringResource(d.title)) },
            text = { Text(d.message.resolve()) },
            confirmButton = { TextButton(onClick = { dialog = null }) { Text(stringResource(R.string.account_ok)) } },
        )
        null -> Unit
    }

    working?.let { label ->
        Dialog(onDismissRequest = {}, properties = DialogProperties(dismissOnBackPress = false, dismissOnClickOutside = false)) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                CircularProgressIndicator()
                Spacer(Modifier.width(16.dp))
                Text(stringResource(label), color = MaterialTheme.colorScheme.onSurface)
            }
        }
    }
}

/** ログアウトの確認(iOS の `SignOutCopy`)。未送信があってオンラインなら「送ってからログアウト」も出す。 */
@Composable
private fun SignOutDialog(prompt: SignOutFlow.Prompt, isAnonymous: Boolean, onChoose: (SignOutFlow.Choice) -> Unit, onDismiss: () -> Unit) {
    val base = stringResource(if (isAnonymous) R.string.account_sign_out_anonymous else R.string.account_sign_out_signed_in)
    val unsent = (prompt as? SignOutFlow.Prompt.Unsent)?.let {
        pluralStringResource(if (it.canSend) R.plurals.account_unsent_will_send else R.plurals.account_unsent_offline, it.count, it.count)
    }
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text(stringResource(R.string.account_sign_out_title)) },
        text = { Text(listOfNotNull(base, unsent).joinToString("\n\n")) },
        confirmButton = {
            Column(horizontalAlignment = Alignment.End) {
                if (prompt is SignOutFlow.Prompt.Unsent && prompt.canSend) {
                    TextButton(onClick = { onChoose(SignOutFlow.Choice.SEND_THEN_WIPE) }) {
                        Text(stringResource(R.string.account_send_then_sign_out))
                    }
                }
                val label = if (prompt is SignOutFlow.Prompt.Unsent) R.string.account_sign_out_without_sending else R.string.account_sign_out
                TextButton(onClick = { onChoose(SignOutFlow.Choice.DISCARD_AND_WIPE) }) {
                    Text(stringResource(label), color = MaterialTheme.colorScheme.error)
                }
            }
        },
        dismissButton = { TextButton(onClick = onDismiss) { Text(stringResource(R.string.account_cancel)) } },
    )
}
