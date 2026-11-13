package jp.kz2601.metamon.desktop.voice

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

class PositionHintTest {
    private fun a(x: Float, y: Float) = PositionHint.Anchor(x, y)

    @Test
    fun japaneseWords() {
        val cases = mapOf(
            "右下のボタンから追加できます" to a(0.85f, 0.85f),
            "左下に保存があります" to a(0.15f, 0.85f),
            "右上の設定から変えられます" to a(0.85f, 0.15f),
            "左上に戻るがあります" to a(0.15f, 0.15f),
            "一番下に送信があります" to a(0.5f, 0.88f),
            "一番上に見出しがあります" to a(0.5f, 0.12f),
            "下部の欄に入れます" to a(0.5f, 0.85f),
            "上部の欄に入れます" to a(0.5f, 0.15f),
            "中央のカードを押します" to a(0.5f, 0.5f),
            "右のボタンで追加" to a(0.85f, 0.5f),
            "左のボタンで戻る" to a(0.15f, 0.5f),
        )
        for ((text, expected) in cases) assertEquals(text, expected, PositionHint.anchor(text))
    }

    @Test
    fun englishWordsIgnoreCase() {
        val cases = mapOf(
            "Use the button at the Bottom Right" to a(0.85f, 0.85f),
            "see bottom left" to a(0.15f, 0.85f),
            "Top right menu" to a(0.85f, 0.15f),
            "top left back" to a(0.15f, 0.15f),
            "Send is at the bottom" to a(0.5f, 0.85f),
            "Title is at the top" to a(0.5f, 0.15f),
            "Card in the center" to a(0.5f, 0.5f),
        )
        for ((text, expected) in cases) assertEquals(text, expected, PositionHint.anchor(text))
    }

    @Test
    fun cornerBeatsSide() {
        assertEquals(a(0.85f, 0.85f), PositionHint.anchor("右にあって、右下にもあります"))
    }

    @Test
    fun firstTableEntryWins() {
        assertEquals(a(0.15f, 0.85f), PositionHint.anchor("右上ではなく左下"))
    }

    @Test
    fun noWordGivesNull() {
        assertNull(PositionHint.anchor("追加できます"))
        assertNull(PositionHint.anchor(""))
    }
}
