package jp.kz2601.metamon.desktop.theme

import androidx.compose.ui.graphics.Color
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/** #2207: 配色の選び方(明 / 暗 / コントラスト)とコントラストの配色の作り方。 */
class HighContrast2207Test {
    private val on = HighContrastState(on = true)
    private val off = HighContrastState(on = false)

    @Test
    fun contrastModeParses() {
        assertEquals(ThemeMode.CONTRAST, ThemeMode.parse("contrast"))
        assertEquals(ThemeMode.CONTRAST, ThemeMode.parse(" Contrast "))
    }

    @Test
    fun choose_fixedLightDarkWinOverOsContrast() {
        assertEquals(Palette.LIGHT, ThemePalette.choose(ThemeMode.LIGHT, osDark = true, osContrast = on, composeFallback = true))
        assertEquals(Palette.DARK, ThemePalette.choose(ThemeMode.DARK, osDark = false, osContrast = on, composeFallback = false))
    }

    @Test
    fun choose_contrastFixedOrFromOs() {
        assertEquals(Palette.CONTRAST, ThemePalette.choose(ThemeMode.CONTRAST, osDark = false, osContrast = null, composeFallback = false))
        assertEquals(Palette.CONTRAST, ThemePalette.choose(ThemeMode.SYSTEM, osDark = false, osContrast = on, composeFallback = false))
    }

    @Test
    fun choose_systemWithoutContrastFollowsDarkness() {
        assertEquals(Palette.DARK, ThemePalette.choose(ThemeMode.SYSTEM, osDark = true, osContrast = off, composeFallback = false))
        assertEquals(Palette.LIGHT, ThemePalette.choose(ThemeMode.SYSTEM, osDark = false, osContrast = null, composeFallback = true))
        assertEquals(Palette.DARK, ThemePalette.choose(ThemeMode.SYSTEM, osDark = null, osContrast = null, composeFallback = true))
    }

    @Test
    fun colorRefIsBgrToRgb() {
        // COLORREF 0x00BBGGRR: 赤 = 0x0000FF
        assertEquals(0xFF0000, SystemColors.fromColorRef(0x0000FF))
        assertEquals(0x0000FF, SystemColors.fromColorRef(0xFF0000))
        assertEquals(0x123456, SystemColors.fromColorRef(0x563412))
    }

    @Test
    fun windowsFlag() {
        assertTrue(WindowsHighContrast.isOn(0x1))
        assertTrue(WindowsHighContrast.isOn(0x7F))
        assertFalse(WindowsHighContrast.isOn(0x2))
    }

    @Test
    fun gnomeHighContrast() {
        assertEquals(true, OsTheme.parseGnomeHighContrast("true\n"))
        assertEquals(false, OsTheme.parseGnomeHighContrast("false"))
        assertNull(OsTheme.parseGnomeHighContrast("No such schema “org.gnome.desktop.a11y.interface”"))
    }

    @Test
    fun contrastScheme_usesSystemColorsForBackgroundTextBorderSelection() {
        val sys = SystemColors(
            window = 0x000000, windowText = 0xFFFFFF, highlight = 0x00FF00, highlightText = 0x000000,
            grayText = 0x808080, hotlight = 0xFFFF00,
        )
        val s = ThemePalette.scheme(Palette.CONTRAST, HighContrastState(on = true, colors = sys), osDark = false)
        assertEquals(Color.Black, s.background)
        assertEquals(Color.Black, s.surface)
        assertEquals(Color.White, s.onBackground)
        assertEquals(Color.White, s.onSurface)
        assertEquals(Color.White, s.outline)
        assertEquals(Color.White, s.outlineVariant)
        assertEquals(Color(0xFF00FF00), s.primary)
        assertEquals(Color.Black, s.onPrimary)
        assertEquals(Color(0xFF00FF00), s.secondaryContainer)
        assertEquals(Color(0xFFFFFF00), s.secondary)
    }

    @Test
    fun contrastScheme_withoutOsColorsPicksBlackOrWhiteByDarkness() {
        val dark = ThemePalette.scheme(Palette.CONTRAST, osContrast = null, osDark = true)
        assertEquals(Color.Black, dark.background)
        assertEquals(Color.White, dark.onBackground)
        val light = ThemePalette.scheme(Palette.CONTRAST, osContrast = on, osDark = false)
        assertEquals(Color.White, light.background)
        assertEquals(Color.Black, light.onBackground)
    }

    @Test
    fun readersAreNullOffWindows() {
        // テストは Windows の外(CI の Linux・手元の Mac)で走る。JNA を載せずに null を返す
        if (!System.getProperty("os.name").orEmpty().startsWith("Windows", ignoreCase = true)) {
            assertNull(WindowsHighContrast.read())
            assertNull(OsTheme.detectHighContrast("mac os x"))
        }
    }
}
