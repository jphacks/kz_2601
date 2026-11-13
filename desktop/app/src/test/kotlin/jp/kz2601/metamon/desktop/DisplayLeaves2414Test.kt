package jp.kz2601.metamon.desktop

import androidx.compose.ui.graphics.Color
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test
import jp.kz2601.metamon.dsl.Color as DslColor

/** #2414: DisplayLeaves の純粋な関数(initials・dslColor・軸の文字の切り詰め)。 */
class DisplayLeaves2414Test {
    // MARK: initials

    @Test
    fun initialsEmptyOrBlankIsPersonMark() {
        assertEquals("👤", initials(null))
        assertEquals("👤", initials(""))
        assertEquals("👤", initials("   \t\n"))
    }

    @Test
    fun initialsSingleCharacter() {
        assertEquals("A", initials("a"))
        assertEquals("太", initials("太"))
    }

    @Test
    fun initialsJapaneseTakesFirstCharacterOnly() {
        assertEquals("山", initials("山田 太郎"))
        assertEquals("た", initials("たろう"))
    }

    @Test
    fun initialsEnglishTwoWordsTakesBothInitialsUppercased() {
        assertEquals("JD", initials("john doe"))
        assertEquals("AB", initials("  Alice   Bob Carol "))
    }

    @Test
    fun initialsEnglishOneWordTakesFirstLetter() {
        assertEquals("J", initials("john"))
    }

    @Test
    fun initialsNonAsciiLetterFirstWordIsOneCharacter() {
        assertEquals("É", initials("éric dupont"))
    }

    @Test
    fun initialsEmojiIsNotSplitInSurrogatePair() {
        assertEquals("😀", initials("😀"))
        assertEquals("😀", initials("😀 smile"))
        assertEquals("🇯", initials("🇯🇵 日本").take(2))
    }

    // MARK: dslColor

    @Test
    fun dslColorEveryValue() {
        assertEquals(Color(0xFFE5484D), dslColor(DslColor.RED))
        assertEquals(Color(0xFF30A46C), dslColor(DslColor.GREEN))
        assertEquals(Color(0xFFF76808), dslColor(DslColor.ORANGE))
        assertEquals(Color(0xFF0091FF), dslColor(DslColor.BLUE))
        assertEquals(Color(0xFF4F7CFF), dslColor(DslColor.ACCENT))
        assertEquals(Color(0xFF4F7CFF), dslColor(DslColor.PRIMARY))
        assertEquals(Color.Gray, dslColor(DslColor.SECONDARY))
    }

    @Test
    fun dslColorNullAndUnknownAreNull() {
        assertNull(dslColor(null))
        assertNull(dslColor(DslColor.UNKNOWN))
    }

    @Test
    fun dslColorCoversEveryEnumEntry() {
        // 新しい色が増えたら、ここで気づく(UNKNOWN 以外は色を返す)
        for (c in DslColor.entries) {
            if (c == DslColor.UNKNOWN) assertNull(c.name, dslColor(c)) else assertEquals(c.name, true, dslColor(c) != null)
        }
    }

    // MARK: axisLabel

    @Test
    fun axisLabelUpToSixCharactersIsKept() {
        assertEquals("", axisLabel(""))
        assertEquals("abcdef", axisLabel("abcdef"))
        assertEquals("あいうえおか", axisLabel("あいうえおか"))
    }

    @Test
    fun axisLabelSevenOrMoreIsCutToFivePlusEllipsis() {
        assertEquals("abcde…", axisLabel("abcdefg"))
        assertEquals("あいうえお…", axisLabel("あいうえおかき"))
        assertEquals("abcde…", axisLabel("abcdefghijklmnop"))
    }

    @Test
    fun axisLabelCountsAndCutsBySurrogatePairs() {
        val six = "😀".repeat(6)
        assertEquals(six, axisLabel(six))
        assertEquals("😀".repeat(5) + "…", axisLabel("😀".repeat(7)))
        assertEquals("a" + "😀".repeat(4) + "…", axisLabel("a" + "😀".repeat(8)))
    }
}
