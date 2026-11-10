package jp.kz2601.metamon.services.push

import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

class NotifyConsentTest {
    private val client = FakePushDeviceClient()
    private val store = NotifyConsentStore(InMemoryPushPrefs())
    private val toggle = NotifyConsentToggle(client, store)

    @Test
    fun storeIsPerAccountAndAppAndDefaultsToOff() {
        assertFalse(store.allowed("uid-1", "app-1"))
        store.set(true, "uid-1", "app-1")
        assertTrue(store.allowed("uid-1", "app-1"))
        assertFalse(store.allowed("uid-1", "app-2"))
        assertFalse(store.allowed("uid-2", "app-1"))
        store.set(false, "uid-1", "app-1")
        assertFalse(store.allowed("uid-1", "app-1"))
    }

    @Test
    fun changeShowsTheNewValueFirstAndSavesOnSuccess() = runTest {
        val seen = mutableListOf<NotifyConsentToggle.State>()
        val end = toggle.change(true, "uid-1", "app-1") { seen += it }
        assertEquals(listOf(NotifyConsentToggle.State(isOn = true)), seen)
        assertEquals(NotifyConsentToggle.State(isOn = true), end)
        assertEquals(listOf("consent app-1 true"), client.calls)
        assertTrue(toggle.initial("uid-1", "app-1").isOn)
    }

    @Test
    fun failureRollsBackAndKeepsTheOldValue() = runTest {
        client.failConsent = true
        val seen = mutableListOf<NotifyConsentToggle.State>()
        val end = toggle.change(true, "uid-1", "app-1") { seen += it }
        assertTrue(seen.single().isOn)
        assertEquals(NotifyConsentToggle.State(isOn = false, failed = true), end)
        assertFalse(store.allowed("uid-1", "app-1"))
    }

    @Test
    fun signedOutDoesNotSend() = runTest {
        val end = toggle.change(true, null, "app-1") {}
        assertTrue(end.failed)
        assertFalse(end.isOn)
        assertTrue(client.calls.isEmpty())
        assertFalse(toggle.initial(null, "app-1").isOn)
    }

    @Test
    fun messageReadsAppIdTitleAndBody() {
        val m = PushMessage.from(mapOf("appId" to "kakeibo-1", "title" to "記録が増えました", "body" to "さくらさんが 1 件足しました", "appName" to "家計簿"))
        assertEquals(PushMessage("kakeibo-1", "記録が増えました", "さくらさんが 1 件足しました"), m)
        assertFalse(m.isEmpty)
    }

    @Test
    fun messagePrefersTheNotificationPartAndFallsBackToTheAppName() {
        val m = PushMessage.from(mapOf("appId" to "a1", "title" to "data", "appName" to "家計簿"), notificationTitle = null, notificationBody = "本文")
        assertEquals("data", m.title)
        assertEquals("本文", m.body)
        assertEquals("家計簿", PushMessage.from(mapOf("appName" to "家計簿", "body" to "x")).title)
        assertEquals("通知", PushMessage.from(mapOf("title" to "data"), notificationTitle = "通知").title)
    }

    @Test
    fun messageRejectsMalformedAppIdsAndEmptyMessages() {
        assertNull(PushMessage.from(mapOf("appId" to "../etc", "body" to "x")).appId)
        assertNull(PushMessage.from(mapOf("appId" to "a".repeat(129), "body" to "x")).appId)
        assertNull(PushMessage.appId(null))
        assertEquals("app_1.v:2-x", PushMessage.appId(" app_1.v:2-x "))
        assertTrue(PushMessage.from(mapOf("appId" to "a1")).isEmpty)
    }
}
