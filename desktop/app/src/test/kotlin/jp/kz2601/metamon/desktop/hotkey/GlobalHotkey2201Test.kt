package jp.kz2601.metamon.desktop.hotkey

import jp.kz2601.metamon.desktop.i18n.Strings
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import java.util.concurrent.CountDownLatch
import java.util.concurrent.LinkedBlockingQueue
import java.util.concurrent.TimeUnit

/** #2201: キーの文字列の読み取り・登録の失敗の扱い・押した時の openVoice への渡し方(Windows の呼び出しは作り物)。 */
class GlobalHotkey2201Test {
    private fun ok(raw: String) = (HotkeySpec.parse(raw) as HotkeyParse.Ok).chord
    private fun err(raw: String?) = (HotkeySpec.parse(raw) as HotkeyParse.Error).reason

    // MARK: - キーの文字列

    @Test
    fun parsesModifiersAndKey() {
        val c = ok("ctrl+shift+space")
        assertTrue(c.ctrl && c.shift && !c.alt && !c.meta)
        assertEquals(0x20, c.vk)
        assertEquals("Ctrl+Shift+Space", c.label)
        assertEquals(0x4B, ok(" Alt + Win + k ").vk)
        assertEquals("Alt+Win+K", ok("alt+win+k").label)
        assertEquals(0x37, ok("control+7").vk)
        assertEquals(0x7B, ok("ctrl+F12").vk)
        assertEquals("F9 だけでもよい", 0x78, ok("f9").vk)
        assertTrue(ok("CMD+Option+Enter").let { it.meta && it.alt && it.vk == 0x0D })
    }

    @Test
    fun rejectsBrokenStrings() {
        assertEquals(HotkeyParseError.EMPTY, err(null))
        assertEquals(HotkeyParseError.EMPTY, err(" + "))
        assertEquals(HotkeyParseError.NO_KEY, err("ctrl+shift"))
        assertEquals(HotkeyParseError.MULTIPLE_KEYS, err("ctrl+a+b"))
        assertEquals(HotkeyParseError.UNKNOWN_KEY, err("ctrl+banana"))
        assertEquals(HotkeyParseError.UNKNOWN_KEY, err("ctrl+f25"))
        assertEquals("普段の入力を奪わない", HotkeyParseError.NO_MODIFIER, err("space"))
        assertEquals(HotkeyParseError.NO_MODIFIER, err("a"))
    }

    @Test
    fun offByDefaultAndEnvBeatsSetting() {
        val none = object : StringSetting { override fun get(): String? = null }
        val saved = object : StringSetting { override fun get() = "alt+f2" }
        assertNull(HotkeySettings.raw(props = { null }, env = { null }, setting = none))
        assertEquals("alt+f2", HotkeySettings.raw(props = { null }, env = { null }, setting = saved))
        assertEquals("ctrl+shift+space", HotkeySettings.raw(props = { null }, env = mapOf("TAILORIT_HOTKEY" to "ctrl+shift+space")::get, setting = saved))
        assertNull("off で設定も止める", HotkeySettings.raw(props = { null }, env = mapOf("TAILORIT_HOTKEY" to "off")::get, setting = saved))
        assertEquals("ctrl+k", HotkeySettings.raw(props = mapOf("tailorit.hotkey" to "ctrl+k")::get, env = mapOf("TAILORIT_HOTKEY" to "off")::get, setting = none))
    }

    @Test
    fun windowsModifierBits() {
        // MOD_CONTROL | MOD_SHIFT | MOD_NOREPEAT
        assertEquals(0x4006, WindowsGlobalHotkey.modifiers(ok("ctrl+shift+space")))
        assertEquals(0x4009, WindowsGlobalHotkey.modifiers(ok("alt+win+k")))
    }

    // MARK: - Windows の登録(作り物の Win32)

    private class FakeWin32(private val registerError: Int? = null) : Win32Hotkeys {
        val messages = LinkedBlockingQueue<Win32Hotkeys.Message>()
        val registered = mutableListOf<Triple<Int, Int, Int>>()
        val unregistered = CountDownLatch(1)
        var quitTo: Int? = null

        override fun currentThreadId() = 42
        override fun ensureQueue() = Unit
        override fun register(id: Int, modifiers: Int, vk: Int): Int? {
            registered += Triple(id, modifiers, vk)
            return registerError
        }
        override fun unregister(id: Int) = unregistered.countDown()
        override fun nextMessage(): Win32Hotkeys.Message = messages.take()
        override fun postQuit(threadId: Int) {
            quitTo = threadId
            messages.put(Win32Hotkeys.Message.Quit)
        }
    }

