package jp.kz2601.metamon.desktop.os

import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class GnomeSearchProviderTest {
    private val apps = listOf("shopping" to "買い物リスト", "diary" to "Diary", "daily-budget" to "Daily Budget")
    private val opened = mutableListOf<String>()

    private fun provider(on: Boolean = true) = GnomeSearchProvider({ apps }, { opened += it }, { on })

    @Test
    fun offByDefaultReturnsNothing() {
        val p = GnomeSearchProvider({ apps }, { opened += it })
        assertEquals(emptyList<String>(), p.getInitialResultSet(listOf("diary")))
        assertEquals(emptyList<GnomeSearchProvider.ResultMeta>(), p.getResultMetas(listOf("diary")))
        p.activateResult("diary")
        assertTrue(opened.isEmpty())
    }

    @Test
    fun matchesUseTheSameRulesAsThePalette() {
        val p = provider()
        // 完全一致 > 頭から一致、全角・大小は同じ、カタカナ / ひらがなも同じ
        assertEquals(listOf("diary"), p.getInitialResultSet(listOf("ＤＩＡＲＹ")))
        // 同点は元の順
        assertEquals(listOf("diary", "daily-budget"), p.getInitialResultSet(listOf("d")))
        assertEquals(listOf("shopping"), p.getInitialResultSet(listOf("買い")))
    }

    @Test
    fun multipleTermsAreJoinedLikeThePalette() {
        val p = provider()
        assertEquals(listOf("daily-budget"), p.getInitialResultSet(listOf("daily", "bud")))
    }

    @Test
    fun emptyOrNoMatchReturnsNothing() {
        val p = provider()
        assertEquals(emptyList<String>(), p.getInitialResultSet(emptyList()))
        assertEquals(emptyList<String>(), p.getInitialResultSet(listOf("  ")))
        assertEquals(emptyList<String>(), p.getInitialResultSet(listOf("zzz")))
    }

    @Test
    fun subsearchOnlyNarrowsThePreviousResults() {
        val p = provider()
        assertEquals(listOf("daily-budget"), p.getSubsearchResultSet(listOf("daily-budget"), listOf("d")))
        assertEquals(emptyList<String>(), p.getSubsearchResultSet(listOf("shopping"), listOf("diary")))
    }

    @Test
    fun metasHaveOnlyTheNameAndSkipUnknownIds() {
        val metas = provider().getResultMetas(listOf("diary", "gone", "shopping"))
        assertEquals(listOf(GnomeSearchProvider.ResultMeta("diary", "Diary"), GnomeSearchProvider.ResultMeta("shopping", "買い物リスト")), metas)
    }

    @Test
    fun activateOpensOnlyKnownApps() {
        val p = provider()
        p.activateResult("diary")
        p.activateResult("../etc")
        assertEquals(listOf("diary"), opened)
    }

    @Test
    fun launchSearchOnlyWhenEnabled() {
        var n = 0
        provider(false).launchSearch { n++ }
        provider(true).launchSearch { n++ }
        assertEquals(1, n)
    }

    @Test
    fun iniFileMatchesTheConstants() {
        val ini = javaClass.classLoader.getResource("linux/tailor-it-search-provider.ini")!!.readText().lines().filter { it.contains('=') }.associate { it.substringBefore('=') to it.substringAfter('=') }
        assertEquals(GnomeSearchProvider.BUS_NAME, ini["BusName"])
        assertEquals(GnomeSearchProvider.OBJECT_PATH, ini["ObjectPath"])
        assertEquals("2", ini["Version"])
    }
}
