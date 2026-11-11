package jp.kz2601.metamon.renderer.components.feed

import jp.kz2601.metamon.dsl.feed.FeedItem
import jp.kz2601.metamon.dsl.feed.FeedQuery
import jp.kz2601.metamon.dsl.feed.FeedResult
import jp.kz2601.metamon.dsl.feed.FeedSource
import jp.kz2601.metamon.renderer.NoopDSLRuntime
import kotlinx.coroutines.runBlocking
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.IOException

class FeedLeafTest {
    private val source = FeedSource("気象庁ホームページ", "https://www.jma.go.jp/", "公共データ利用規約(第1.0版)")
    private val query = FeedQuery("weather", region = "170000")

    @Test
    fun icons() {
        assertEquals("cloud.sun", FeedFormat.icon("weather"))
        assertEquals("gauge.with.dots.needle.33percent", FeedFormat.icon("pressure"))
        assertEquals("newspaper", FeedFormat.icon("rss"))
        assertEquals("newspaper", FeedFormat.icon(null))
    }

    @Test
    fun tempsShowEitherSide() {
        assertEquals("18° / 9°", FeedFormat.temps(FeedItem(title = "", max = 18.4, min = 8.6)))
        assertEquals("18°", FeedFormat.temps(FeedItem(title = "", max = 18.4)))
        assertEquals("-2°", FeedFormat.temps(FeedItem(title = "", min = -2.2)))
        assertNull(FeedFormat.temps(FeedItem(title = "")))
    }

    @Test
    fun pressureRange() {
        assertEquals("1006〜1013 hPa", FeedFormat.pressureRange(FeedItem(title = "", min = 1006.2, max = 1012.7)))
        assertEquals("1013 hPa", FeedFormat.pressureRange(FeedItem(title = "", min = 1012.6, max = 1013.4)))
        assertEquals("1010 hPa", FeedFormat.pressureRange(FeedItem(title = "", max = 1010.0)))
        assertNull(FeedFormat.pressureRange(FeedItem(title = "")))
    }

    @Test
    fun dateTextByProvider() {
        assertEquals("11/7(土)", FeedFormat.dateText("2026-11-07", "weather"))
        assertEquals("11/7(土)", FeedFormat.dateText("2026-11-07T00:00:00+09:00", "holidays"))
        // RFC 3339 は日本時間で
        assertEquals("11/5 18:30", FeedFormat.dateText("2026-11-05T09:30:00Z", "rss"))
        assertEquals("11/5 18:30", FeedFormat.dateText("2026-11-05T18:30:00+09:00", "warnings"))
        assertNull(FeedFormat.dateText("not a date", "arxiv"))
        assertNull(FeedFormat.dateText("", "weather"))
        assertNull(FeedFormat.dateText(null, "weather"))
    }

    @Test
    fun sparklineFillsTheHeight() {
        assertEquals(listOf(0f to 1f, 0.5f to 0f, 1f to 0.5f), FeedFormat.sparkline(listOf(1000.0, 1010.0, 1005.0)))
        assertEquals(listOf(0f to 0.5f, 1f to 0.5f), FeedFormat.sparkline(listOf(1000.0, 1000.0)))
        assertTrue(FeedFormat.sparkline(listOf(1000.0)).isEmpty())
    }

    @Test
    fun phases() {
        val shown = FeedResult("weather", "t", listOf(FeedItem(title = "晴れ")), source)
        assertEquals(FeedViewState.Phase.LOADING, FeedViewState().phase)
        assertEquals(FeedViewState.Phase.SHOWN, FeedViewState(shown).phase)
        assertEquals(FeedViewState.Phase.SHOWN, FeedViewState(shown, failed = true).phase)
        assertEquals(FeedViewState.Phase.EMPTY, FeedViewState(shown.copy(items = emptyList())).phase)
        assertEquals(FeedViewState.Phase.UNAVAILABLE, FeedViewState(failed = true).phase)
    }

    private class FakeHost(var next: FeedResult?, val cached: FeedResult? = null) : FeedHost {
        val forces = mutableListOf<Boolean>()
        override suspend fun feed(query: FeedQuery, force: Boolean): FeedResult {
            forces += force
            return next ?: throw IOException("offline")
        }

        override suspend fun cachedFeed(query: FeedQuery): FeedResult? = cached
    }

    @Test
    fun loadingKeepsShownRowsOnFailure() = runBlocking {
        val a = FeedResult("weather", "t", listOf(FeedItem(title = "a")), source)
        val host = FakeHost(a, cached = a.copy(title = "old"))
        assertEquals("old", FeedLoading.initial(host, query).result?.title)

        val loaded = FeedLoading.load(host, query, force = false, current = FeedViewState())
        assertEquals(FeedViewState(a), loaded)
        host.next = null
        assertEquals(loaded, FeedLoading.load(host, query, force = true, current = loaded))
        assertEquals(listOf(false, true), host.forces)
        assertEquals(FeedViewState(failed = true), FeedLoading.load(host, query, force = false, current = FeedViewState()))
    }

    @Test
    fun withoutHostIsUnavailable() = runBlocking {
        assertEquals(FeedViewState(failed = true), FeedLoading.load(null, query, force = false, current = FeedViewState()))
        assertNull(FeedLoading.initial(null, query).result)
        assertNull(NoopDSLRuntime.feeds)
    }
}
