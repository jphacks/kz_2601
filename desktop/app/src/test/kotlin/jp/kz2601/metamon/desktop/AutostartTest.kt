package jp.kz2601.metamon.desktop

import jp.kz2601.metamon.desktop.i18n.Lang
import jp.kz2601.metamon.desktop.i18n.Strings
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.rules.TemporaryFolder
import java.io.File
import java.nio.charset.StandardCharsets

/** ログイン時に開く: OS ごとの登録先・既定 off・トレイの切り替え。本物のレジストリ・~/.config には触らない。 */
class AutostartTest {
    @get:Rule
    val tmp = TemporaryFolder()

    @Test
    fun onlyWhenPackagedAndNotOnMac() {
        val home = tmp.root.absolutePath
        // gradlew run(jpackage でない)では出さない
        assertNull(Autostart.forCurrentOs("Windows 11", appPath = null, env = emptyMap(), home = home))
        assertNull(Autostart.forCurrentOs("Linux", appPath = "", env = emptyMap(), home = home))
        // macOS は Mac 版の SMAppService が受け持つ
        assertNull(Autostart.forCurrentOs("Mac OS X", appPath = "/Applications/Tailor It.app", env = emptyMap(), home = home))
        assertTrue(Autostart.forCurrentOs("Windows 10", appPath = "C:\\a.exe", env = emptyMap(), home = home) is WindowsRunKey)
        assertTrue(Autostart.forCurrentOs("Linux", appPath = "/opt/a", env = emptyMap(), home = home) is XdgAutostart)
    }

    @Test
    fun xdgDirFollowsConfigHome() {
        assertEquals(File("/x/cfg/autostart"), XdgAutostart.dir(mapOf("XDG_CONFIG_HOME" to "/x/cfg"), "/home/u"))
        assertEquals(File("/home/u/.config/autostart"), XdgAutostart.dir(emptyMap(), "/home/u"))
        // 相対の XDG_CONFIG_HOME は決まりどおり無視する
        assertEquals(File("/home/u/.config/autostart"), XdgAutostart.dir(mapOf("XDG_CONFIG_HOME" to "rel"), "/home/u"))
    }

    @Test
    fun xdgWritesAndRemovesDesktopEntry() {
        val dir = File(tmp.root, "autostart")
        val item = XdgAutostart("/opt/tailor-it/bin/Tailor It", dir)
        assertFalse(item.isEnabled())
        item.setEnabled(false) // 無い物を消しても落ちない
        item.setEnabled(true)
        assertTrue(item.isEnabled())
        assertEquals(
            "[Desktop Entry]\nType=Application\nName=Tailor It\nExec=\"/opt/tailor-it/bin/Tailor It\"\nX-GNOME-Autostart-enabled=true\n",
            item.file.readText(),
        )
        item.setEnabled(false)
        assertFalse(item.isEnabled())
        assertFalse(item.file.exists())
    }

    @Test
    fun execQuoteEscapesReservedCharacters() {
        assertEquals("\"/opt/a b\"", XdgAutostart.execQuote("/opt/a b"))
        assertEquals("\"/x/\\\$y\\\"z\\`w\\\\v 100%%\"", XdgAutostart.execQuote("/x/\$y\"z`w\\v 100%"))
    }

    @Test
    fun windowsRegFileKeepsQuotesAroundPath() {
        val reg = WindowsRunKey.regFile("C:\\Program Files\\Tailor It\\Tailor It.exe")
        assertTrue(reg.startsWith("Windows Registry Editor Version 5.00\r\n\r\n[HKEY_CURRENT_USER\\Software\\Microsoft\\Windows\\CurrentVersion\\Run]\r\n"))
        // 値は "C:\Program Files\...\Tailor It.exe"(引用符付き)
        assertTrue(reg.endsWith("\"TailorIt\"=\"\\\"C:\\\\Program Files\\\\Tailor It\\\\Tailor It.exe\\\"\"\r\n"))
    }

