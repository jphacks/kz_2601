package jp.kz2601.metamon.desktop.crash

import jp.kz2601.metamon.desktop.DesktopKeyboard
import jp.kz2601.metamon.desktop.DesktopSession
import jp.kz2601.metamon.desktop.Examples
import jp.kz2601.metamon.desktop.HostOs
import jp.kz2601.metamon.desktop.MockStore
import jp.kz2601.metamon.desktop.net.DesktopApi
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

/** #2200: 問い合わせ用の「診断の情報」。入れる物は揃い、uid・トークン・記録・パスは入らない。 */
class Diagnostics2200Test {
    private val secretToken = "eyJhbGciOiJSUzI1NiJ9.secret-dev-token"
    private val home = "/home/taro-yamada"
    private val uid = "uid-8f3a2b"

    /** 入れてはいけない値をわざと混ぜた環境。 */
    private val props = mapOf(
        "os.name" to "Windows 11",
        "os.version" to "10.0",
        "os.arch" to "amd64",
        "java.version" to "21.0.5",
        "java.vendor" to "Eclipse Adoptium",
        "user.home" to home,
        "user.name" to "taro-yamada",
        "user.dir" to "$home/work",
        "java.io.tmpdir" to "$home/tmp",
    )
    private val env = mapOf(
        "TAILORIT_API" to "stg",
        "TAILORIT_DEV_TOKEN" to secretToken,
        "TAILORIT_APPCONFIG_URL" to "https://appconfig.internal.example/$uid",
        "APPDATA" to "$home\\AppData\\Roaming",
        "HOME" to home,
        "FIREBASE_UID" to uid,
    )

    private fun text(voiceEnabled: Boolean = true, voiceLibrary: Boolean? = true) = Diagnostics.format(
        Diagnostics.collect(
            props = props::get,
            env = env,
            dark = true,
            traySupported = false,
            pending = 3,
            voiceEnabled = voiceEnabled,
            voiceLibrary = { voiceLibrary },
            language = "ja",
        ),
    )

    @Test
    fun containsTheAgreedItems() {
        val t = text()
        assertTrue(t, t.contains("app: ${DesktopApi.VERSION} (${DesktopApi.BUILD})"))
        assertTrue(t, t.contains("os: Windows 11 10.0"))
        assertTrue(t, t.contains("arch: amd64"))
        assertTrue(t, t.contains("java: 21.0.5 (Eclipse Adoptium)"))
        assertTrue(t, t.contains("language: ja"))
        assertTrue(t, t.contains("theme: dark"))
        assertTrue(t, t.contains("voice: on (library: yes)"))
        assertTrue(t, t.contains("tray: no"))
        assertTrue(t, t.contains("api: stg"))
        assertTrue(t, t.contains("pending: 3"))
    }

    @Test
    fun leavesOutIdsTokensRecordsAndPaths() {
        val t = text()
        for (bad in listOf(secretToken, "secret", home, "taro-yamada", uid, "appconfig", "AppData", "/home", "\\")) {
            assertFalse("「$bad」が入っている:\n$t", t.contains(bad))
        }
    }

    @Test
    fun voiceOffDoesNotProbeTheLibrary() {
        var probed = false
        val info = Diagnostics.collect(
            props = props::get, env = env, dark = null, traySupported = null, pending = null,
            voiceEnabled = false, voiceLibrary = { probed = true; true }, language = "en",
        )
        assertFalse(probed)
        val t = Diagnostics.format(info)
        assertTrue(t, t.contains("voice: off (library: unknown)"))
        assertTrue(t, t.contains("theme: unknown"))
        assertTrue(t, t.contains("pending: unknown"))
    }

    @Test
    fun mockByDefaultAndValuesStayOnOneLine() {
        val info = Diagnostics.collect(
            props = mapOf("os.name" to "Linux\nInjected: yes", "os.version" to "", "os.arch" to "aarch64")::get,
            env = emptyMap(), dark = false, traySupported = true, pending = 0, voiceEnabled = false, language = "ja",
        )
        val t = Diagnostics.format(info)
        assertTrue(t, t.contains("api: mock"))
        assertTrue(t, t.contains("os: Linux Injected: yes -"))
        assertEquals(11, t.lines().size) // 見出し + 10 行
    }

    @Test
    fun paletteShowsTheEntryOnlyWhenWired() {
        val s = DesktopSession(Examples.load("shopping"), MockStore())
        val k = DesktopKeyboard(s.dsl, s, HostOs.WINDOWS)
        assertFalse(k.commands().any { it.id == "about:diagnostics" })
        var copied = 0
        k.copyDiagnostics = { copied++ }
        val c = k.commands().first { it.id == "about:diagnostics" }
        c.run()
        assertEquals(1, copied)
    }
}
