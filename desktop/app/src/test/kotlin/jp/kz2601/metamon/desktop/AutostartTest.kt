package jp.kz2601.metamon.desktop

import jp.kz2601.metamon.desktop.i18n.Lang
import jp.kz2601.metamon.desktop.i18n.Strings
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.runBlocking
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
    fun pathsWithLineBreaksAreRejected() {
        val home = tmp.root.absolutePath
        assertNull(Autostart.forCurrentOs("Windows 11", appPath = "C:\\a.exe\r\n", env = emptyMap(), home = home))
        assertNull(Autostart.forCurrentOs("Linux", appPath = "/opt/a\nb", env = emptyMap(), home = home))
        assertFalse(runCatching { WindowsRunKey("C:\\a\rb.exe", { 0 }, tmp.root) }.isSuccess)
        assertFalse(runCatching { XdgAutostart("/opt/a\nb", tmp.root) }.isSuccess)
    }

    @Test
    fun regExeIsAbsolute() {
        assertEquals(File("D:\\Win", "System32\\reg.exe").path, WindowsRunKey.defaultRegExe("D:\\Win"))
        assertEquals(File("C:\\Windows", "System32\\reg.exe").path, WindowsRunKey.defaultRegExe(null))
        assertEquals(File("C:\\Windows", "System32\\reg.exe").path, WindowsRunKey.defaultRegExe(""))
    }

    @Test
    fun xdgDirFollowsConfigHome() {
        // "/x" は Windows では絶対パスでないので、どの OS でも絶対になる tmp から作る
        val cfg = File(tmp.root, "cfg").absolutePath
        val home = File(tmp.root, "home").absolutePath
        val fallback = File(File(home, ".config"), "autostart")
        assertEquals(File(cfg, "autostart"), XdgAutostart.dir(mapOf("XDG_CONFIG_HOME" to cfg), home))
        assertEquals(fallback, XdgAutostart.dir(emptyMap(), home))
        // 相対の XDG_CONFIG_HOME は決まりどおり無視する
        assertEquals(fallback, XdgAutostart.dir(mapOf("XDG_CONFIG_HOME" to "rel"), home))
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
    fun xdgEntryForAnotherPathIsRegisteredButNotEnabled() {
        val dir = File(tmp.root, "autostart")
        XdgAutostart("/opt/old/Tailor It", dir).setEnabled(true)
        val current = XdgAutostart("/opt/new/Tailor It", dir)
        assertTrue(current.isRegistered())
        assertFalse(current.isEnabled())
        current.setEnabled(true)
        assertTrue(current.isEnabled())
    }

    @Test
    fun execQuoteEscapesReservedCharacters() {
        assertEquals("\"/opt/a b\"", XdgAutostart.execQuote("/opt/a b"))
        // Exec の引用(\$ \" \` \\)の後に、文字列としての逃がしで \ が倍になる
        assertEquals("\"/x/\\\\\$y\\\\\"z\\\\`w\\\\\\\\v 100%%\"", XdgAutostart.execQuote("/x/\$y\"z`w\\v 100%"))
    }

    @Test
    fun windowsRegFileKeepsQuotesAroundPath() {
        val reg = WindowsRunKey.regFile("C:\\Program Files\\Tailor It\\Tailor It.exe")
        assertTrue(reg.startsWith("Windows Registry Editor Version 5.00\r\n\r\n[HKEY_CURRENT_USER\\Software\\Microsoft\\Windows\\CurrentVersion\\Run]\r\n"))
        // 値は "C:\Program Files\...\Tailor It.exe"(引用符付き)
        assertTrue(reg.endsWith("\"TailorIt\"=\"\\\"C:\\\\Program Files\\\\Tailor It\\\\Tailor It.exe\\\"\"\r\n"))
        // 書いた物を読み戻すと同じ値
        assertEquals("\"C:\\Program Files\\Tailor It\\Tailor It.exe\"", WindowsRunKey.parseValue(reg))
        assertNull(WindowsRunKey.parseValue("Windows Registry Editor Version 5.00\r\n\r\n[HKEY_CURRENT_USER\\x]\r\n\"Other\"=\"a\"\r\n"))
    }

    /** reg.exe の偽物: Run の TailorIt の値を 1 つ持つ。 */
    private class FakeReg {
        val calls = mutableListOf<List<String>>()
        var value: String? = null
        var imported: ByteArray? = null

        val runner = CommandRunner { cmd ->
            calls += cmd
            when (cmd[1]) {
                "export" -> {
                    val body = "Windows Registry Editor Version 5.00\r\n\r\n[HKEY_CURRENT_USER\\Software\\Microsoft\\Windows\\CurrentVersion\\Run]\r\n" +
                        "\"Other\"=\"x\"\r\n" +
                        (value?.let { WindowsRunKey.regFile(it.removeSurrounding("\"")).substringAfterLast("]\r\n") } ?: "")
                    File(cmd[3]).writeBytes(byteArrayOf(0xFF.toByte(), 0xFE.toByte()) + body.toByteArray(StandardCharsets.UTF_16LE))
                    0
                }
                "import" -> {
                    val bytes = File(cmd[2]).readBytes()
                    imported = bytes
                    value = WindowsRunKey.parseValue(WindowsRunKey.decodeReg(bytes))
                    0
                }
                "delete" -> {
                    value = null
                    0
                }
                else -> 1
            }
        }
    }

    @Test
    fun windowsRunsRegAndImportsUtf16File() {
        val reg = FakeReg()
        val item = WindowsRunKey("C:\\家\\Tailor It.exe", reg.runner, tmp.root, regExe = "C:\\Windows\\System32\\reg.exe")
        assertFalse(item.isEnabled())
        item.setEnabled(false) // 無ければ delete を呼ばない
        assertEquals(listOf("export", "export"), reg.calls.map { it[1] })
        // reg は絶対パスで呼ぶ
        assertTrue(reg.calls.all { it[0] == "C:\\Windows\\System32\\reg.exe" })
        item.setEnabled(true)
        assertTrue(item.isEnabled())
        val bytes = reg.imported!!
        assertEquals(listOf(0xFF.toByte(), 0xFE.toByte()), bytes.take(2))
        assertTrue(String(bytes, 2, bytes.size - 2, StandardCharsets.UTF_16LE).contains("C:\\\\家\\\\Tailor It.exe"))
        // 一時の .reg は残さない
        assertEquals(emptyList<String>(), tmp.root.list()!!.filter { it.endsWith(".reg") })
        item.setEnabled(false)
        assertEquals(listOf("C:\\Windows\\System32\\reg.exe", "delete", WindowsRunKey.KEY, "/v", "TailorIt", "/f"), reg.calls.last())
        assertFalse(item.isEnabled())
    }

    @Test
    fun windowsValueForAnotherPathIsRegisteredButNotEnabled() {
        val reg = FakeReg().apply { value = "\"C:\\old\\Tailor It.exe\"" }
        val item = WindowsRunKey("C:\\new\\Tailor It.exe", reg.runner, tmp.root)
        assertTrue(item.isRegistered())
        assertFalse(item.isEnabled())
        item.setEnabled(true)
        assertEquals("\"C:\\new\\Tailor It.exe\"", reg.value)
        assertTrue(item.isEnabled())
    }

    @Test(expected = IllegalStateException::class)
    fun windowsFailureThrows() {
        WindowsRunKey("C:\\a.exe", { 5 }, tmp.root).setEnabled(true)
    }

    /** [registered] は前の道で登録が残っている状態。 */
    private class FakeLoginItem(var on: Boolean = false, var fail: Boolean = false, var registered: Boolean = false) : LoginItem {
        var onSet: () -> Unit = {}
        var sets = 0
        override fun isEnabled() = on
        override fun isRegistered() = on || registered
        override fun setEnabled(on: Boolean) {
            onSet()
            sets++
            if (fail) error("denied")
            this.on = on
            registered = false
        }
    }

    private fun controller(fake: FakeLoginItem?) =
        TrayController(Examples.load("kakeibo"), true, MemoryBoolSetting(true), MemoryBoolSetting(true), loginItem = fake, loginItemDispatcher = Dispatchers.Unconfined)

    private fun loginToggle(c: TrayController) =
        c.items(emptyList()).filterIsInstance<TrayItem.Toggle>().single { it.command == TrayCommand.OPEN_AT_LOGIN }

    private fun toggles(c: TrayController) =
        c.items(emptyList()).filterIsInstance<TrayItem.Toggle>().map { "${it.command}=${it.checked}" }

    @Test
    fun trayShowsToggleOnlyWhenAvailableAndDefaultsOff() = Strings.withLang(Lang.JA) {
        val without = controller(null)
        assertNull(without.openAtLogin)
        assertEquals(listOf("CLOSE_TO_TRAY=true"), toggles(without))

        val fake = FakeLoginItem()
        val c = controller(fake)
        // 読み込むまでは off で押せない
        assertEquals(false, loginToggle(c).checked)
        assertFalse(loginToggle(c).enabled)
        runBlocking { c.loadOpenAtLogin() }
        assertTrue(loginToggle(c).enabled)
        // 自分からは登録しない
        assertEquals(false, c.openAtLogin)
        assertFalse(fake.on)
        assertEquals(0, fake.sets)
        assertEquals("ログイン時に開く", loginToggle(c).label)
        // 「終了」の直前
        assertEquals(TrayCommand.QUIT, (c.items(emptyList()).last() as TrayItem.Command).command)
        runBlocking { c.updateOpenAtLogin(true) { _, _ -> error("no notice") } }
        assertTrue(fake.on)
        assertEquals(listOf("CLOSE_TO_TRAY=true", "OPEN_AT_LOGIN=true"), toggles(c))
    }

    @Test
    fun trayLoadsStateFromOs() {
        val c = controller(FakeLoginItem(on = true))
        runBlocking { c.loadOpenAtLogin() }
        assertTrue(loginToggle(c).checked)
    }

    @Test
    fun trayRewritesStaleEntryAtStartup() {
        val fake = FakeLoginItem(registered = true)
        val c = controller(fake)
        runBlocking { c.loadOpenAtLogin() }
        assertEquals(1, fake.sets)
        assertTrue(fake.on)
        assertTrue(loginToggle(c).checked)
    }

    @Test
    fun trayToggleIsDisabledWhileRunning() {
        val fake = FakeLoginItem()
        val c = controller(fake)
        runBlocking { c.loadOpenAtLogin() }
        var enabledDuring: Boolean? = null
        fake.onSet = { enabledDuring = loginToggle(c).enabled }
        runBlocking { c.updateOpenAtLogin(true) { _, _ -> } }
        assertEquals(false, enabledDuring)
        assertTrue(loginToggle(c).enabled)
    }

    @Test
    fun trayFailureNotifiesAndRebuildsItemWithOsState() = Strings.withLang(Lang.EN) {
        val fake = FakeLoginItem(on = true, fail = true)
        val c = controller(fake)
        runBlocking { c.loadOpenAtLogin() }
        val before = loginToggle(c)
        assertTrue(before.checked)
        val shown = mutableListOf<String>()
        runBlocking { c.updateOpenAtLogin(false) { _, body -> shown += body } }
        assertEquals(listOf("Couldn't change the open-at-login setting"), shown)
        // 押した側で反転した行を、OS 側の状態(on)で作り直す
        val after = loginToggle(c)
        assertTrue(after.checked)
        assertTrue(after.enabled)
        assertTrue(after.revision != before.revision)
    }
}
