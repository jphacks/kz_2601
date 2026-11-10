package jp.kz2601.metamon.share

import android.content.ClipData
import android.content.ClipboardManager
import android.content.Context
import android.content.Intent
import androidx.compose.foundation.Image
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.rememberModalBottomSheetState
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.FilterQuality
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import jp.kz2601.metamon.R
import jp.kz2601.metamon.renderer.components.actions.QrMatrix
import jp.kz2601.metamon.renderer.components.actions.bitmap
import jp.kz2601.metamon.services.Share
import jp.kz2601.metamon.services.sharing.ShareFailure
import jp.kz2601.metamon.services.sharing.ShareLinks
import jp.kz2601.metamon.services.sharing.ShareNotice
import jp.kz2601.metamon.services.sharing.ShareScope
import jp.kz2601.metamon.services.sharing.ShareSheetModel
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch

/** 共有シートを開く対象(変身一覧の行・設定の「このアプリをシェア」)。 */
data class ShareTarget(val appId: String, val name: String)

/**
 * アプリの共有シート(iOS の `ShareAppSheet` の上半分・#901): リンクの発行・QR・送る(ACTION_SEND)・コピー・取り消し。
 * 「探す」への公開(下半分)は別 Issue。リンクから開く・参加する側は [ShareLinkHost](App Links はまだ無い)。
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun ShareSheet(target: ShareTarget, model: ShareSheetModel, onDismiss: () -> Unit) {
    val state by model.state.collectAsStateWithLifecycle()
    val scope = rememberCoroutineScope()
    val context = LocalContext.current
    var confirmRevoke by rememberSaveable { mutableStateOf(false) }
    LaunchedEffect(model) { model.load() }

    val name = target.name.ifEmpty { stringResource(R.string.share_untitled) }
    ModalBottomSheet(
        onDismissRequest = onDismiss,
        sheetState = rememberModalBottomSheetState(skipPartiallyExpanded = true),
        modifier = Modifier.testTag("share.sheet"),
    ) {
        Column(Modifier.verticalScroll(rememberScrollState()).padding(horizontal = 16.dp).padding(bottom = 24.dp)) {
            Row(verticalAlignment = Alignment.Top) {
                Column(Modifier.weight(1f)) {
                    Text(stringResource(R.string.share_title, name), style = MaterialTheme.typography.titleLarge)
                    state.scope?.let { Text(stringResource(it.textRes), style = MaterialTheme.typography.bodySmall) }
                }
                TextButton(onClick = onDismiss, modifier = Modifier.testTag("share.close")) { Text(stringResource(R.string.share_close)) }
            }
            Spacer(Modifier.size(16.dp))
            val share = state.share
            val failure = state.failure
            when {
                share != null -> LinkSection(
                    share = share,
                    busy = state.busy,
                    onSend = { context.sendShare(name, share) },
                    onCopy = {
                        context.copyLink(ShareLinks.appUrl(share))
                        model.noteCopied()
                    },
                    onRevoke = { confirmRevoke = true },
                )
                failure != null -> Column(Modifier.fillMaxWidth(), horizontalAlignment = Alignment.CenterHorizontally) {
                    Text(stringResource(failure.textRes), textAlign = TextAlign.Center, modifier = Modifier.testTag("share.error"))
                    if (failure != ShareFailure.UNAVAILABLE) {
                        OutlinedButton(onClick = { scope.launch { model.issue() } }, modifier = Modifier.padding(top = 8.dp)) {
                            Text(stringResource(R.string.share_retry))
                        }
                    }
                }
                else -> Box(Modifier.fillMaxWidth().padding(vertical = 48.dp), contentAlignment = Alignment.Center) {
                    val label = stringResource(R.string.share_creating)
                    CircularProgressIndicator(Modifier.semantics { contentDescription = label })
                }
            }
            state.notice?.let { notice ->
                Text(
                    stringResource(notice.textRes),
                    style = MaterialTheme.typography.bodySmall,
                    color = if (notice == ShareNotice.REVOKE_FAILED) MaterialTheme.colorScheme.error else MaterialTheme.colorScheme.onSurfaceVariant,
                    modifier = Modifier.fillMaxWidth().padding(top = 12.dp).testTag("share.notice"),
                    textAlign = TextAlign.Center,
                )
                LaunchedEffect(notice) {
                    delay(NOTICE_MILLIS)
                    model.consumeNotice()
                }
            }
        }
    }

    if (confirmRevoke) {
        AlertDialog(
            onDismissRequest = { confirmRevoke = false },
            title = { Text(stringResource(R.string.share_revoke_title)) },
            text = { Text(stringResource(R.string.share_revoke_message)) },
            confirmButton = {
                TextButton(onClick = {
                    confirmRevoke = false
                    scope.launch { model.revoke() }
                }, modifier = Modifier.testTag("share.revoke.confirm")) {
                    Text(stringResource(R.string.share_revoke_confirm), color = MaterialTheme.colorScheme.error)
                }
            },
            dismissButton = { TextButton(onClick = { confirmRevoke = false }) { Text(stringResource(R.string.share_revoke_cancel)) } },
        )
    }
}

@Composable
private fun LinkSection(share: Share, busy: Boolean, onSend: () -> Unit, onCopy: () -> Unit, onRevoke: () -> Unit) {
    val appUrl = ShareLinks.appUrl(share)
    Column(Modifier.fillMaxWidth(), horizontalAlignment = Alignment.CenterHorizontally) {
        // ダークモードでも白地に黒(読み取りやすさ優先・QrCodeLeaf と同じ)
        val qr = remember(appUrl) { QrMatrix.encode(appUrl)?.bitmap() }
        val qrLabel = stringResource(R.string.share_qr_label)
        if (qr != null) {
            Image(
                bitmap = qr,
                contentDescription = qrLabel,
                filterQuality = FilterQuality.None,
                contentScale = ContentScale.Fit,
                modifier = Modifier.size(216.dp).background(Color.White, RoundedCornerShape(12.dp)).padding(8.dp).testTag("share.qr"),
            )
        }
        Text(
            stringResource(R.string.share_qr_caption),
            style = MaterialTheme.typography.bodySmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
            modifier = Modifier.padding(top = 8.dp),
        )
        Button(onClick = onSend, modifier = Modifier.fillMaxWidth().padding(top = 16.dp).testTag("share.send")) {
            Text(stringResource(R.string.share_send))
        }
        Row(Modifier.fillMaxWidth().padding(top = 8.dp), horizontalArrangement = Arrangement.SpaceBetween) {
            TextButton(onClick = onCopy, enabled = !busy, modifier = Modifier.testTag("share.copy")) { Text(stringResource(R.string.share_copy)) }
            TextButton(onClick = onRevoke, enabled = !busy, modifier = Modifier.testTag("share.revoke")) {
                Text(stringResource(R.string.share_revoke), color = MaterialTheme.colorScheme.error)
            }
        }
    }
}

/**
 * 設定の「このアプリをシェア」の行(#901)。シェアが使えない時・まだ中身の無いアプリでは出さない(呼び出し側が null を渡す)。
 */
