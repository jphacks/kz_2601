package jp.kz2601.metamon.desktop.share

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.semantics.LiveRegionMode
import androidx.compose.ui.semantics.liveRegion
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import jp.kz2601.metamon.desktop.i18n.Strings
import jp.kz2601.metamon.services.AppConfigClient
import jp.kz2601.metamon.services.InstallResult
import jp.kz2601.metamon.services.SharePreview
import jp.kz2601.metamon.services.http.ApiError
import jp.kz2601.metamon.services.http.FailureKind
import jp.kz2601.metamon.services.http.SharedAccessDenial
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.launch

/** シェアリンクの確かめと取り込み(`GET /v1/shares/{token}`・`POST` の import)。[AppConfigClient] の 2 つだけを使う。 */
interface ShareImporter {
    suspend fun preview(token: String): SharePreview

    suspend fun import(token: String): InstallResult

    companion object {
        fun of(client: AppConfigClient): ShareImporter = object : ShareImporter {
            override suspend fun preview(token: String) = client.previewShare(token)
            override suspend fun import(token: String) = client.importShare(token, displayName = null)
        }
    }
}

/** 開けなかった理由(Android の `ShareOpenFailure` と同じ分け方)。文言は Android のキー。 */
enum class AddShareFailure(val key: String) {
    UNAVAILABLE("share_open_unavailable"),
    REVOKED("share_open_revoked"),
    INVITER_LEFT("share_open_inviter_left"),
    MEMBERS_FULL("share_open_members_full"),
    OFFLINE("share_open_offline"),
    OTHER("share_open_failed"),
    ;

    companion object {
        fun of(e: Throwable): AddShareFailure {
            when (SharedAccessDenial.from(e)) {
                SharedAccessDenial.INVITER_LEFT -> return INVITER_LEFT
                SharedAccessDenial.MEMBERS_FULL -> return MEMBERS_FULL
                else -> Unit
            }
            if ((e as? ApiError)?.status == 404) return REVOKED
            return when (FailureKind.classify(e)) {
                FailureKind.Offline, FailureKind.Timeout -> OFFLINE
                else -> OTHER
            }
        }
    }
}

/** 記録を共有しないアプリのリンクを開いた時の、取り込む前の確認。[preview] = null は確かめられなかった時(名前なしで聞く)。 */
data class PendingAddShare(val token: String, val preview: SharePreview?) {
    /** 「〈名前〉を追加しますか?」。名前が無ければ「シェアされたアプリを追加しますか?」 */
    fun title(): String = preview?.name?.takeIf { it.isNotBlank() }?.let { Strings.format("share_add_title", it) }
        ?: Strings["share_add_title_unnamed"]

    val description: String? get() = preview?.description?.takeIf { it.isNotBlank() }
}

/**
 * シェアリンクを開く(#3364・iOS の `AddShareSheet`・Android の `ShareLinkOpener` の追加の側)。
 * 記録を共有するアプリ(自分のものでない)は参加の流れ([Opened.Join])へそのまま渡し、それ以外は取り込む前に聞く。
 * @param importer null = サーバーにつながっていない構成(モック)
 */
class AddShareFlow(private val importer: ShareImporter?) {
    sealed interface Opened {
        /** 記録を共有するアプリ。既存の参加の流れへ */
        data class Join(val token: String, val preview: SharePreview) : Opened
        data class AskToAdd(val pending: PendingAddShare) : Opened
        data class Failed(val reason: AddShareFailure) : Opened
    }

    sealed interface Added {
        /** 取り込んだ。このアプリを開く */
        data class Done(val app: InstallResult) : Added
        data class Failed(val reason: AddShareFailure) : Added
    }

    suspend fun open(token: String): Opened {
        val importer = importer ?: return Opened.Failed(AddShareFailure.UNAVAILABLE)
        val preview = try {
            importer.preview(token)
        } catch (e: CancellationException) {
            throw e
        } catch (_: Exception) {
            null
        }
        if (preview != null && preview.sharedData && !preview.mine) return Opened.Join(token, preview)
        return Opened.AskToAdd(PendingAddShare(token, preview))
    }

    /** 確認の「追加する」。 */
    suspend fun add(pending: PendingAddShare): Added {
        val importer = importer ?: return Added.Failed(AddShareFailure.UNAVAILABLE)
        return try {
            Added.Done(importer.import(pending.token))
        } catch (e: CancellationException) {
            throw e
        } catch (e: Exception) {
            Added.Failed(AddShareFailure.of(e))
        }
    }
}

/**
 * 取り込む前の確認のダイアログ(iOS の `AddShareSheet`)。「追加する」で取り込み、うまくいけば [onAdded](そのアプリを開く)。
 * 失敗したら理由を出して開いたまま(読み上げる)。「追加しない」・Esc・外のクリックでやめる(取り込まない)。
 */
@Composable
fun AddShareDialog(pending: PendingAddShare, flow: AddShareFlow, onAdded: (InstallResult) -> Unit, onDismiss: () -> Unit) {
    val scope = rememberCoroutineScope()
    var busy by remember(pending) { mutableStateOf(false) }
    var failure by remember(pending) { mutableStateOf<AddShareFailure?>(null) }
    AlertDialog(
        onDismissRequest = { if (!busy) onDismiss() },
        title = { Text(pending.title()) },
        text = {
            Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                pending.description?.let { Text(it, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant) }
                Text(Strings["share_add_note"], style = MaterialTheme.typography.bodyMedium)
                failure?.let {
                    Text(
                        Strings[it.key],
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.error,
                        modifier = Modifier.semantics { liveRegion = LiveRegionMode.Polite },
                    )
                }
            }
        },
        confirmButton = {
            Button(
                enabled = !busy,
                onClick = {
                    scope.launch {
                        busy = true
                        failure = null
                        try {
                            when (val r = flow.add(pending)) {
                                is AddShareFlow.Added.Done -> onAdded(r.app)
                                is AddShareFlow.Added.Failed -> failure = r.reason
                            }
                        } finally {
                            busy = false
                        }
                    }
                },
            ) { Text(Strings["share_add_confirm"]) }
        },
        dismissButton = { TextButton(onClick = onDismiss, enabled = !busy) { Text(Strings["share_add_cancel"]) } },
    )
}