    @Test
    fun registersRunsCallbackOnHotkeyAndUnregistersOnQuit() {
        val win = FakeWin32()
        val h = WindowsGlobalHotkey({ win })
        val pressed = LinkedBlockingQueue<Int>()
        val r = h.register(ok("ctrl+shift+space")) { pressed.put(1) }
        assertEquals(HotkeyRegistration.Registered(ok("ctrl+shift+space")), r)
        assertEquals(Triple(WindowsGlobalHotkey.HOTKEY_ID, 0x4006, 0x20), win.registered.single())
        win.messages.put(Win32Hotkeys.Message.Other)
        win.messages.put(Win32Hotkeys.Message.Hotkey(999)) // 他の id は無視
        win.messages.put(Win32Hotkeys.Message.Hotkey(WindowsGlobalHotkey.HOTKEY_ID))
        assertEquals(1, pressed.poll(2, TimeUnit.SECONDS))
        assertNull(pressed.poll(100, TimeUnit.MILLISECONDS))
        h.unregister()
        assertEquals(42, win.quitTo)
        assertTrue("登録したスレッドで外す", win.unregistered.await(2, TimeUnit.SECONDS))
    }

    @Test
    fun alreadyRegisteredByAnotherAppIsInUse() {
        val win = FakeWin32(registerError = WindowsGlobalHotkey.ERROR_HOTKEY_ALREADY_REGISTERED)
        val r = WindowsGlobalHotkey({ win }).register(ok("ctrl+shift+space")) {}
        assertEquals(HotkeyRegistration.Failed(HotkeyFailure.IN_USE), r)
        assertEquals(1L, win.unregistered.count) // 登録していないので外さない
    }

    @Test
    fun otherErrorsCarryTheCodeAndNoJnaMeansUnsupported() {
        val r = WindowsGlobalHotkey({ FakeWin32(registerError = 5) }).register(ok("ctrl+k")) {}
        assertEquals(HotkeyRegistration.Failed(HotkeyFailure.ERROR, "GetLastError=5"), r)
        assertEquals(HotkeyRegistration.Failed(HotkeyFailure.UNSUPPORTED), WindowsGlobalHotkey({ null }).register(ok("ctrl+k")) {})
        assertEquals(HotkeyFailure.UNSUPPORTED, (UnsupportedGlobalHotkey.register(ok("ctrl+k")) {} as HotkeyRegistration.Failed).reason)
    }

    // MARK: - 押した時・知らせ

    @Test
    fun pressStartsVoiceOnlyWhenEnabledAndOnlyInTheFrontWindow() {
        val req = HotkeyVoiceRequests()
        val notices = mutableListOf<String>()
        var fronted = 0
        assertFalse(req.press(voiceEnabled = false, notify = { _, b -> notices += b }, bringToFront = { fronted++ }))
        assertEquals(listOf(Strings["desktop_unavailable_voice"]), notices)
        assertEquals(0, req.requests)
        assertTrue(req.press(voiceEnabled = true, notify = { _, b -> notices += b }, bringToFront = { fronted++ }))
        assertEquals(1, fronted)
        assertEquals(1, req.requests)
        assertFalse("後ろのウィンドウは受けない", req.take { false })
        assertTrue(req.take { true })
        assertFalse("1 回の押下で 1 回だけ", req.take { true })
    }

    @Test
    fun startFromSettingsNotifiesFailuresAndStaysQuietWhenOff() {
        val notices = mutableListOf<String>()
        fun start(raw: String?, hotkey: GlobalHotkey) = GlobalHotkeys.startFromSettings(
            raw = raw, hotkey = { hotkey }, voiceEnabled = { true }, bringToFront = {}, notify = { _, b -> notices += b }, post = { it.run() }, log = {},
        )
        assertNull(start(null, UnsupportedGlobalHotkey))
        assertNull("この OS では黙る", start("ctrl+shift+space", UnsupportedGlobalHotkey))
        assertTrue(notices.isEmpty())
        assertNull(start("ctrl+banana", UnsupportedGlobalHotkey))
        assertEquals(Strings.format("desktop_hotkey_invalid", "ctrl+banana"), notices.removeAt(0))
        val inUse = WindowsGlobalHotkey({ FakeWin32(registerError = WindowsGlobalHotkey.ERROR_HOTKEY_ALREADY_REGISTERED) })
        assertNull(start("ctrl+shift+space", inUse))
        assertEquals(Strings.format("desktop_hotkey_in_use", "Ctrl+Shift+Space"), notices.removeAt(0))

        val win = FakeWin32()
        val h = start("ctrl+shift+space", WindowsGlobalHotkey({ win }))!!
        assertTrue(notices.isEmpty())
        h.close()
        assertTrue(win.unregistered.await(2, TimeUnit.SECONDS))
    }
}
