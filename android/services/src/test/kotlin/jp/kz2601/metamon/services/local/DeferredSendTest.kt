package jp.kz2601.metamon.services.local

import kotlinx.coroutines.test.runCurrent
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

class DeferredSendTest {
    @Test
    fun onlineDoesNotHold() = runTest {
        val d = DeferredSend<String>(ManualConnectivity(online = true), backgroundScope)
        assertFalse(d.holdIfOffline("a") { error("呼ばれない") })
        assertNull(d.pending)
    }

    @Test
    fun keepsOnlyTheLatestAndSendsOnceOnReconnect() = runTest {
        val net = ManualConnectivity(online = false)
        val d = DeferredSend<String>(net, backgroundScope)
        val sent = mutableListOf<String>()
        assertTrue(d.holdIfOffline("a") { sent += it })
        runCurrent()
        assertTrue(d.holdIfOffline("b") { sent += it })
        runCurrent()
        assertEquals("b", d.pending)
        net.set(true)
        runCurrent()
        net.set(false)
        net.set(true)
        runCurrent()
        assertEquals(listOf("b"), sent)
        assertNull(d.pending)
    }

    @Test
    fun cancelledHoldIsNotSent() = runTest {
        val net = ManualConnectivity(online = false)
        val d = DeferredSend<String>(net, backgroundScope)
        val sent = mutableListOf<String>()
        d.holdIfOffline("a") { sent += it }
        runCurrent()
        d.cancel()
        net.set(true)
        runCurrent()
        assertTrue(sent.isEmpty())
    }
}
