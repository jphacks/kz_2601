package jp.kz2601.metamon.push

import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.DslJson
import jp.kz2601.metamon.services.push.InMemoryPushPrefs
import jp.kz2601.metamon.services.push.NotifyConsentStore
import jp.kz2601.metamon.services.push.NotifyConsentToggle
import jp.kz2601.metamon.services.push.PushDeviceClient
import jp.kz2601.metamon.services.push.PushDeviceRegistrar
import jp.kz2601.metamon.services.push.PushDeviceRegistration
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

class PushHostTest {
    private val calls = mutableListOf<String>()
    private val client = object : PushDeviceClient {
        override suspend fun putDevice(installationId: String, registration: PushDeviceRegistration) { calls += "put ${registration.token}" }
        override suspend fun deleteDevice(installationId: String) { calls += "delete" }
        override suspend fun setNotifyConsent(appId: String, allowed: Boolean) { calls += "consent $appId $allowed" }
    }
    private val prefs = InMemoryPushPrefs()
    private var uid: String? = "uid-1"
    private var canPost = true
    private var token: String? = "fcm-a"

    private fun host(registerDevices: Boolean = true): PushHost {
        val store = NotifyConsentStore(prefs)
        return PushHost(
            registrar = PushDeviceRegistrar(client, prefs, appVersion = "0.1.0", os = "Android 15", newId = { "install-1" }),
            consentStore = store,
            consent = NotifyConsentToggle(client, store),
            uid = { uid },
            canPost = { canPost },
            fetchToken = { token },
            registerDevices = registerDevices,
        )
    }

    @Test
    fun refreshRegistersOnlyWhenSignedInAndAllowed() = runTest {
        val h = host()
        uid = null
        h.refresh()
        assertTrue(calls.isEmpty())
        uid = "uid-1"
        h.refresh()
        h.refresh()
        assertEquals(listOf("put fcm-a"), calls)
    }

    @Test
    fun refreshAfterNotificationsWereTurnedOffDeletesTheRegistration() = runTest {
        val h = host()
        h.refresh()
        canPost = false
        h.refresh()
        assertEquals(listOf("put fcm-a", "delete"), calls)
    }

    @Test
    fun newTokenIsSentAndMissingTokenIsIgnored() = runTest {
        val h = host()
        token = null
        h.refresh()
        assertTrue(calls.isEmpty())
        h.receive("fcm-b")
        assertEquals(listOf("put fcm-b"), calls)
    }

    @Test
    fun nothingIsSentWhileTheBackendDoesNotAcceptAndroid() = runTest {
        val h = host(registerDevices = false)
        h.refresh()
        h.receive("fcm-b")
        assertTrue(calls.isEmpty())
    }

    @Test
    fun signOutDeletesTheRegistration() = runTest {
        val h = host()
        h.refresh()
        h.unregister()
        assertEquals(listOf("put fcm-a", "delete"), calls)
    }

    @Test
    fun openOnlyWhenSignedIn() = runTest {
        val opened = mutableListOf<String>()
        val h = host()
        uid = null
        h.open("app-1") { opened += it }
        uid = "uid-1"
        h.open("app-1") { opened += it }
        assertEquals(listOf("app-1"), opened)
    }

    @Test
    fun consentIsReadPerAccount() = runTest {
        val h = host()
        assertFalse(h.consentAllowed("app-1"))
        h.consent.change(true, h.currentUid(), "app-1") {}
        assertTrue(h.consentAllowed("app-1"))
        uid = "uid-2"
        assertFalse(h.consentAllowed("app-1"))
    }

    @Test
    fun consentIsOfferedOnlyForSharedOrPublicRecords() {
        fun dsl(scope: String?) = DslJson.decodeFromString(
            AppDSL.serializer(),
            """{"schemaVersion":1,"app":{"name":"x"},"initialScreen":"home","screens":[{"id":"home","title":"h","components":[]}],
               "entities":[{"id":"e","name":"e"${scope?.let { ",\"scope\":\"$it\"" } ?: ""},"fields":[]}]}""",
        )
        assertFalse(PushHost.canSetConsent(dsl(null)))
        assertFalse(PushHost.canSetConsent(dsl("private")))
        assertTrue(PushHost.canSetConsent(dsl("shared")))
        assertTrue(PushHost.canSetConsent(dsl("public")))
    }

    @Test
    fun pendingOpenIsConsumedOnce() {
        PushOpen.consume()
        PushOpen.offer(null)
        assertNull(PushOpen.pending.value)
        PushOpen.offer("app-1")
        assertEquals("app-1", PushOpen.consume())
        assertNull(PushOpen.consume())
    }
}
