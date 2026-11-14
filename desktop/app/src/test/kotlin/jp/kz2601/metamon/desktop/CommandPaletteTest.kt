package jp.kz2601.metamon.desktop

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/** パレットの語の区切りと飛び飛びの一致(#2615)。 */
class CommandPaletteTest {
    private fun cmd(title: String, keywords: List<String> = emptyList()) = PaletteCommand(title, title, PaletteCommand.Kind.ACTION, keywords) {}

    private fun titles(cs: List<PaletteCommand>, q: String) = CommandRanking.rank(cs, q).map { it.title }

    // MARK: words

    @Test
    fun wordsSplitOnSeparators() {
        val cases = listOf(
            "買い物 リスト" to listOf("買い物", "リスト"),
            "a\tb\nc" to listOf("a", "b", "c"),
            "設定/表示" to listOf("設定", "表示"),
            "家計・支出" to listOf("家計", "支出"),
            "snake_case" to listOf("snake", "case"),
            "pre-fix" to listOf("pre", "fix"),
            "保存(Ctrl)" to listOf("保存", "Ctrl"),
            "保存（全角）" to listOf("保存", "全角"),
            // 続いた区切り・前後の区切りは空の語を作らない
            " / 買い物・・リスト_- " to listOf("買い物", "リスト"),
            "買い物リスト" to listOf("買い物リスト"),
            "" to emptyList(),
            "・/()" to emptyList(),
        )
        for ((s, want) in cases) assertEquals(s, want, CommandRanking.words(s))
    }

    @Test
    fun wordHeadMatchUsesEverySeparator() {
        // 語の頭から合う = 600 点。区切りの種類ごと
        for (title in listOf("今月 支出", "今月/支出", "今月・支出", "今月_支出", "今月-支出", "今月(支出)", "今月（支出）")) {
            assertEquals(title, 600, CommandRanking.score(cmd(title), "支出"))
        }
        // 区切りが無ければ途中に含む(前ほど上)
        assertEquals(400 - 2, CommandRanking.score(cmd("今月支出"), "支出"))
    }

    // MARK: subsequenceGap

    @Test
    fun subsequenceGapCountsSkippedChars() {
        assertEquals(0, CommandRanking.subsequenceGap("abc", "abc"))
        // 最初の文字より前は数えない
        assertEquals(0, CommandRanking.subsequenceGap("xxabc", "abc"))
        assertEquals(1, CommandRanking.subsequenceGap("abc", "ac"))
        assertEquals(2, CommandRanking.subsequenceGap("axbxc", "abc"))
        assertEquals(0, CommandRanking.subsequenceGap("abc", ""))
        // 順に現れない・無い文字は null
        assertNull(CommandRanking.subsequenceGap("abc", "ca"))
        assertNull(CommandRanking.subsequenceGap("abc", "abcd"))
        assertNull(CommandRanking.subsequenceGap("", "a"))
        assertNull(CommandRanking.subsequenceGap("abc", "aa"))
    }

    @Test
    fun closerSubsequenceScoresHigher() {
        val near = CommandRanking.score(cmd("買い物のリスト"), "買リ")!!
        val far = CommandRanking.score(cmd("買い物をする時のリスト"), "買リ")!!
        assertEquals(100 - 3, near)
        assertTrue("連続に近い方が上", near > far)
        // 飛び飛びの一致は途中に含むより下・離れすぎても 1 点は残る
        assertTrue(near < CommandRanking.score(cmd("リスト買リ"), "買リ")!!)
        assertEquals(1, CommandRanking.score(cmd("a" + "x".repeat(200) + "b"), "ab"))
        assertNull(CommandRanking.score(cmd("リスト"), "買リ"))
        assertEquals(listOf("買い物のリスト", "買い物をする時のリスト"), titles(listOf(cmd("買い物をする時のリスト"), cmd("買い物のリスト"), cmd("設定")), "買リ"))
    }

    // MARK: 並び順の表

    @Test
    fun rankingTable() {
        val cs = listOf(cmd("今月の家計"), cmd("家の計画"), cmd("レシート家計簿"), cmd("家計簿"), cmd("設定", keywords = listOf("家計の設定")), cmd("印刷"))
        val cases = listOf(
            // 頭から > 途中に含む(前ほど上)> 別名 > 飛び飛び。合わない物は出ない
            "家計" to listOf("家計簿", "今月の家計", "レシート家計簿", "設定", "家の計画"),
            "家計簿" to listOf("家計簿", "レシート家計簿"),
            "計" to listOf("家計簿", "家の計画", "今月の家計", "レシート家計簿", "設定"),
            "いんさつ" to emptyList(),
            "印" to listOf("印刷"),
        )
        for ((q, want) in cases) assertEquals(q, want, titles(cs, q))
    }

    @Test
    fun bundledAppsPutKakeiboFirst() {
        val apps = BundledApps.all().map { (id, name) -> PaletteCommand("app:$id", name, PaletteCommand.Kind.APP) {} }
        val ranked = CommandRanking.rank(apps, "家計")
        assertEquals("家計簿", ranked.first().title)
        assertTrue(ranked.any { it.title == "レシート家計簿" })
    }
}
