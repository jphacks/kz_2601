package jp.kz2601.metamon.desktop.settings

import jp.kz2601.metamon.desktop.i18n.Lang
import jp.kz2601.metamon.desktop.i18n.Strings
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import java.time.ZoneId
import java.time.ZonedDateTime

// 同期の行・文字(Android の SyncStatusRow と同じ分け方)
class SyncStatusTest {
    private val utc = ZoneId.of("UTC")
    private fun millis(y: Int, mo: Int, d: Int, h: Int, mi: Int) = ZonedDateTime.of(y, mo, d, h, mi, 0, 0, utc).toInstant().toEpochMilli()
    private fun snap(live: Boolean = true, pending: Int = 0, last: Long? = null, paused: Boolean = false) = SyncSnapshot(live, pending, last, paused)

    @Test
    fun syncedWhenNothingWaits() {
        val r = SyncStatus.of(snap())
        assertEquals("desktop_sync_synced", r.labelKey)
        assertEquals(SyncStatusRow.Tone.SYNCED, r.tone)
        assertNull(r.count)
        assertFalse(r.canSyncNow)
        assertNull(r.blockerKey)
    }

    @Test
    fun pendingShowsCountAndAllowsSendNow() {
        val r = SyncStatus.of(snap(pending = 3))
        assertEquals("desktop_sync_pending", r.labelKey)
        assertEquals(3, r.count)
        assertEquals(SyncStatusRow.Tone.PENDING, r.tone)
        assertTrue(r.canSyncNow)
        assertNull(r.blockerKey)
    }

    @Test
    fun pausedExplainsOfflineAndStillAllowsSendNow() {
        val r = SyncStatus.of(snap(pending = 2, paused = true))
        assertEquals("desktop_sync_offline_pending", r.labelKey)
        assertEquals(SyncStatusRow.Tone.OFFLINE, r.tone)
        assertEquals("desktop_sync_blocker_offline", r.blockerKey)
        assertTrue(r.canSyncNow)
    }

    @Test
    fun signedOutCannotSendAndSaysWhy() {
        val r = SyncStatus.of(snap(live = false, pending = 4, paused = true))
        assertEquals("desktop_sync_not_signed_in", r.labelKey)
        assertEquals("desktop_sync_blocker_signed_out", r.blockerKey)
        assertFalse(r.canSyncNow)
    }

    @Test
    fun pausedWithNothingWaitingIsSynced() {
        assertEquals("desktop_sync_synced", SyncStatus.of(snap(pending = 0, paused = true)).labelKey)
    }

    @Test
    fun lastSentTodayIsTimeOnly() = Strings.withLang(Lang.EN) {
        val now = millis(2026, 11, 13, 18, 0)
        assertEquals("Last sent: 14:05", SyncStatus.lastSentText(millis(2026, 11, 13, 14, 5), now, utc))
    }

    @Test
    fun lastSentOtherDayHasDate() = Strings.withLang(Lang.JA) {
        val now = millis(2026, 11, 13, 18, 0)
        assertEquals("最後に送れた時刻: 11/12 09:30", SyncStatus.lastSentText(millis(2026, 11, 12, 9, 30), now, utc))
    }

    @Test
    fun neverSent() = Strings.withLang(Lang.EN) {
        assertEquals("Nothing sent yet", SyncStatus.lastSentText(null, 0, utc))
    }

    @Test
    fun textsInBothLanguages() {
        val s = snap(pending = 2, paused = true)
        val ja = Strings.withLang(Lang.JA) { SyncStatus.texts(s, 0, utc) }
        val en = Strings.withLang(Lang.EN) { SyncStatus.texts(s, 0, utc) }
        assertEquals("送れていません(送信待ち 2 件)", ja.status)
        assertEquals("Not sending (2 waiting to send)", en.status)
        assertNotEquals(ja.blocker, en.blocker)
        assertTrue(ja.canSyncNow && en.canSyncNow)
    }

    @Test
    fun sendNowLabelExistsInBothLanguages() {
        assertEquals("今送る", Strings.withLang(Lang.JA) { Strings["desktop_sync_now"] })
        assertEquals("Send now", Strings.withLang(Lang.EN) { Strings["desktop_sync_now"] })
    }
}
