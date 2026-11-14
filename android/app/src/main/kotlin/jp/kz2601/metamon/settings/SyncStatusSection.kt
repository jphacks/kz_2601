package jp.kz2601.metamon.settings

import androidx.annotation.StringRes
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import jp.kz2601.metamon.R
import jp.kz2601.metamon.renderer.a11y.rememberAnnouncer
import jp.kz2601.metamon.services.local.RecordSyncEngine

/** 設定の「同期」の行の中身(iOS の `SettingsSheet.syncLabel` / `syncColor`)。 */
data class SyncStatusRow(
    @StringRes val label: Int,
    /** 文言の %1$d(件数を出さない状態は null) */
    val count: Int?,
    val tone: Tone,
) {
    /** 丸の色: 最新 = 緑・未送信 / 送信中 = 橙・オフライン = 灰 */
    enum class Tone { SYNCED, PENDING, OFFLINE }

    companion object {
        fun of(status: RecordSyncEngine.Status): SyncStatusRow = when (status) {
            RecordSyncEngine.Status.Synced -> SyncStatusRow(R.string.settings_sync_synced, null, Tone.SYNCED)
            is RecordSyncEngine.Status.Pending -> SyncStatusRow(R.string.settings_sync_pending, status.count, Tone.PENDING)
            is RecordSyncEngine.Status.Syncing -> SyncStatusRow(R.string.settings_sync_sending, status.count, Tone.PENDING)
            is RecordSyncEngine.Status.Offline ->
                if (status.pending == 0) SyncStatusRow(R.string.settings_sync_offline, null, Tone.OFFLINE)
                else SyncStatusRow(R.string.settings_sync_offline_pending, status.pending, Tone.OFFLINE)
        }

        /** 「今すぐ同期」を押せるか(iOS と同じ: オンラインで未送信がある時だけ) */
        fun canSyncNow(isOnline: Boolean, pendingCount: Int): Boolean = isOnline && pendingCount > 0
    }
}

/**
 * 設定の「データ」(#48 / #990・iOS の `SettingsSheet.syncSection`)。同期の状態(最新・未送信 N 件・送信中・オフライン)と「今すぐ同期」。
 * 状態は [RecordSyncEngine.status]。押すと止まっている列も流し直す([RecordSyncEngine.syncNow])。
 */
@Composable
fun SyncStatusSection(sync: RecordSyncEngine) {
    val status by sync.status.collectAsStateWithLifecycle()
    val pending by sync.pendingCount.collectAsStateWithLifecycle()
    val row = SyncStatusRow.of(status)
    val statusText = if (row.count != null) stringResource(row.label, row.count) else stringResource(row.label)
    // 「今すぐ同期」を押した後、送り終えた(最新・オフライン)ら TalkBack に読ませる(H8)。開いただけでは読まない
    val announce = rememberAnnouncer()
    var requested by rememberSaveable { mutableStateOf(false) }
    LaunchedEffect(requested, row.tone) {
        if (requested && row.tone != SyncStatusRow.Tone.PENDING) {
            announce(statusText)
            requested = false
        }
    }
    SettingsHeader(stringResource(R.string.settings_sync_header))
    Row(
        Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 8.dp).semantics(mergeDescendants = true) {}.testTag("settings.sync"),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(8.dp),
    ) {
        Text(stringResource(R.string.settings_sync), modifier = Modifier.weight(1f))
        Box(Modifier.size(8.dp).background(row.tone.color(), CircleShape))
        Text(
            statusText,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
            modifier = Modifier.testTag("settings.sync.status"),
        )
    }
    // オンライン・未送信は status に入っている(到達性は status が Offline かで分かる)
    val online = status !is RecordSyncEngine.Status.Offline
    TextButton(
        onClick = {
            requested = true
            sync.syncNow()
        },
        enabled = SyncStatusRow.canSyncNow(online, pending),
        modifier = Modifier.padding(horizontal = 4.dp).testTag("settings.sync.now"),
    ) { Text(stringResource(R.string.settings_sync_now)) }
    SettingsFooter(stringResource(R.string.settings_sync_footer))
}

private fun SyncStatusRow.Tone.color(): Color = when (this) {
    SyncStatusRow.Tone.SYNCED -> Color(0xFF34C759)
    SyncStatusRow.Tone.PENDING -> Color(0xFFFF9500)
    SyncStatusRow.Tone.OFFLINE -> Color(0xFF8E8E93)
}
