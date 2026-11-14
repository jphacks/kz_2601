package jp.kz2601.metamon.desktop.hotkey

import jp.kz2601.metamon.desktop.i18n.Strings
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/** #2204: 押したキーの読み取り・保存・失敗の表示。 */
class HotkeySettings2204Test {
    private class MemorySetting(var value: String? = null) : MutableStringSetting {
        override fun get() = value
        override fun set(value: String) {
            this.value = value
        }
    }

    private fun chord(raw: String) = (HotkeySpec.parse(raw) as HotkeyParse.Ok).chord

    // MARK: - 押したキーの読み取り(AWT のキーの番号)

    @Test
    fun captureSpec() {
        assertEquals("ctrl+shift+space", HotkeyCapture.spec(0x20, ctrl = true, alt = false, shift = true, meta = false))
        assertEquals("alt+win+k", HotkeyCapture.spec(0x4B, ctrl = false, alt = true, shift = false, meta = true))
        assertEquals("AWT の Enter は 0x0A", "ctrl+enter", HotkeyCapture.spec(0x0A, ctrl = true, alt = false, shift = false, meta = false))
        assertEquals("ctrl+7", HotkeyCapture.spec(0x37, ctrl = true, alt = false, shift = false, meta = false))
        assertEquals("f12", HotkeyCapture.spec(0x7B, ctrl = false, alt = false, shift = false, meta = false))
        assertEquals("F13 は 0xF000", "f13", HotkeyCapture.spec(0xF000, ctrl = false, alt = false, shift = false, meta = false))
        assertEquals("ctrl+insert", HotkeyCapture.spec(0x9B, ctrl = true, alt = false, shift = false, meta = false))
        assertNull("Ctrl だけはまだ途中", HotkeyCapture.spec(0x11, ctrl = true, alt = false, shift = false, meta = false))
        assertNull(HotkeyCapture.spec(0x10, ctrl = false, alt = false, shift = true, meta = false))
        // 読み取った形はそのまま HotkeySpec で読める
        assertEquals(chord("ctrl+shift+space"), chord(HotkeyCapture.spec(0x20, true, false, true, false)!!))
        val unknown = HotkeySpec.parse(HotkeyCapture.spec(0x2C, ctrl = true, alt = false, shift = false, meta = false)) as HotkeyParse.Error
        assertEquals(HotkeyParseError.UNKNOWN_KEY, unknown.reason)
    }

    // MARK: - 保存

    @Test
    fun offByDefaultShowsDefaultKey() {
        val m = HotkeySettingsModel(MemorySetting(), MemorySetting())
        assertEquals(HotkeySettingsState(enabled = false, chord = chord(HotkeySettingsModel.DEFAULT)), m.state())
    }

    @Test
    fun changeSavesAndTurnsOn() {
        val setting = MemorySetting()
        val last = MemorySetting()
        val m = HotkeySettingsModel(setting, last)
        assertTrue(m.change("alt+f2") is HotkeyParse.Ok)
        assertEquals("alt+f2", setting.value)
        assertEquals(HotkeySettingsState(true, chord("alt+f2")), m.state())
        // 読める値は #2201 の起動時の読み方でもそのまま読める
        assertEquals("alt+f2", HotkeySettings.raw(props = { null }, env = { null }, setting = setting))
    }

    @Test
    fun invalidCaptureIsNotSaved() {
        val setting = MemorySetting("ctrl+k")
        val m = HotkeySettingsModel(setting, MemorySetting("ctrl+k"))
        val r = m.change("space") as HotkeyParse.Error
        assertEquals("ctrl+k", setting.value)
        assertEquals(Strings["desktop_hotkey_need_modifier"], HotkeySettingsText.captureError(r))
        assertEquals(Strings["desktop_hotkey_unknown_key"], HotkeySettingsText.captureError(m.change("ctrl+?") as HotkeyParse.Error))
    }

    @Test
    fun offKeepsLastKeyAndOnRestoresIt() {
        val setting = MemorySetting()
        val last = MemorySetting()
        val m = HotkeySettingsModel(setting, last)
        m.change("ctrl+alt+v")
        m.setEnabled(false)
        assertEquals("off", setting.value)
        assertNull("off は #2201 の読み方でも off", HotkeySettings.raw(props = { null }, env = { null }, setting = setting))
        assertEquals(HotkeySettingsState(false, chord("ctrl+alt+v")), m.state())
        m.setEnabled(true)
        assertEquals("ctrl+alt+v", setting.value)
        m.reset()
        assertEquals(HotkeySettingsModel.DEFAULT, setting.value)
        assertEquals(HotkeySettingsModel.DEFAULT, last.value)
        assertTrue(m.state().enabled)
    }

    @Test
    fun onWithoutHistoryUsesDefault() {
        val setting = MemorySetting()
        HotkeySettingsModel(setting, MemorySetting("broken+")).setEnabled(true)
        assertEquals(HotkeySettingsModel.DEFAULT, setting.value)
    }

    // MARK: - 失敗の表示

    @Test
    fun failureText() {
        val on = HotkeySettingsState(true, chord("ctrl+shift+space"))
        val linuxX11 = mapOf("DISPLAY" to ":0")::get
        assertEquals(
            Strings.format("desktop_hotkey_in_use", "Ctrl+Shift+Space"),
            HotkeySettingsText.failure(on, HotkeyRegistration.Failed(HotkeyFailure.IN_USE), "Windows 11", linuxX11),
        )
        assertEquals(
            Strings.format("desktop_hotkey_failed", "Ctrl+Shift+Space"),
            HotkeySettingsText.failure(on, HotkeyRegistration.Failed(HotkeyFailure.ERROR, "x"), "Windows 11", linuxX11),
        )
        assertEquals(
            "この環境では使えない",
            Strings["desktop_hotkey_unavailable_wayland"],
            HotkeySettingsText.failure(on, HotkeyRegistration.Failed(HotkeyFailure.UNSUPPORTED), "Linux", mapOf("WAYLAND_DISPLAY" to "w")::get),
        )
        assertNull(HotkeySettingsText.failure(on, HotkeyRegistration.Registered(on.chord!!), "Linux", linuxX11))
        assertNull(HotkeySettingsText.failure(on, null, "Linux", linuxX11))
        assertNull("off の時は出さない", HotkeySettingsText.failure(on.copy(enabled = false), HotkeyRegistration.Failed(HotkeyFailure.IN_USE), "Windows 11", linuxX11))
    }

    // MARK: - 登録し直し

    @Test
    fun runnerClosesPreviousOnRestart() {
        val closed = mutableListOf<String?>()
        class Fake(val raw: String?) : GlobalHotkey {
            override fun register(chord: HotkeyChord, onPressed: () -> Unit) = HotkeyRegistration.Registered(chord)
            override fun unregister() {
                closed += raw
            }
        }
        val runner = HotkeyRunner { raw -> raw?.let { Fake(it) } }
        runner.restart("ctrl+a")
        runner.restart(null)
        assertEquals(listOf<String?>("ctrl+a"), closed)
        runner.restart("ctrl+b")
        runner.close()
        assertEquals(listOf<String?>("ctrl+a", "ctrl+b"), closed)
        assertFalse(closed.contains(null))
    }
}
