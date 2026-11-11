package jp.kz2601.metamon.dsl.logic

import org.junit.Assert.assertEquals
import org.junit.Test

/** iOS の TextHighlightTests(照合と語の決め方)の写し。 */
class TextHighlightsTest {
    private fun marked(text: String, words: List<String>): List<String> =
        HighlightWords.ranges(text, words).map { text.substring(it.first, it.last + 1) }

    @Test
    fun matchesCaseWidthAndJapaneseSubstrings() {
        assertEquals(listOf("TypeError"), marked("TypeError: Cannot read", listOf("typeerror")))
        assertEquals("全角の文字も合う(元の文字のまま印)", listOf("ＴｙｐｅＥｒｒｏｒ"), marked("ＴｙｐｅＥｒｒｏｒ が出た", listOf("TypeError")))
        assertEquals("半角カナも合う", listOf("ﾀｲﾑｱｳﾄ"), marked("ﾀｲﾑｱｳﾄしました", listOf("タイムアウト")))
        assertEquals("日本語は語の一部でも合う", listOf("タイムアウト"), marked("接続がタイムアウトしました", listOf("タイムアウト")))
        assertEquals("何度出ても全部", listOf("null", "null"), marked("null と null", listOf("null")))
        assertEquals("重なりはまとめる", listOf("undefined is not"), marked("undefined is not", listOf("undefined is", "is not")))
        assertEquals(emptyList<String>(), marked("abc", listOf("x", "")))
        assertEquals(emptyList<String>(), marked("", listOf("a")))
        assertEquals("合成された絵文字の後ろでもずれない", listOf("ok"), marked("👍🏽 ok", listOf("ok")))
    }

    @Test
    fun wordsFromFieldAndCap() {
        fun fromField(value: String?) = HighlightWords.resolve(null, "keywords", value)
        assertEquals(
            "「,」「、」「，」改行で区切り・空と重複は捨てる",
            listOf("TypeError", "undefined", "タイムアウト", "null"),
            fromField("TypeError, undefined、タイムアウト\nnull，null ,  "),
        )
        assertEquals(emptyList<String>(), fromField(null))
        val many = (1..30).joinToString(",") { "w$it" }
        assertEquals("語は 20 個まで", HighlightWords.MAX_WORDS, fromField(many).size)
        assertEquals(listOf("期限"), HighlightWords.resolve(listOf("期限"), null, "無視"))
    }
}
