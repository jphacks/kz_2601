package jp.kz2601.metamon.discover

import org.junit.Assert.assertEquals
import org.junit.Test

class DiscoverSheetTest {
    @Test
    fun initialKeepsEmojiAndCombiningCharactersWhole() {
        assertEquals("家", initialOf("家計簿"))
        assertEquals("🏃", initialOf("🏃 散歩"))
        assertEquals("が", initialOf("がいもの")) // 結合の濁点を切らない
        assertEquals("", initialOf("  "))
    }
}
