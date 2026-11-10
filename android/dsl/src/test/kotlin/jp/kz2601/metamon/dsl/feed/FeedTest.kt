package jp.kz2601.metamon.dsl.feed

import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.ComponentType
import jp.kz2601.metamon.dsl.Provider
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

class FeedTest {
    private fun feed(provider: Provider?, region: String? = null, feedId: String? = null, query: String? = null, limit: Int? = null) =
        Component(type = ComponentType.FEED, provider = provider, region = region, feedId = feedId, query = query, limit = limit)

    @Test
    fun queryKeepsOnlyTheParametersOfItsProvider() {
        assertEquals(
            FeedQuery("weather", region = "170000", limit = 3),
            FeedQuery.of(feed(Provider.WEATHER, region = "170000", feedId = "digital-news", query = "x", limit = 3)),
        )
        assertEquals(FeedQuery("rss", feedId = "digital-news"), FeedQuery.of(feed(Provider.RSS, region = "170000", feedId = "digital-news")))
        assertEquals(FeedQuery("diet", q = "予算 委員会"), FeedQuery.of(feed(Provider.DIET, query = "予算 委員会")))
        assertEquals(FeedQuery("holidays"), FeedQuery.of(feed(Provider.HOLIDAYS, region = "170000")))
    }

    @Test
    fun unknownProviderOrEmptyDietQueryIsNotDrawn() {
        assertNull(FeedQuery.of(feed(null)))
        assertNull(FeedQuery.of(feed(Provider.UNKNOWN)))
        assertNull(FeedQuery.of(feed(Provider.DIET, query = "  ")))
    }

    @Test
    fun limitIsClampedTo1Through20() {
        assertEquals(1, FeedQuery.of(feed(Provider.HOLIDAYS, limit = 0))?.limit)
        assertEquals(20, FeedQuery.of(feed(Provider.HOLIDAYS, limit = 99))?.limit)
    }

    @Test
    fun bodyOmitsMissingKeysAndPathCarriesTheProvider() {
        val q = FeedQuery("diet", q = "予算", limit = 5)
        assertEquals("""{"q":"予算","limit":5}""", q.body())
        assertEquals(listOf("v1", "feeds", "diet"), q.path)
        assertEquals("""{"region":"130000","limit":3}""", FeedQuery("weather", region = "130000", limit = 3).body())
    }

    @Test
    fun decodeDropsBrokenRowsAndIgnoresUnknownKeys() {
        val r = FeedResult.decode(
            """
            {"provider":"weather","title":"石川県の天気","fetchedAt":"2026-11-12T03:00:00Z","new":1,
             "items":[{"date":"2026-11-12","title":"晴れ","icon":"sun.max","max":18.4,"min":9,"pop":10},{"date":"2026-11-13"},"x"],
             "source":{"name":"気象庁ホームページ","url":"https://www.jma.go.jp/","license":"公共データ利用規約(第1.0版)"}}
            """.trimIndent(),
        )
        assertEquals("石川県の天気", r.title)
        assertEquals(1, r.items.size)
        assertEquals(18.4, r.items[0].max)
        assertEquals(10, r.items[0].pop)
        assertEquals(false, r.stale)
        assertEquals("気象庁ホームページ", r.source.name)
    }

    @Test
    fun encodeRoundTrips() {
        val r = FeedResult("pressure", "気圧", listOf(FeedItem(title = "a", drop = true, series = listOf(1010.0, 1008.5))), FeedSource("n", "https://e", "CC BY 4.0"), stale = true)
        assertEquals(r, FeedResult.decode(FeedResult.encode(r)))
    }

    @Test
    fun linkIsOnlyHttpsWithAHost() {
        assertEquals("https://www.digital.go.jp/news/1", FeedItem(title = "a", url = "https://www.digital.go.jp/news/1").link)
        assertNull(FeedItem(title = "a", url = "http://example.com").link)
        assertNull(FeedItem(title = "a", url = "javascript:alert(1)").link)
        assertNull(FeedItem(title = "a", url = "https:///nohost").link)
        assertNull(FeedItem(title = "a").link)
        assertTrue(FeedItem(date = "d", title = "t", url = "u").id.contains("|t|"))
    }
}
