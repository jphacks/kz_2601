package jp.kz2601.metamon.desktop.theme

import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.material3.ColorScheme
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.darkColorScheme
import androidx.compose.material3.lightColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.graphics.Color
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.delay
import kotlinx.coroutines.withContext
import java.util.concurrent.TimeUnit

/** 明暗の決め方。既定は OS に従う。`-Dtailorit.theme` / `TAILORIT_THEME` に light・dark・contrast で固定できる。 */
enum class ThemeMode {
    SYSTEM, LIGHT, DARK, CONTRAST;

    companion object {
        fun parse(raw: String?): ThemeMode = when (raw?.trim()?.lowercase()) {
            "light" -> LIGHT
            "dark" -> DARK
            "contrast" -> CONTRAST
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

    /** 明暗を決める。固定があればそれ、無ければ OS、OS が読めなければ Compose の判定。コントラストの固定は明暗を OS に任せる。 */
    fun isDark(mode: ThemeMode, os: Boolean?, composeFallback: Boolean): Boolean = when (mode) {
        ThemeMode.LIGHT -> false
        ThemeMode.DARK -> true
        ThemeMode.SYSTEM, ThemeMode.CONTRAST -> os ?: composeFallback
    }

    /** GNOME の `gsettings get org.gnome.desktop.a11y.interface high-contrast` の出力。`true` / `false`。 */
    fun parseGnomeHighContrast(out: String): Boolean? = out.trim().toBooleanStrictOrNull()

    /** OS のコントラストのテーマを読む。Windows は JNA、Linux は GNOME の設定、他は読まない(null)。 */
    fun detectHighContrast(os: String = System.getProperty("os.name").orEmpty().lowercase()): HighContrastState? = when {
        os.startsWith("windows") -> WindowsHighContrast.read()
        os.contains("mac") -> null
        else -> run("gsettings", "get", "org.gnome.desktop.a11y.interface", "high-contrast")
            ?.let(::parseGnomeHighContrast)?.let { HighContrastState(on = it) }
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

/** 使う配色。 */
enum class Palette { LIGHT, DARK, CONTRAST }

/** 配色の選び方とコントラストの配色の作り方(#2207)。 */
object ThemePalette {
    /** 明 / 暗の固定 > コントラストの固定 > OS のコントラスト > OS の明暗 > Compose の判定。 */
    fun choose(mode: ThemeMode, osDark: Boolean?, osContrast: HighContrastState?, composeFallback: Boolean): Palette = when {
        mode == ThemeMode.LIGHT -> Palette.LIGHT
        mode == ThemeMode.DARK -> Palette.DARK
        mode == ThemeMode.CONTRAST || osContrast?.on == true -> Palette.CONTRAST
        OsTheme.isDark(mode, osDark, composeFallback) -> Palette.DARK
        else -> Palette.LIGHT
    }

    /** OS の色が読めない時の配色。暗い側ならハイコントラスト黒、明るい側ならハイコントラスト白。 */
    fun contrastColors(sys: SystemColors?, dark: Boolean): SystemColors =
        sys ?: if (dark) SystemColors.ContrastBlack else SystemColors.ContrastWhite

    /** 背景・文字・枠・選択を OS の色から作る。枠は文字の色にして、灰色の薄い線を残さない。 */
    fun contrastScheme(c: SystemColors): ColorScheme {
        val bg = rgb(c.window)
        val fg = rgb(c.windowText)
        val sel = rgb(c.highlight)
        val onSel = rgb(c.highlightText)
        val link = rgb(c.hotlight)
        val base = if (luminance(c.window) < 0.5) darkColorScheme() else lightColorScheme()
        return base.copy(
            primary = sel, onPrimary = onSel, primaryContainer = sel, onPrimaryContainer = onSel, inversePrimary = link,
            secondary = link, onSecondary = bg, secondaryContainer = sel, onSecondaryContainer = onSel,
            tertiary = link, onTertiary = bg, tertiaryContainer = sel, onTertiaryContainer = onSel,
            background = bg, onBackground = fg,
            surface = bg, onSurface = fg, surfaceVariant = bg, onSurfaceVariant = fg, surfaceTint = bg,
            inverseSurface = fg, inverseOnSurface = bg,
            surfaceBright = bg, surfaceDim = bg,
            surfaceContainer = bg, surfaceContainerHigh = bg, surfaceContainerHighest = bg,
            surfaceContainerLow = bg, surfaceContainerLowest = bg,
            outline = fg, outlineVariant = fg, scrim = Color.Black,
        )
    }

    fun scheme(palette: Palette, osContrast: HighContrastState?, osDark: Boolean): ColorScheme = when (palette) {
        Palette.LIGHT -> lightColorScheme()
        Palette.DARK -> darkColorScheme()
        Palette.CONTRAST -> contrastScheme(contrastColors(osContrast?.colors, osDark))
    }

    private fun rgb(v: Int) = Color(0xFF000000.toInt() or (v and 0xFFFFFF))

    /** 0〜1 のおおよその明るさ。 */
    internal fun luminance(v: Int): Double {
        val r = (v shr 16) and 0xFF
        val g = (v shr 8) and 0xFF
        val b = v and 0xFF
        return (0.2126 * r + 0.7152 * g + 0.0722 * b) / 255.0
    }
}

/** 起動中に OS の明暗・コントラストが変わっても追いかける間隔。 */
private const val POLL_MS = 5_000L

/** アプリの Material テーマ。OS のダークモードとコントラストのテーマに従う(#1759・#2207)。 */
@Composable
fun TailorItTheme(mode: ThemeMode = ThemeMode.fromEnv(), content: @Composable () -> Unit) {
    val fallback = isSystemInDarkTheme()
    var os by remember { mutableStateOf<Boolean?>(null) }
    var contrast by remember { mutableStateOf<HighContrastState?>(null) }
    if (mode == ThemeMode.SYSTEM || mode == ThemeMode.CONTRAST) {
        LaunchedEffect(Unit) {
            while (true) {
                val (d, c) = withContext(Dispatchers.IO) { OsTheme.detect() to OsTheme.detectHighContrast() }
                os = d
                contrast = c
                delay(POLL_MS)
            }
        }
    }
    val dark = OsTheme.isDark(mode, os, fallback)
    val palette = ThemePalette.choose(mode, os, contrast, fallback)
    MaterialTheme(colorScheme = ThemePalette.scheme(palette, contrast, dark), content = content)
}
