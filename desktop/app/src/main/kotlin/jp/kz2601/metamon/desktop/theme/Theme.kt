package jp.kz2601.metamon.desktop.theme

import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.darkColorScheme
import androidx.compose.material3.lightColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.delay
import kotlinx.coroutines.withContext
import java.util.concurrent.TimeUnit

/** 明暗の決め方。既定は OS に従う。`-Dtailorit.theme` / `TAILORIT_THEME` に light・dark で固定できる。 */
enum class ThemeMode {
    SYSTEM, LIGHT, DARK;

    companion object {
        fun parse(raw: String?): ThemeMode = when (raw?.trim()?.lowercase()) {
            "light" -> LIGHT
            "dark" -> DARK
            else -> SYSTEM
        }

        fun fromEnv(): ThemeMode = parse(System.getProperty("tailorit.theme") ?: System.getenv("TAILORIT_THEME"))
    }
}

/**
 * OS のダークモードを読む。読めない時は null(Compose の [isSystemInDarkTheme] に任せる)。
 * - Windows: レジストリ `HKCU\…\Themes\Personalize` の `AppsUseLightTheme`(0 = 暗い)
 * - Linux: freedesktop の設定ポータル `org.freedesktop.appearance color-scheme`(1 = 暗い・2 = 明るい)、無ければ GNOME の gsettings、`GTK_THEME` の `:dark`
 * - macOS: `defaults read -g AppleInterfaceStyle` が Dark
 */
object OsTheme {
    /** `reg query` の出力。`AppsUseLightTheme    REG_DWORD    0x0`。 */
    fun parseWindowsRegistry(out: String): Boolean? =
        Regex("""AppsUseLightTheme\s+REG_DWORD\s+0x([0-9a-fA-F]+)""").find(out)?.groupValues?.get(1)?.toIntOrNull(16)?.let { it == 0 }

    /** `gdbus call … org.freedesktop.portal.Settings.Read(One)` の出力。`(<<uint32 1>>,)` か `(<uint32 1>,)`。0(指定なし)は null。 */
    fun parsePortal(out: String): Boolean? = when (Regex("""uint32\s+(\d+)""").find(out)?.groupValues?.get(1)) {
        "1" -> true
        "2" -> false
        else -> null
    }

    /** `gsettings get org.gnome.desktop.interface color-scheme` の出力。`'prefer-dark'`。 */
    fun parseGsettings(out: String): Boolean? = when (out.trim().trim('\'')) {
        "prefer-dark" -> true
        "prefer-light", "default" -> false
        else -> null
    }

    /** `GTK_THEME=Adwaita:dark` のような指定。 */
    fun parseGtkTheme(value: String?): Boolean? = value?.takeIf { it.isNotBlank() }?.let { it.lowercase().endsWith(":dark") || it.lowercase().endsWith("-dark") }

    fun detect(os: String = System.getProperty("os.name").orEmpty().lowercase()): Boolean? = when {
        os.startsWith("windows") ->
            run("reg", "query", """HKCU\Software\Microsoft\Windows\CurrentVersion\Themes\Personalize""", "/v", "AppsUseLightTheme")?.let(::parseWindowsRegistry)
        os.contains("mac") -> run("defaults", "read", "-g", "AppleInterfaceStyle").let { it?.trim()?.equals("Dark", ignoreCase = true) ?: false }
        else -> run(
            "gdbus", "call", "--session", "--dest", "org.freedesktop.portal.Desktop", "--object-path", "/org/freedesktop/portal/desktop",
            "--method", "org.freedesktop.portal.Settings.Read", "org.freedesktop.appearance", "color-scheme",
        )?.let(::parsePortal)
            ?: run("gsettings", "get", "org.gnome.desktop.interface", "color-scheme")?.let(::parseGsettings)
            ?: parseGtkTheme(System.getenv("GTK_THEME"))
    }

    /** 明暗を決める。固定があればそれ、無ければ OS、OS が読めなければ Compose の判定。 */
    fun isDark(mode: ThemeMode, os: Boolean?, composeFallback: Boolean): Boolean = when (mode) {
        ThemeMode.LIGHT -> false
        ThemeMode.DARK -> true
        ThemeMode.SYSTEM -> os ?: composeFallback
    }

    private fun run(vararg cmd: String): String? = runCatching {
        val p = ProcessBuilder(*cmd).redirectErrorStream(true).start()
        if (!p.waitFor(2, TimeUnit.SECONDS)) {
            p.destroyForcibly()
            return null
        }
        val out = p.inputStream.bufferedReader().readText()
        if (p.exitValue() == 0) out else null
    }.getOrNull()
}

/** 起動中に OS の明暗が変わっても追いかける間隔。 */
private const val POLL_MS = 5_000L

/** アプリの Material テーマ。OS のダークモードに従う(#1759)。 */
@Composable
fun TailorItTheme(mode: ThemeMode = ThemeMode.fromEnv(), content: @Composable () -> Unit) {
    val fallback = isSystemInDarkTheme()
    var os by remember { mutableStateOf<Boolean?>(null) }
    if (mode == ThemeMode.SYSTEM) {
        LaunchedEffect(Unit) {
            while (true) {
                os = withContext(Dispatchers.IO) { OsTheme.detect() }
                delay(POLL_MS)
            }
        }
    }
    val dark = OsTheme.isDark(mode, os, fallback)
    MaterialTheme(colorScheme = if (dark) darkColorScheme() else lightColorScheme(), content = content)
}
