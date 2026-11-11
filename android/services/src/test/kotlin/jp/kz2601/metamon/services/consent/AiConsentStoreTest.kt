package jp.kz2601.metamon.services.consent

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import java.time.Instant

/** 同意の保存と版の判定(iOS の AIConsentStoreTests と同じ筋)。 */
class AiConsentStoreTest {
    private val now = Instant.parse("2026-11-11T03:00:00Z")
    private val prefs = InMemoryKeyValueStore()
    private fun store() = AiConsentStore(prefs) { now }

    @Test
    fun firstLaunchIsNotPromptedAndAsks() {
        val s = store()
        assertEquals(AiConsentStatus.NOT_PROMPTED, s.state.value.status)
        assertFalse(s.isGranted)
        assertTrue(s.shouldPromptOnLaunch)
    }

    @Test
    fun laterDoesNotAskOnLaunchAgain() {
        val s = store()
        s.markPrompted()
        assertEquals(AiConsentStatus.DEFERRED, s.state.value.status)
        assertFalse(s.shouldPromptOnLaunch)
        assertFalse(store().shouldPromptOnLaunch) // 保存される
    }

    @Test
    fun grantSavesCurrentVersionAndDate() {
        store().grant()
        val s = store()
        assertTrue(s.isGranted)
        assertEquals(now, s.state.value.grantedAt)
        assertEquals(AiConsentStore.CURRENT_VERSION, prefs.getInt(AiConsentStore.VERSION_KEY))
        assertFalse(s.shouldPromptOnLaunch)
    }

    @Test
    fun revokeStopsSendingAndDoesNotAskOnLaunch() {
        val s = store()
        s.grant()
        s.revoke()
        assertFalse(s.isGranted)
        assertNull(s.state.value.grantedAt)
        assertEquals(AiConsentStatus.DEFERRED, s.state.value.status)
        assertFalse(s.shouldPromptOnLaunch)
        assertFalse(store().isGranted)
    }

    @Test
    fun olderVersionNeedsReconsentAndAsksOncePerVersion() {
        // 古い版(2)に同意し、その版で説明を出した人
        prefs.putInt(AiConsentStore.VERSION_KEY, AiConsentStore.CURRENT_VERSION - 1)
        prefs.putBoolean(AiConsentStore.PROMPTED_KEY, true)
        prefs.putInt(AiConsentStore.PROMPTED_VERSION_KEY, AiConsentStore.CURRENT_VERSION - 1)
        val s = store()
        assertFalse(s.isGranted)
        assertTrue(s.state.value.needsReconsent)
        assertTrue(s.shouldPromptOnLaunch)
        val before = s.state.value
        s.markPrompted() // 新しい版の説明を出した(「あとで」)
        assertTrue(s.state.value.needsReconsent)
        assertFalse(s.shouldPromptOnLaunch)
        // state は変わらない(流れない)。shouldPromptOnLaunch を state をキーに覚えてはいけない(ShareLinkHost)
        assertEquals(before, s.state.value)
        s.grant()
        assertTrue(s.isGranted)
        assertFalse(s.state.value.needsReconsent)
    }

    @Test
    fun newerVersionCountsAsGranted() {
        prefs.putInt(AiConsentStore.VERSION_KEY, AiConsentStore.CURRENT_VERSION + 1)
        assertTrue(store().isGranted)
    }

    @Test
    fun reloadFollowsTheStore() {
        val s = store()
        s.grant()
        prefs.remove(AiConsentStore.VERSION_KEY) // 端末データの削除など
        assertTrue(s.isGranted)
        s.reload()
        assertFalse(s.isGranted)
    }

    @Test
    fun keysMatchIos() {
        assertEquals("metamon.aiConsent.version", AiConsentStore.VERSION_KEY)
        assertEquals("metamon.aiConsent.date", AiConsentStore.DATE_KEY)
        assertEquals(3, AiConsentStore.CURRENT_VERSION)
    }

    @Test
    fun forgetClearsConsentAndPromptMarkSoTheNextPersonIsAskedAgain() {
        val s = store()
        s.grant()
        s.forget()
        assertEquals(AiConsentStatus.NOT_PROMPTED, s.state.value.status)
        assertFalse(s.isGranted)
        assertTrue(s.shouldPromptOnLaunch)
        assertNull(prefs.getInt(AiConsentStore.VERSION_KEY))
        assertNull(prefs.getBoolean(AiConsentStore.PROMPTED_KEY))
        assertNull(prefs.getInt(AiConsentStore.PROMPTED_VERSION_KEY))
        assertFalse(store().isGranted) // 保存先からも消える
    }
}
