package jp.kz2601.metamon.desktop

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

/** ログイン時に開く: .desktop の中身・Run の値・使えない道の判定。 */
class AutostartEntryTest {
    @Test
    fun desktopEntryQuotesSpacesQuotesAndPercent() {
        val entry = XdgAutostart.desktopEntry("/opt/a \"b\" 100%")
        // Exec の引用で " を \" に、% を %% に、文字列の逃がしで \ を \\ に
        assertEquals(
            "[Desktop Entry]\nType=Application\nName=Tailor It\nExec=\"/opt/a \\\\\"b\\\\\" 100%%\"\nX-GNOME-Autostart-enabled=true\n",
            entry,
        )
        assertEquals(1, entry.lines().count { it.startsWith("Exec=") })
    }

    @Test
    fun desktopEntryKeepsPlainPathInQuotes() {
        val exec = XdgAutostart.desktopEntry("/opt/tailor/bin/app").lines().single { it.startsWith("Exec=") }
        assertEquals("Exec=\"/opt/tailor/bin/app\"", exec)
    }

    @Test
    fun runValueWrapsPathInQuotes() {
        assertEquals("\"C:\\a.exe\"", WindowsRunKey.runValue("C:\\a.exe"))
        assertEquals("\"C:\\Program Files\\Tailor It\\Tailor It.exe\"", WindowsRunKey.runValue("C:\\Program Files\\Tailor It\\Tailor It.exe"))
        // \ はそのまま(.reg に書く時だけ逃がす)
        assertEquals("\"\\\\server\\share\\a.exe\"", WindowsRunKey.runValue("\\\\server\\share\\a.exe"))
    }

    @Test
    fun runValueRoundTripsThroughRegFile() {
        val path = "C:\\Users\\家\\Tailor It.exe"
        assertEquals(WindowsRunKey.runValue(path), WindowsRunKey.parseValue(WindowsRunKey.regFile(path)))
    }

    @Test
    fun isSafePathRejectsLineBreaks() {
        assertTrue(Autostart.isSafePath("/opt/a b/c"))
        assertTrue(Autostart.isSafePath("C:\\a\tb.exe"))
        assertTrue(Autostart.isSafePath(""))
        assertFalse(Autostart.isSafePath("/opt/a\nb"))
        assertFalse(Autostart.isSafePath("C:\\a\rb.exe"))
        assertFalse(Autostart.isSafePath("C:\\a.exe\r\n"))
    }
}
