package jp.kz2601.metamon.settings

import jp.kz2601.metamon.R
import jp.kz2601.metamon.services.local.RecordSyncEngine.Status
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

/** #898 設定の「同期」: 最新 / 未送信 N 件 / 送信中 / オフライン(iOS の syncLabel・syncColor と同じ出し分け)。 */
class SyncStatusRowTest {
    @Test
    fun syncedHasNoCount() {
        assertEquals(SyncStatusRow(R.string.settings_sync_synced, null, SyncStatusRow.Tone.SYNCED), SyncStatusRow.of(Status.Synced))
    }

    @Test
    fun pendingAndSendingShowTheCount() {
        assertEquals(SyncStatusRow(R.string.settings_sync_pending, 3, SyncStatusRow.Tone.PENDING), SyncStatusRow.of(Status.Pending(3)))
        assertEquals(SyncStatusRow(R.string.settings_sync_sending, 2, SyncStatusRow.Tone.PENDING), SyncStatusRow.of(Status.Syncing(2)))
    }

    @Test
    fun offlineShowsUnsentOnlyWhenThereAreSome() {
        assertEquals(SyncStatusRow(R.string.settings_sync_offline, null, SyncStatusRow.Tone.OFFLINE), SyncStatusRow.of(Status.Offline(0)))
        assertEquals(SyncStatusRow(R.string.settings_sync_offline_pending, 4, SyncStatusRow.Tone.OFFLINE), SyncStatusRow.of(Status.Offline(4)))
    }

    @Test
    fun syncNowNeedsConnectionAndSomethingToSend() {
        assertTrue(SyncStatusRow.canSyncNow(isOnline = true, pendingCount = 1))
        assertFalse("未送信が無ければ押せない", SyncStatusRow.canSyncNow(isOnline = true, pendingCount = 0))
        assertFalse("オフラインでは押せない", SyncStatusRow.canSyncNow(isOnline = false, pendingCount = 5))
    }
}