@Composable
fun ShareSettingsRow(onShare: (() -> Unit)?) {
    onShare ?: return
    Column(Modifier.fillMaxWidth().clickable(onClick = onShare).padding(horizontal = 16.dp, vertical = 12.dp).testTag("settings.share")) {
        Text(stringResource(R.string.share_settings_row), style = MaterialTheme.typography.bodyLarge)
        Text(
            stringResource(R.string.share_settings_footer),
            style = MaterialTheme.typography.bodySmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
    }
}

/** Android の共有シート(ACTION_SEND)。本文にアプリの URL、続けてブラウザの URL(iOS の ShareLink と同じ中身)。 */
private fun Context.sendShare(name: String, share: Share) {
    val message = getString(R.string.share_send_message, name, ShareLinks.appUrl(share))
    val send = Intent(Intent.ACTION_SEND).apply {
        type = "text/plain"
        putExtra(Intent.EXTRA_SUBJECT, name)
        putExtra(Intent.EXTRA_TEXT, ShareLinks.sendText(message, share))
    }
    startActivity(Intent.createChooser(send, getString(R.string.share_send)).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK))
}

private fun Context.copyLink(url: String) {
    val clipboard = getSystemService(ClipboardManager::class.java) ?: return
    clipboard.setPrimaryClip(ClipData.newPlainText(getString(R.string.share_qr_label), url))
}

private val ShareScope.textRes: Int
    get() = when (this) {
        ShareScope.COPY -> R.string.share_scope_copy
        ShareScope.PUBLIC -> R.string.share_scope_public
        ShareScope.MEMBERS -> R.string.share_scope_members
    }

private val ShareFailure.textRes: Int
    get() = when (this) {
        ShareFailure.NOT_MEMBER -> R.string.share_failure_not_member
        ShareFailure.BLANK_APP -> R.string.share_failure_blank
        ShareFailure.OFFLINE -> R.string.share_failure_offline
        ShareFailure.UNAVAILABLE -> R.string.share_failure_unavailable
        ShareFailure.OTHER -> R.string.share_failure_generic
    }

private val ShareNotice.textRes: Int
    get() = when (this) {
        ShareNotice.COPIED -> R.string.share_copied
        ShareNotice.REVOKED -> R.string.share_revoked
        ShareNotice.REVOKE_FAILED -> R.string.share_revoke_failed
    }

private const val NOTICE_MILLIS = 3_000L
