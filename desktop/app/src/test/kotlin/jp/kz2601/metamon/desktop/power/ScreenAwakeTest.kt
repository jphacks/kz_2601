package jp.kz2601.metamon.desktop.power

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

// 効かせる / 戻すの順(OS の呼び出しは偽物)
class ScreenAwakeTest {
    private class Fake : ScreenAwakeApi {
        val calls = mutableListOf<Boolean>()
        override fun setAwake(on: Boolean) { calls += on }
    }

    private fun awake(): Pair<ScreenAwake, Fake> = Fake().let { ScreenAwake(it) to it }

    @Test
    fun holdThenRelease() {
        val (a, f) = awake()
        a.setWindowShown("w1", true)
        val h = a.hold()
        assertEquals(listOf(true), f.calls)
        h.release()
        assertEquals(listOf(true, false), f.calls)
    }

    @Test
    fun releaseIsIdempotent() {
        val (a, f) = awake()
        a.setWindowShown("w1", true)
        val h1 = a.hold()
        val h2 = a.hold()
        h1.release(); h1.release(); h1.release()
        assertEquals(listOf(true), f.calls)
        h2.release()
        assertEquals(listOf(true, false), f.calls)
    }

    @Test
    fun minimizeReleasesAndRestoreHoldsAgain() {
        val (a, f) = awake()
        a.setWindowShown("w1", true)
        a.hold()
        a.setWindowShown("w1", false)
        a.setWindowShown("w1", true)
        assertEquals(listOf(true, false, true), f.calls)
    }

    @Test
    fun otherVisibleWindowKeepsItOn() {
        val (a, f) = awake()
        a.setWindowShown("w1", true)
        a.setWindowShown("w2", true)
        a.hold()
        a.setWindowShown("w1", false)
        assertEquals(listOf(true), f.calls)
        a.setWindowShown("w2", false)
        assertEquals(listOf(true, false), f.calls)
    }

    @Test
    fun noCallWhileNothingShown() {
        val (a, f) = awake()
        a.hold()
        assertTrue(f.calls.isEmpty())
        a.setWindowShown("w1", true)
        assertEquals(listOf(true), f.calls)
    }

    @Test
    fun releaseAllRestores() {
        val (a, f) = awake()
        a.setWindowShown("w1", true)
        a.hold(); a.hold()
        a.releaseAll()
        assertEquals(listOf(true, false), f.calls)
    }

    @Test
    fun osFailureDoesNotThrow() {
        val a = ScreenAwake(object : ScreenAwakeApi { override fun setAwake(on: Boolean) = error("boom") })
        a.setWindowShown("w1", true)
        a.hold().release()
    }

    @Test
    fun windowsFlags() {
        assertEquals(0x80000002.toInt(), WindowsScreenAwake.flags(true))
        assertEquals(0x80000000.toInt(), WindowsScreenAwake.flags(false))
    }

    @Test
    fun timerOnlyWhileRunning() {
        assertTrue(ScreenAwake.timerWantsAwake(true))
        assertFalse(ScreenAwake.timerWantsAwake(false))
    }
}
