package jp.kz2601.metamon.desktop

import org.junit.Assert.assertEquals
import org.junit.Test

/** グラフの軸の目盛りの文字(axisLabel・#2799)。6 文字までそのまま、超えたら 5 文字 + 「…」。 */
class AxisLabelTest {
    @Test
    fun shortLabelsStayAsIs() {
        assertEquals("", axisLabel(""))
        assertEquals("0", axisLabel("0"))
        assertEquals("食費", axisLabel("食費"))
        assertEquals("123456", axisLabel("123456"))
    }

    @Test
    fun largeNumbersAreCutAfterFiveChars() {
        assertEquals("1,000", axisLabel("1,000"))
        assertEquals("1,000…", axisLabel("1,000,000"))
        assertEquals("12345…", axisLabel("1234567"))
        assertEquals("1.2万", axisLabel("1.2万"))
        assertEquals("1000万", axisLabel("1000万"))
    }

    @Test
    fun decimalsAndNegatives() {
        assertEquals("-12.5", axisLabel("-12.5"))
        assertEquals("0.0001", axisLabel("0.0001"))
        assertEquals("-1234…", axisLabel("-1234.56"))
        assertEquals("3.141…", axisLabel("3.14159"))
    }

    @Test
    fun dates() {
        assertEquals("11/13", axisLabel("11/13"))
        assertEquals("2026-…", axisLabel("2026-11-13"))
        assertEquals("2026年…", axisLabel("2026年11月"))
    }

    @Test
    fun countsCodePointsAndNeverSplitsSurrogates() {
        // 6 個の絵文字(12 char)はそのまま
        assertEquals("😀😀😀😀😀😀", axisLabel("😀😀😀😀😀😀"))
        // 7 個なら 5 個 + …(サロゲートの片割れを残さない)
        val cut = axisLabel("😀😀😀😀😀😀😀")
        assertEquals("😀😀😀😀😀…", cut)
        assertEquals(6, cut.codePointCount(0, cut.length))
        assertEquals("a😀b😀c…", axisLabel("a😀b😀c😀d"))
    }
}
