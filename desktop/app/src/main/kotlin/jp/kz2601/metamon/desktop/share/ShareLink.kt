package jp.kz2601.metamon.desktop.share

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.text.selection.SelectionContainer
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import jp.kz2601.metamon.desktop.PaletteCommand
import jp.kz2601.metamon.desktop.QrClipboard
import jp.kz2601.metamon.desktop.QrImage
import jp.kz2601.metamon.desktop.QrMatrix
import jp.kz2601.metamon.desktop.i18n.Strings
import jp.kz2601.metamon.services.AppConfigClient
import jp.kz2601.metamon.services.Share
import jp.kz2601.metamon.services.sharing.IssuedShares
import jp.kz2601.metamon.services.sharing.ShareFailure
import jp.kz2601.metamon.services.sharing.ShareLinks
import jp.kz2601.metamon.services.sharing.ShareNotice
import jp.kz2601.metamon.services.sharing.ShareSheetModel
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch
import java.awt.GraphicsEnvironment
import java.awt.Toolkit
import java.awt.datatransfer.Clipboard
import java.awt.datatransfer.StringSelection

/** 共有のリンクを作るアプリ(サーバーのアプリ)。 */
data class ShareTarget(val appId: String, val name: String)

/**
 * 共有のリンク(`POST /v1/apps/{appId}/share`・#2503)。発行・取り消しは Android と同じ [ShareSheetModel](android/services)。
 * デスクトップはコマンドパレットから開き、QR とコピーで渡す(送るシートは無いのでコピーまで)。
 */
object ShareLink {
    /** QR とコピーの中身(Android・iOS と同じアプリの URL)。 */
    fun url(share: Share): String = ShareLinks.appUrl(share)

    fun qr(share: Share): QrMatrix? = QrMatrix.encode(url(share))

    /** リンクをクリップボードへ。入れられたか。 */
    fun copy(share: Share, clipboard: Clipboard? = systemClipboard()): Boolean {
        val cb = clipboard ?: return false
        return runCatching { cb.setContents(StringSelection(url(share)), null) }.isSuccess
    }

    fun title(name: String): String = Strings.format("share_title", name.ifBlank { Strings["share_untitled"] })

    fun failureKey(f: ShareFailure): String = when (f) {
        ShareFailure.NOT_MEMBER -> "share_failure_not_member"
        ShareFailure.BLANK_APP -> "share_failure_blank"
        ShareFailure.OFFLINE -> "share_failure_offline"
        ShareFailure.UNAVAILABLE -> "share_failure_unavailable"
        ShareFailure.OTHER -> "share_failure_generic"
    }

    fun noticeKey(n: ShareNotice): String = when (n) {
        ShareNotice.COPIED -> "share_copied"
        ShareNotice.REVOKED -> "share_revoked"
        ShareNotice.REVOKE_FAILED -> "share_revoke_failed"
    }

    private fun systemClipboard(): Clipboard? =
        if (GraphicsEnvironment.isHeadless()) null else runCatching { Toolkit.getDefaultToolkit().systemClipboard }.getOrNull()
}

/**
 * 1 つのウィンドウの共有のリンク。[client] が null(モック)なら「サーバー接続時のみ」を出す。
 * 発行済みのリンクは [issued] に残り、開き直してもトークンを増やさない。
 */
class ShareLinkEntry(val target: ShareTarget, client: AppConfigClient?, issued: IssuedShares) {
    val model = ShareSheetModel(target.appId, client, issued, loadDsl = { null })
    var open by mutableStateOf(false)

    fun command(): PaletteCommand = PaletteCommand(
        "share:link", Strings["share_settings_row"], PaletteCommand.Kind.ACTION,
        keywords = listOf("share", "link", "qr", Strings["share_action"], Strings["share_copy"]),
    ) { open = true }
}

private const val NOTICE_MILLIS = 3_000L

@Composable
fun ShareLinkDialog(entry: ShareLinkEntry, onDismiss: () -> Unit) {
    val model = entry.model
    val state by model.state.collectAsState()
    val scope = rememberCoroutineScope()
    var confirmRevoke by remember { mutableStateOf(false) }
    LaunchedEffect(model) { model.load() }

    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text(ShareLink.title(entry.target.name)) },
        text = {
            Column(Modifier.fillMaxWidth(), horizontalAlignment = Alignment.CenterHorizontally, verticalArrangement = Arrangement.spacedBy(8.dp)) {
                val share = state.share
                val failure = state.failure
                when {
                    share != null -> LinkSection(
                        share,
                        busy = state.busy,
                        onCopy = { if (ShareLink.copy(share)) model.noteCopied() },
                        onRevoke = { confirmRevoke = true },
                    )
                    failure != null -> {
                        Text(Strings[ShareLink.failureKey(failure)], textAlign = TextAlign.Center)
                        if (failure != ShareFailure.UNAVAILABLE) {
                            OutlinedButton(onClick = { scope.launch { model.issue() } }) { Text(Strings["share_retry"]) }
                        }
                    }
                    else -> Box(Modifier.fillMaxWidth().padding(vertical = 32.dp), contentAlignment = Alignment.Center) {
                        val label = Strings["share_creating"]
                        CircularProgressIndicator(Modifier.semantics { contentDescription = label })
                    }
                }
                state.notice?.let { notice ->
                    Text(
                        Strings[ShareLink.noticeKey(notice)],
                        style = MaterialTheme.typography.bodySmall,
                        color = if (notice == ShareNotice.REVOKE_FAILED) MaterialTheme.colorScheme.error else MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                    LaunchedEffect(notice) {
                        delay(NOTICE_MILLIS)
                        model.consumeNotice()
                    }
                }
            }
        },
        confirmButton = { TextButton(onClick = onDismiss) { Text(Strings["share_close"]) } },
    )

    if (confirmRevoke) {
        AlertDialog(
            onDismissRequest = { confirmRevoke = false },
            title = { Text(Strings["share_revoke_title"]) },
            text = { Text(Strings["share_revoke_message"]) },
            confirmButton = {
                TextButton(onClick = {
                    confirmRevoke = false
                    scope.launch { model.revoke() }
                }) { Text(Strings["share_revoke_confirm"], color = MaterialTheme.colorScheme.error) }
            },
            dismissButton = { TextButton(onClick = { confirmRevoke = false }) { Text(Strings["share_revoke_cancel"]) } },
        )
    }
}

@Composable
private fun LinkSection(share: Share, busy: Boolean, onCopy: () -> Unit, onRevoke: () -> Unit) {
    val url = ShareLink.url(share)
    val matrix = remember(url) { ShareLink.qr(share) }
    if (matrix != null) {
        val label = Strings["share_qr_label"]
        QrImage(matrix, Modifier.size(216.dp).semantics { contentDescription = label })
        TextButton(onClick = { QrClipboard.copy(matrix) }) { Text(Strings["desktop_qr_copy_image"]) }
    }
    Text(Strings["share_qr_caption"], style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
    SelectionContainer { Text(url, style = MaterialTheme.typography.bodySmall) }
    Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.SpaceBetween) {
        TextButton(onClick = onCopy, enabled = !busy) { Text(Strings["share_copy"]) }
        TextButton(onClick = onRevoke, enabled = !busy) { Text(Strings["share_revoke"], color = MaterialTheme.colorScheme.error) }
    }
}
