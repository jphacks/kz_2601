package jp.kz2601.metamon.desktop.hotkey

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertSame
import org.junit.Assert.assertTrue
import org.junit.Test
import java.util.concurrent.CountDownLatch
import java.util.concurrent.LinkedBlockingQueue
import java.util.concurrent.TimeUnit
import java.util.concurrent.atomic.AtomicInteger

/** #2202: Linux の画面の判定・X11 の修飾キーの組み・つかむ / 外す(Xlib は作り物)。 */
class X11GlobalHotkey2202Test {
    private fun chord(raw: String) = (HotkeySpec.parse(raw) as HotkeyParse.Ok).chord

    // MARK: - 環境の判定

    @Test
    fun detectsDisplayKind() {
        assertEquals(LinuxDisplay.X11, LinuxDisplay.detect(mapOf("DISPLAY" to ":0")::get))
        assertEquals("XWayland の DISPLAY があっても Wayland", LinuxDisplay.WAYLAND, LinuxDisplay.detect(mapOf("DISPLAY" to ":0", "WAYLAND_DISPLAY" to "wayland-0")::get))
        assertEquals(LinuxDisplay.NONE, LinuxDisplay.detect(mapOf<String, String>()::get))
        assertEquals("空は無いのと同じ", LinuxDisplay.X11, LinuxDisplay.detect(mapOf("DISPLAY" to ":1", "WAYLAND_DISPLAY" to " ")::get))
    }

    @Test
    fun picksImplementationPerOsAndDisplay() {
        assertTrue(GlobalHotkeys.forOs("Linux", mapOf("DISPLAY" to ":0")::get) is X11GlobalHotkey)
        assertSame(UnsupportedGlobalHotkey, GlobalHotkeys.forOs("Linux", mapOf("WAYLAND_DISPLAY" to "wayland-0", "DISPLAY" to ":0")::get))
        assertSame(UnsupportedGlobalHotkey, GlobalHotkeys.forOs("Linux", mapOf<String, String>()::get))
        assertSame(UnsupportedGlobalHotkey, GlobalHotkeys.forOs("Mac OS X", mapOf("DISPLAY" to ":0")::get))
        assertNull("Linux 以外では libX11 を載せない", JnaXlib.load("Windows 11", mapOf("DISPLAY" to ":0")::get))
        assertNull("Wayland では載せない", JnaXlib.load("Linux", mapOf("WAYLAND_DISPLAY" to "w")::get))
    }

    // MARK: - 修飾キー

    @Test
    fun modifierMasks() {
        assertEquals(X11Keys.CONTROL_MASK or X11Keys.SHIFT_MASK, X11Keys.modifiers(chord("ctrl+shift+space")))
        assertEquals(X11Keys.MOD1_MASK or X11Keys.MOD4_MASK, X11Keys.modifiers(chord("alt+super+k")))
        assertEquals(0, X11Keys.modifiers(chord("f9")))
    }

    @Test
    fun grabsEveryLockCombination() {
        val sets = X11Keys.withLocks(X11Keys.CONTROL_MASK)
        assertEquals(listOf(4, 6, 20, 22), sets)
        assertEquals(4, sets.toSet().size)
    }

    @Test
    fun keysyms() {
        assertEquals(0x20L, X11Keys.keysym(chord("ctrl+space")))
        assertEquals(0x6BL, X11Keys.keysym(chord("ctrl+K")))
        assertEquals(0x37L, X11Keys.keysym(chord("ctrl+7")))
        assertEquals(0xFF0DL, X11Keys.keysym(chord("ctrl+enter")))
        assertEquals(0xFF1BL, X11Keys.keysym(chord("ctrl+esc")))
        assertEquals(0xFFBEL, X11Keys.keysym(chord("f1")))
        assertEquals(0xFFC9L, X11Keys.keysym(chord("ctrl+f12")))
        assertEquals(0xFFD5L, X11Keys.keysym(chord("ctrl+f24")))
        assertNull(X11Keys.keysym(HotkeyChord(ctrl = true, vk = 0xFFFF, keyName = "?")))
    }

    // MARK: - つかむ・外す

