package jp.kz2601.metamon.desktop.theme

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/** 明暗: 固定 > OS > Compose の判定。OS の読み方は Windows・Linux・macOS の出力から(#1759)。 */
class ThemeTest {
    @Test
    fun modeParsesAndDefaultsToSystem() {
        assertEquals(ThemeMode.DARK, ThemeMode.parse("dark"))
        assertEquals(ThemeMode.LIGHT, ThemeMode.parse(" Light "))
        assertEquals(ThemeMode.SYSTEM, ThemeMode.parse("system"))
        assertEquals(ThemeMode.SYSTEM, ThemeMode.parse(null))
        assertEquals(ThemeMode.SYSTEM, ThemeMode.parse("purple"))
    }

    @Test
    fun selectionPrefersFixedThenOsThenCompose() {
        assertTrue(OsTheme.isDark(ThemeMode.DARK, os = false, composeFallback = false))
        assertFalse(OsTheme.isDark(ThemeMode.LIGHT, os = true, composeFallback = true))
        assertTrue(OsTheme.isDark(ThemeMode.SYSTEM, os = true, composeFallback = false))
        assertFalse(OsTheme.isDark(ThemeMode.SYSTEM, os = false, composeFallback = true))
        assertTrue(OsTheme.isDark(ThemeMode.SYSTEM, os = null, composeFallback = true))
        assertFalse(OsTheme.isDark(ThemeMode.SYSTEM, os = null, composeFallback = false))
    }

    @Test
    fun windowsRegistry() {
        val dark = "\r\nHKEY_CURRENT_USER\\Software\\Microsoft\\Windows\\CurrentVersion\\Themes\\Personalize\r\n    AppsUseLightTheme    REG_DWORD    0x0\r\n"
        val light = dark.replace("0x0", "0x1")
        assertEquals(true, OsTheme.parseWindowsRegistry(dark))
        assertEquals(false, OsTheme.parseWindowsRegistry(light))
        assertNull(OsTheme.parseWindowsRegistry("ERROR: The system was unable to find the specified registry key or value."))
    }

    @Test
    fun linuxPortalGsettingsAndGtkTheme() {
        assertEquals(true, OsTheme.parsePortal("(<<uint32 1>>,)\n"))
        assertEquals(false, OsTheme.parsePortal("(<uint32 2>,)"))
        assertNull(OsTheme.parsePortal("(<<uint32 0>>,)"))
        assertNull(OsTheme.parsePortal("Error: GDBus.Error:org.freedesktop.DBus.Error.ServiceUnknown"))
        assertEquals(true, OsTheme.parseGsettings("'prefer-dark'\n"))
        assertEquals(false, OsTheme.parseGsettings("'default'"))
        assertNull(OsTheme.parseGsettings("No such key “color-scheme”"))
        assertEquals(true, OsTheme.parseGtkTheme("Adwaita:dark"))
        assertEquals(false, OsTheme.parseGtkTheme("Adwaita"))
        assertNull(OsTheme.parseGtkTheme(null))
    }

    @Test
    fun envDefaultIsFixedLightInTests() {
        // build.gradle.kts でテストは -Dtailorit.theme=light(機械の設定に左右されない)
        assertEquals(ThemeMode.LIGHT, ThemeMode.fromEnv())
    }

    // MARK: - detect: OS の名前で読み方を選ぶ(読む所は作り物)・#2627

    /** 呼ばれたコマンドの先頭の語を記録し、[answers] の出力を返す(無ければ失敗 = null)。 */
    private class FakeRunner(private val answers: Map<String, String>) {
        val called = mutableListOf<String>()
        fun run(cmd: List<String>): String? = cmd.first().also { called += it }.let(answers::get)
    }

    private fun detect(os: String, answers: Map<String, String>, gtk: String? = null): Pair<Boolean?, List<String>> {
        val r = FakeRunner(answers)
        return OsTheme.detect(os, r::run) { gtk } to r.called
    }

    @Test
    fun detectWindowsReadsTheRegistry() {
        val dark = "    AppsUseLightTheme    REG_DWORD    0x0\r\n"
        assertEquals(true to listOf("reg"), detect("windows 11", mapOf("reg" to dark)))
        assertEquals(false, detect("windows 10", mapOf("reg" to dark.replace("0x0", "0x1"))).first)
        assertEquals("読めなければ Compose に任せる", null to listOf("reg"), detect("windows 11", emptyMap(), gtk = "Adwaita:dark"))
    }

    @Test
    fun detectMacReadsDefaults() {
        assertEquals(true to listOf("defaults"), detect("mac os x", mapOf("defaults" to "Dark\n")))
        assertEquals("キーが無い(明るい)は失敗 = false", false to listOf("defaults"), detect("mac os x", emptyMap()))
    }

    @Test
    fun detectLinuxFallsBackFromPortalToGsettingsToGtkTheme() {
        assertEquals(true to listOf("gdbus"), detect("linux", mapOf("gdbus" to "(<<uint32 1>>,)", "gsettings" to "'prefer-light'")))
        assertEquals("ポータルが指定なしなら gsettings", false to listOf("gdbus", "gsettings"), detect("linux", mapOf("gdbus" to "(<<uint32 0>>,)", "gsettings" to "'default'")))
        assertEquals(true to listOf("gdbus", "gsettings"), detect("linux", emptyMap(), gtk = "Adwaita:dark"))
        assertEquals(null to listOf("gdbus", "gsettings"), detect("linux", emptyMap()))
    }

    @Test
    fun detectUnknownOsUsesTheLinuxWay() {
        assertEquals(true to listOf("gdbus", "gsettings"), detect("freebsd", mapOf("gsettings" to "'prefer-dark'")))
        assertEquals(null to listOf("gdbus", "gsettings"), detect("", emptyMap()))
    }
}
