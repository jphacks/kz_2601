package jp.kz2601.metamon.desktop.theme

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

/** 文字の倍率: 値の読み方・範囲外・Ctrl の倍率との掛け合わせ(#3000)。 */
class TextScaleTest {
    private fun reg(hex: String) = "\r\nHKEY_CURRENT_USER\\Software\\Microsoft\\Accessibility\r\n    TextScaleFactor    REG_DWORD    0x$hex\r\n"

    @Test
    fun parsesWindowsRegistry() {
        assertEquals(1.8f, TextScale.parseWindowsRegistry(reg("b4"))!!, 0.001f)
        assertEquals(1.0f, TextScale.parseWindowsRegistry(reg("64"))!!, 0.001f)
        assertEquals(2.25f, TextScale.parseWindowsRegistry(reg("e1"))!!, 0.001f)
    }

    @Test
    fun clampsOutOfRange() {
        assertEquals(1.0f, TextScale.parseWindowsRegistry(reg("32"))!!, 0.001f)
        assertEquals(2.25f, TextScale.parseWindowsRegistry(reg("1f4"))!!, 0.001f)
    }

    @Test
    fun unreadableIsNullOrOne() {
        assertNull(TextScale.parseWindowsRegistry("ERROR: The system was unable to find the specified registry key or value."))
        assertNull(TextScale.parseWindowsRegistry(""))
        assertEquals(1f, TextScale.detect("windows 11") { null }, 0f)
        assertEquals(1f, TextScale.detect("windows 11") { "garbage" }, 0f)
        assertEquals(1.5f, TextScale.detect("windows 11") { reg("96") }, 0.001f)
        assertEquals(1f, TextScale.detect("mac os x") { reg("96") }, 0f)
    }

    @Test
    fun combinesWithCtrlScaleAndCaps() {
        assertEquals(1.8f, TextScale.combine(1.8f, 1f), 0.001f)
        assertEquals(1.8f * 1.25f, TextScale.combine(1.8f, 1.25f), 0.001f)
        assertEquals(TextScale.MAX_COMBINED, TextScale.combine(2.25f, 2f), 0f)
        assertEquals(TextScale.MIN_COMBINED, TextScale.combine(1f, 0.1f), 0f)
    }
}
