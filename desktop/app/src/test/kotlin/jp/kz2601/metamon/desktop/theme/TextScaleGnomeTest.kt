package jp.kz2601.metamon.desktop.theme

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

/** GNOME の text-scaling-factor の読み方(#3001)。 */
class TextScaleGnomeTest {
    @Test
    fun parsesGsettingsOutput() {
        assertEquals(1.25f, TextScale.parseGsettings("1.25\n")!!, 0.001f)
        assertEquals(1.5f, TextScale.parseGsettings("double 1.5")!!, 0.001f)
        assertEquals(2f, TextScale.parseGsettings("2")!!, 0.001f)
    }

    @Test
    fun clampsOutOfRange() {
        assertEquals(1f, TextScale.parseGsettings("0.5")!!, 0f)
        assertEquals(3f, TextScale.parseGsettings("4.0")!!, 0f)
    }

    @Test
    fun unreadableIsOne() {
        assertNull(TextScale.parseGsettings(""))
        assertNull(TextScale.parseGsettings("No such schema"))
        assertEquals(1f, TextScale.detect("linux") { null }, 0f)
        assertEquals(1f, TextScale.detect("linux") { "garbage" }, 0f)
        assertEquals(1.25f, TextScale.detect("linux") { cmd -> if (cmd[0] == "gsettings") "1.25\n" else null }, 0.001f)
    }
}