    private class FakeXlib(
        val opens: Boolean = true,
        val keycodeOf: Int = 65,
        val grabError: Int? = null,
    ) : Xlib {
        val events = LinkedBlockingQueue<Xlib.Event>()
        val grabbed = mutableListOf<Pair<Int, Int>>()
        val ungrabbed = mutableListOf<Pair<Int, Int>>()
        val closed = CountDownLatch(1)
        var opened = false

        override fun open(): Boolean = opens.also { opened = it }
        override fun keycode(keysym: Long) = keycodeOf
        override fun grab(keycode: Int, modifierSets: List<Int>): Int? {
            modifierSets.forEach { grabbed += keycode to it }
            return grabError
        }

        override fun ungrab(keycode: Int, modifierSets: List<Int>) {
            modifierSets.forEach { ungrabbed += keycode to it }
        }

        override fun next(waitMillis: Long): Xlib.Event = events.poll(waitMillis, TimeUnit.MILLISECONDS) ?: Xlib.Event.Idle
        override fun close() = closed.countDown()
    }

    @Test
    fun registersAndFiresOncePerHold() {
        val x = FakeXlib()
        val presses = AtomicInteger()
        val pressed = CountDownLatch(2)
        val h = X11GlobalHotkey({ x }, pollMillis = 5)
        val r = h.register(chord("ctrl+shift+space")) {
            presses.incrementAndGet()
            pressed.countDown()
        }
        assertTrue(r is HotkeyRegistration.Registered)
        assertEquals(4, x.grabbed.size)
        // 押しっぱなし(繰り返しの Press)は 1 回・離して押し直すと 2 回目・違うキーは数えない
        x.events += listOf(Xlib.Event.Press(65), Xlib.Event.Press(65), Xlib.Event.Press(65), Xlib.Event.Press(10), Xlib.Event.Release(65), Xlib.Event.Press(65))
        assertTrue(pressed.await(2, TimeUnit.SECONDS))
        h.unregister()
        assertEquals(2, presses.get())
        assertEquals("外す時は全部 XUngrabKey", x.grabbed, x.ungrabbed)
        assertTrue(x.closed.await(1, TimeUnit.SECONDS))
    }

    @Test
    fun badAccessIsInUseAndStillUngrabs() {
        val x = FakeXlib(grabError = X11GlobalHotkey.BAD_ACCESS)
        val r = X11GlobalHotkey({ x }, pollMillis = 5).register(chord("ctrl+shift+space")) {}
        assertEquals(HotkeyRegistration.Failed(HotkeyFailure.IN_USE), r)
        assertEquals("一部つかめた組みを残さない", x.grabbed, x.ungrabbed)
        assertTrue(x.closed.await(1, TimeUnit.SECONDS))
        assertEquals(HotkeyFailure.ERROR, (X11GlobalHotkey.failure(2) as HotkeyRegistration.Failed).reason)
    }

    @Test
    fun noDisplayOrNoKeycode() {
        assertEquals(HotkeyFailure.UNSUPPORTED, (X11GlobalHotkey({ null }).register(chord("ctrl+k")) {} as HotkeyRegistration.Failed).reason)
        val closedDisplay = FakeXlib(opens = false)
        assertEquals(HotkeyFailure.UNSUPPORTED, (X11GlobalHotkey({ closedDisplay }).register(chord("ctrl+k")) {} as HotkeyRegistration.Failed).reason)
        val noKey = FakeXlib(keycodeOf = 0)
        assertEquals(HotkeyFailure.ERROR, (X11GlobalHotkey({ noKey }).register(chord("ctrl+k")) {} as HotkeyRegistration.Failed).reason)
        assertTrue(noKey.grabbed.isEmpty())
        assertTrue("開いた Display は閉じる", noKey.closed.await(1, TimeUnit.SECONDS))
    }

    @Test
    fun reRegisterReleasesPrevious() {
        val first = FakeXlib()
        val second = FakeXlib(keycodeOf = 66)
        val queue = ArrayDeque(listOf(first, second))
        val h = X11GlobalHotkey({ queue.removeFirst() }, pollMillis = 5)
        assertTrue(h.register(chord("ctrl+a")) {} is HotkeyRegistration.Registered)
        assertTrue(h.register(chord("ctrl+b")) {} is HotkeyRegistration.Registered)
        assertEquals(first.grabbed, first.ungrabbed)
        assertTrue(second.ungrabbed.isEmpty())
        h.close()
        assertEquals(second.grabbed, second.ungrabbed)
        h.unregister() // 2 回目は何もしない
    }

    @Test
    fun repeatFilter() {
        val f = RepeatFilter()
        assertTrue(f.press())
        assertFalse(f.press())
        f.release()
        assertTrue(f.press())
    }
}
