package jp.kz2601.metamon.desktop

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

/** 表示の部品: グラフの数の表記(chartNumber)とリンクにする URL の判定(Markdown.linkable)。 */
class DisplayLeavesChartLinkTest {
    @Test
    fun chartNumber_integersHaveNoDecimals() {
        assertEquals("0", chartNumber(0.0))
        assertEquals("3", chartNumber(3.0))
        assertEquals("100", chartNumber(100.0))
    }

    @Test
    fun chartNumber_decimalsUpToTwoPlaces() {
        assertEquals("1.5", chartNumber(1.5))
        assertEquals("1.25", chartNumber(1.25))
        // 3 桁目は丸める・後ろの 0 は付けない
        assertEquals("1.23", chartNumber(1.234))
        assertEquals("0.1", chartNumber(0.1))
        assertEquals("2", chartNumber(1.999))
    }

    @Test
    fun chartNumber_largeNumbersWithoutGrouping() {
        assertEquals("1234567", chartNumber(1_234_567.0))
        assertEquals("123456789", chartNumber(123_456_789.0))
        assertEquals("9999999.99", chartNumber(9_999_999.99))
    }

    @Test
    fun chartNumber_negative() {
        assertEquals("-2", chartNumber(-2.0))
        assertEquals("-0.5", chartNumber(-0.5))
        assertEquals("-1234.56", chartNumber(-1234.56))
    }

    @Test
    fun linkable_httpsAndOtherAllowedSchemes() {
        assertTrue(Markdown.linkable("https://example.com/a?b=c"))
        assertTrue(Markdown.linkable("HTTPS://EXAMPLE.COM"))
        assertTrue(Markdown.linkable("mailto:a@example.com"))
        assertTrue(Markdown.linkable("tel:0312345678"))
    }

    @Test
    fun linkable_trimsSurroundingWhitespace() {
        assertTrue(Markdown.linkable("  https://example.com  "))
        assertTrue(Markdown.linkable("\thttps://example.com\n"))
        assertFalse(Markdown.linkable("  javascript:alert(1)  "))
    }

    @Test
    fun linkable_rejectsOtherSchemesAndBareText() {
        assertFalse(Markdown.linkable("http://example.com"))
        assertFalse(Markdown.linkable("javascript:alert(1)"))
        assertFalse(Markdown.linkable("file:///etc/passwd"))
        assertFalse(Markdown.linkable("data:text/html,<b>x</b>"))
        assertFalse(Markdown.linkable("example.com"))
        assertFalse(Markdown.linkable("/relative/path"))
        assertFalse(Markdown.linkable(""))
        assertFalse(Markdown.linkable("   "))
    }
}