    @Test
    fun windowsRunsRegAndImportsUtf16File() {
        val calls = mutableListOf<List<String>>()
        var imported: ByteArray? = null
        var present = false
        val runner = CommandRunner { cmd ->
            calls += cmd
            when (cmd[1]) {
                "query" -> if (present) 0 else 1
                "import" -> {
                    imported = File(cmd[2]).readBytes()
                    present = true
                    0
                }
                "delete" -> {
                    present = false
                    0
                }
                else -> 1
            }
        }
        val item = WindowsRunKey("C:\\家\\Tailor It.exe", runner, tmp.root)
        assertFalse(item.isEnabled())
        item.setEnabled(false) // 無ければ delete を呼ばない
        assertEquals(listOf("query", "query"), calls.map { it[1] })
        item.setEnabled(true)
        assertTrue(item.isEnabled())
        val bytes = imported!!
        assertEquals(listOf(0xFF.toByte(), 0xFE.toByte()), bytes.take(2))
        assertTrue(String(bytes, 2, bytes.size - 2, StandardCharsets.UTF_16LE).contains("C:\\\\家\\\\Tailor It.exe"))
        // 一時の .reg は残さない
        assertEquals(emptyList<String>(), tmp.root.list()!!.filter { it.endsWith(".reg") })
        item.setEnabled(false)
        assertEquals(listOf("reg", "delete", WindowsRunKey.KEY, "/v", "TailorIt", "/f"), calls.last())
        assertFalse(item.isEnabled())
    }

    @Test(expected = IllegalStateException::class)
    fun windowsFailureThrows() {
        WindowsRunKey("C:\\a.exe", { 5 }, tmp.root).setEnabled(true)
    }

    private class FakeLoginItem(var on: Boolean = false, var fail: Boolean = false) : LoginItem {
        override fun isEnabled() = on
        override fun setEnabled(on: Boolean) {
            if (fail) error("denied")
            this.on = on
        }
    }

    private fun toggles(c: TrayController) =
        c.items(emptyList()).filterIsInstance<TrayItem.Toggle>().map { "${it.command}=${it.checked}" }

    @Test
    fun trayShowsToggleOnlyWhenAvailableAndDefaultsOff() = Strings.withLang(Lang.JA) {
        val dsl = Examples.load("kakeibo")
        val without = TrayController(dsl, true, MemoryBoolSetting(true), MemoryBoolSetting(true))
        assertNull(without.openAtLogin)
        assertEquals(listOf("CLOSE_TO_TRAY=true"), toggles(without))

        val fake = FakeLoginItem()
        val c = TrayController(dsl, true, MemoryBoolSetting(true), MemoryBoolSetting(true), loginItem = fake)
        // 自分からは登録しない
        assertEquals(false, c.openAtLogin)
        assertFalse(fake.on)
        val toggle = c.items(emptyList()).filterIsInstance<TrayItem.Toggle>().last()
        assertEquals("ログイン時に開く", toggle.label)
        // 「終了」の直前
        assertEquals(TrayCommand.QUIT, (c.items(emptyList()).last() as TrayItem.Command).command)
        c.updateOpenAtLogin(true) { _, _ -> error("no notice") }
        assertTrue(fake.on)
        assertEquals(listOf("CLOSE_TO_TRAY=true", "OPEN_AT_LOGIN=true"), toggles(c))
    }

    @Test
    fun trayFailureNotifiesAndFollowsOsState() = Strings.withLang(Lang.EN) {
        val fake = FakeLoginItem(on = true, fail = true)
        val c = TrayController(Examples.load("kakeibo"), true, MemoryBoolSetting(true), MemoryBoolSetting(true), loginItem = fake)
        assertEquals(true, c.openAtLogin)
        val shown = mutableListOf<String>()
        c.updateOpenAtLogin(false) { _, body -> shown += body }
        assertEquals(listOf("Couldn't change the open-at-login setting"), shown)
        assertEquals(true, c.openAtLogin)
    }
}
