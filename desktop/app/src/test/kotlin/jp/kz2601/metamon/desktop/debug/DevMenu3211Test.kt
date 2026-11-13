package jp.kz2601.metamon.desktop.debug

import jp.kz2601.metamon.desktop.i18n.Lang
import jp.kz2601.metamon.desktop.i18n.Strings
import jp.kz2601.metamon.desktop.net.DesktopApiConfig
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

/** #3211: 開発用メニューの「今の環境」「この端末のデータを消す」は stg の時だけ出る。 */
class DevMenu3211Test {
    private val stg = DesktopApiConfig.fromEnvironment(mapOf("TAILORIT_API" to "stg", "TAILORIT_DEV_TOKEN" to "secret-token"))
    private val mock = DesktopApiConfig.fromEnvironment(emptyMap())

    private class Host(override val config: DesktopApiConfig, override val pendingCount: Int? = 3, override val canWipe: Boolean = true) : DevMenuHost {
        var info: Pair<String, List<String>>? = null
        var wipeAsked = 0
        override fun showInfo(title: String, lines: List<String>) { info = title to lines }
        override fun askWipe() { wipeAsked++ }
    }

    @Test
    fun enabledOnlyForStgAndNotInPackagedBuilds() {
        assertTrue(DevMenu.isEnabled(stg, packaged = false))
        assertFalse(DevMenu.isEnabled(mock, packaged = false))
        assertFalse(DevMenu.isEnabled(stg, packaged = true))
        assertFalse(DevMenu.isPackaged(null))
        assertFalse(DevMenu.isPackaged(""))
        assertTrue(DevMenu.isPackaged("/Applications/Tailor It.app/Contents/MacOS/Tailor It"))
    }

    @Test
    fun paletteShowsEnvironmentAndWipeOnlyForStg() {
        assertEquals(emptyList<String>(), DevMenu.commands(Host(mock), enabled = DevMenu.isEnabled(mock, packaged = false)).map { it.id })
        assertEquals(emptyList<String>(), DevMenu.commands(Host(stg), enabled = DevMenu.isEnabled(stg, packaged = true)).map { it.id })
        val ids = DevMenu.commands(Host(stg), enabled = DevMenu.isEnabled(stg, packaged = false)).map { it.id }
        assertTrue(ids.containsAll(listOf("dev:environment", "dev:wipe")))
    }

    @Test
    fun wipeAsksFirstAndIsHiddenWithoutAWipeHook() {
        val host = Host(stg)
        DevMenu.commands(host, enabled = true).single { it.id == "dev:wipe" }.run()
        assertEquals(1, host.wipeAsked)
        assertFalse(DevMenu.commands(Host(stg, canWipe = false), enabled = true).any { it.id == "dev:wipe" })
    }

    @Test
    fun environmentShowsTargetVersionPendingButNotTheToken() = Strings.withLang(Lang.EN) {
        val host = Host(stg, pendingCount = 4)
        DevMenu.commands(host, enabled = true).single { it.id == "dev:environment" }.run()
        val lines = host.info!!.second
        assertTrue(lines.any { it.contains("stg") && it.contains(DesktopApiConfig.STG_APPCONFIG_URL) })
        assertTrue(lines.any { it.contains("4") })
        assertTrue(lines.any { it.contains("0.1.0") })
        assertFalse(lines.any { it.contains("secret-token") })
        val offline = DevEnvironmentItems.lines(mock, pending = null, version = "9.9.9", build = "7")
        assertEquals(listOf("Server: Mock (offline)", "Version: 9.9.9 (7)", "Waiting to send: -", "Token: none"), offline)
    }
}
