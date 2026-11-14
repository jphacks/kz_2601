package jp.kz2601.metamon.services.push

import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.async
import kotlinx.coroutines.awaitCancellation
import kotlinx.coroutines.test.runTest
import kotlinx.coroutines.yield
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

class FakePushDeviceClient : PushDeviceClient {
    val calls = mutableListOf<String>()
    val registrations = mutableListOf<PushDeviceRegistration>()
    var failPut = false
    var failDelete = false
    var failConsent = false
    var putGate: CompletableDeferred<Unit>? = null
    var hangDelete = false

    override suspend fun putDevice(installationId: String, registration: PushDeviceRegistration) {
        calls += "put $installationId"
        registrations += registration
        putGate?.await()
        if (failPut) throw java.io.IOException("offline")
    }

    override suspend fun deleteDevice(installationId: String) {
        calls += "delete $installationId"
        if (hangDelete) awaitCancellation()
        if (failDelete) throw java.io.IOException("offline")
    }

    override suspend fun setNotifyConsent(appId: String, allowed: Boolean) {
        calls += "consent $appId $allowed"
        if (failConsent) throw java.io.IOException("offline")
    }
}

class PushDeviceRegistrarTest {
    private val client = FakePushDeviceClient()
    private val prefs = InMemoryPushPrefs()
    private var ids = 0
    private fun registrar() = PushDeviceRegistrar(client, prefs, appVersion = "0.1.0", os = "Android 15", newId = { "install-${++ids}" })

    @Test
    fun registersOnceAndSkipsTheSameToken() = runTest {
        val r = registrar()
        assertTrue(r.register("fcm-a", "uid-1"))
        assertTrue(r.register("fcm-a", "uid-1"))
        assertEquals(listOf("put install-1"), client.calls)
        val sent = client.registrations.single()
        assertEquals("fcm-a", sent.token)
        assertEquals("android", sent.platform)
        assertEquals("production", sent.environment)
        assertEquals("Android 15", sent.os)
        // トークンそのものは端末に残さない
        assertFalse(prefs.values.values.any { it.contains("fcm-a") })
    }

    @Test
    fun sendsAgainWhenTheTokenOrAccountChangesWithTheSameInstallationId() = runTest {
        val r = registrar()
        r.register("fcm-a", "uid-1")
        r.register("fcm-b", "uid-1")
        r.register("fcm-b", "uid-2")
        assertEquals(listOf("put install-1", "put install-1", "put install-1"), client.calls)
        assertEquals(listOf("fcm-a", "fcm-b", "fcm-b"), client.registrations.map { it.token })
    }

    @Test
    fun failureIsNotRememberedSoTheNextForegroundRetries() = runTest {
        val r = registrar()
        client.failPut = true
        assertFalse(r.register("fcm-a", "uid-1"))
        assertFalse(r.isRegistered)
        client.failPut = false
        assertTrue(r.register("fcm-a", "uid-1"))
        assertEquals(listOf("put install-1", "put install-1"), client.calls)
        assertTrue(r.isRegistered)
    }

    @Test
    fun blankTokenIsIgnored() = runTest {
        assertFalse(registrar().register(" ", "uid-1"))
        assertTrue(client.calls.isEmpty())
    }

    @Test
    fun unregisterDeletesKeepsTheInstallationIdAndRegistersAgainNextTime() = runTest {
        val r = registrar()
        r.register("fcm-a", "uid-1")
        r.unregister()
        assertEquals(listOf("put install-1", "delete install-1"), client.calls)
        assertNull(prefs.getString(PushDeviceRegistrar.LAST_SENT_KEY))
        assertEquals("install-1", prefs.getString(PushDeviceRegistrar.INSTALLATION_ID_KEY))
        r.register("fcm-a", "uid-2")
        assertEquals("put install-1", client.calls.last())
    }

    @Test
    fun unregisterWithoutEverRegisteringSendsNothing() = runTest {
        registrar().unregister()
        assertTrue(client.calls.isEmpty())
    }

    @Test
    fun unregisterFailureDoesNotThrow() = runTest {
        val r = registrar()
        r.register("fcm-a", "uid-1")
        client.failDelete = true
        r.unregister()
        assertEquals("delete install-1", client.calls.last())
        assertFalse(r.isRegistered)
    }

    @Test
    fun unregisterGivesUpAfterTheTimeout() = runTest {
        val r = registrar()
        r.register("fcm-a", "uid-1")
        client.hangDelete = true
        r.unregister(timeoutMillis = 3_000)
        assertEquals("delete install-1", client.calls.last())
    }

    @Test
    fun unregisterWaitsForTheInFlightRegistrationAndDoesNotRememberIt() = runTest {
        val r = registrar()
        val gate = CompletableDeferred<Unit>()
        client.putGate = gate
        val put = async { r.register("fcm-a", "uid-1") }
        yield()
        val del = async { r.unregister() }
        yield()
        assertEquals(listOf("put install-1"), client.calls)
        gate.complete(Unit)
        put.await()
        del.await()
        assertEquals(listOf("put install-1", "delete install-1"), client.calls)
        assertFalse(r.isRegistered)
    }

    @Test
    fun authorizationRevokedDeletesOnlyWhenRegistered() = runTest {
        val r = registrar()
        r.authorizationRevoked()
        assertTrue(client.calls.isEmpty())
        r.register("fcm-a", "uid-1")
        r.authorizationRevoked()
        assertEquals(listOf("put install-1", "delete install-1"), client.calls)
    }
}
