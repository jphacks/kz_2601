package jp.kz2601.metamon.desktop.power

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class LinuxScreenAwakeTest {
    private class FakeBus(var cookie: Int? = 7, var failInhibit: Boolean = false, var failUn: Boolean = false) : ScreenSaverBus {
        val calls = mutableListOf<String>()
        override fun inhibit(appName: String, reason: String): Int? {
            calls += "inhibit:$appName:$reason"
            if (failInhibit) error("no bus")
            return cookie
        }

        override fun unInhibit(cookie: Int) {
            calls += "un:$cookie"
            if (failUn) error("gone")
        }
    }

    @Test
    fun holdsOnceAndReleasesWhenLastHolderLeaves() {
        val bus = FakeBus()
        val a = LinuxScreenAwake(bus)
        assertTrue(a.hold("timer"))
        assertTrue(a.hold("video"))
        assertEquals(listOf("inhibit:Tailor It:timer"), bus.calls)
        a.release()
        assertTrue(a.isHeld)
        a.release()
        assertFalse(a.isHeld)
        assertEquals(listOf("inhibit:Tailor It:timer", "un:7"), bus.calls)
    }

    @Test
    fun noBusDoesNothingAndDoesNotCrash() {
        val a = LinuxScreenAwake()
        assertFalse(a.hold("timer"))
        a.release()
        a.releaseAll()
        assertFalse(a.isHeld)
    }

    @Test
    fun failingBusDoesNotCrash() {
        val a = LinuxScreenAwake(FakeBus(failInhibit = true))
        assertFalse(a.hold("timer"))
        a.release()
        val b = LinuxScreenAwake(FakeBus(failUn = true))
        assertTrue(b.hold("x"))
        b.release()
        assertFalse(b.isHeld)
    }

    @Test
    fun extraReleaseDoesNotGoNegative() {
        val bus = FakeBus()
        val a = LinuxScreenAwake(bus)
        a.release()
        assertTrue(a.hold("x"))
        a.release()
        a.release()
        assertTrue(a.hold("y"))
        assertEquals(listOf("inhibit:Tailor It:x", "un:7", "inhibit:Tailor It:y"), bus.calls)
    }

    @Test
    fun retriesInhibitOnNextHoldAfterFailure() {
        val bus = FakeBus(cookie = null)
        val a = LinuxScreenAwake(bus)
        assertFalse(a.hold("a"))
        bus.cookie = 3
        assertTrue(a.hold("b"))
    }
}
