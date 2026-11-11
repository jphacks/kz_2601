package jp.kz2601.metamon.share

import android.content.Intent
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/** シェアリンクの intent の受け口(#901)。 */
class ShareLinkIntentsTest {
    @After
    fun tearDown() = ShareLinkIntents.inbox.clear()

    @Test
    fun viewIntentIsHeld() {
        assertTrue(ShareLinkIntents.offer(Intent.ACTION_VIEW, "tailorit://import/tok", 0))
        assertEquals("tok", ShareLinkIntents.inbox.pending.value)
    }

    @Test
    fun relaunchFromRecentsDoesNotReopenTheOldLink() {
        // 最近使ったアプリから開き直すと、前に開いたリンクの intent がそのまま届く
        assertFalse(ShareLinkIntents.offer(Intent.ACTION_VIEW, "tailorit://import/old", Intent.FLAG_ACTIVITY_LAUNCHED_FROM_HISTORY))
        assertNull(ShareLinkIntents.inbox.pending.value)
    }

    @Test
    fun otherActionsAreIgnored() {
        assertFalse(ShareLinkIntents.offer(Intent.ACTION_MAIN, "tailorit://import/tok", 0))
        assertFalse(ShareLinkIntents.offer(null, "tailorit://import/tok", 0))
        assertNull(ShareLinkIntents.inbox.pending.value)
    }
}
