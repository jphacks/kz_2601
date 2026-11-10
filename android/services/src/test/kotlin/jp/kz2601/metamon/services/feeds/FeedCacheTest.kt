package jp.kz2601.metamon.services.feeds

import jp.kz2601.metamon.dsl.feed.FeedItem
import jp.kz2601.metamon.dsl.feed.FeedQuery
import jp.kz2601.metamon.dsl.feed.FeedResult
import jp.kz2601.metamon.dsl.feed.FeedSource
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.runBlocking
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Assert.fail
import org.junit.Rule
import org.junit.Test
import org.junit.rules.TemporaryFolder
import java.io.IOException

class FeedCacheTest {
    @get:Rule
    val tmp = TemporaryFolder()

    private val query = FeedQuery("diet", q = "予算 委員会", limit = 5)
    private var clock = 1_000_000L

    private fun result(title: String) = FeedResult("diet", "国会会議録", listOf(FeedItem(title = title)), FeedSource("国会会議録検索システム", "https://kokkai.ndl.go.jp/", "利用規約"))

    /** 呼ばれた回数を数え、[next] を返す(null なら圏外として投げる)。 */
    private class FakeClient(var next: FeedResult?) : FeedClient {
        var calls = 0
        override suspend fun feed(query: FeedQuery): FeedResult {
            calls++
            return next ?: throw IOException("offline")
        }
    }

    private fun cache(dir: java.io.File? = tmp.root) = FeedCache(dir, now = { clock }, io = Dispatchers.Unconfined)

    @Test
    fun sameQueryWithinTenMinutesDoesNotFetchAgain() = runBlocking {
        val c = cache()
        val client = FakeClient(result("a"))
        assertEquals("a", c.load(query, force = false, client = client).items[0].title)
        client.next = result("b")
        clock += FeedCache.TTL_MILLIS - 1
        assertEquals("a", c.load(query, force = false, client = client).items[0].title)
        assertEquals(1, client.calls)
        clock += 1
        assertEquals("b", c.load(query, force = false, client = client).items[0].title)
        assertEquals(2, client.calls)
    }

    @Test
    fun forceFetchesAgain() = runBlocking {
        val c = cache()
        val client = FakeClient(result("a"))
        c.load(query, force = false, client = client)
        client.next = result("b")
        assertEquals("b", c.load(query, force = true, client = client).items[0].title)
        assertEquals(2, client.calls)
    }

    @Test
    fun failureReturnsTheLastRowsMarkedStaleWithin24Hours() = runBlocking {
        val c = cache()
        val client = FakeClient(result("a"))
        c.load(query, force = false, client = client)
        client.next = null
        clock += FeedCache.TTL_MILLIS
        val stale = c.load(query, force = false, client = client)
        assertTrue(stale.stale)
        assertEquals("a", stale.items[0].title)
        clock += FeedCache.STALE_LIMIT_MILLIS
        try {
            c.load(query, force = false, client = client)
            fail("24 時間を超えた行は出さない")
        } catch (_: IOException) {
        }
    }

    @Test
    fun failureWithoutRowsThrows() = runBlocking {
        try {
            cache().load(query, force = false, client = FakeClient(null))
            fail("throws")
        } catch (_: IOException) {
        }
    }

    @Test
    fun rowsSurviveARestartOnDiskAndFileNameHidesTheQuery() = runBlocking {
        cache().load(query, force = false, client = FakeClient(result("a")))
        val names = tmp.root.list().orEmpty().toList()
        assertEquals(1, names.size)
        assertFalse(names[0].contains("予算"))

        val restarted = cache()
        assertEquals("a", restarted.cached(query)?.items?.get(0)?.title)
        val client = FakeClient(result("b"))
        assertEquals("a", restarted.load(query, force = false, client = client).items[0].title)
        assertEquals(0, client.calls)
        assertNull(restarted.cached(FeedQuery("holidays")))
    }

    @Test
    fun brokenFileIsDroppedAndRemoveAllClears() = runBlocking {
        cache().load(query, force = false, client = FakeClient(result("a")))
        tmp.root.listFiles()!!.single().writeText("{broken")
        assertNull(cache().cached(query))
        assertTrue(tmp.root.list().orEmpty().isEmpty())

        val c = cache()
        c.load(query, force = false, client = FakeClient(result("a")))
        c.removeAll()
        assertNull(c.cached(query))
        assertFalse(tmp.root.exists() && tmp.root.list().orEmpty().isNotEmpty())
    }

    @Test
    fun memoryOnlyWithoutDirectory() = runBlocking {
        val c = cache(null)
        c.load(query, force = false, client = FakeClient(result("a")))
        assertEquals("a", c.cached(query)?.items?.get(0)?.title)
    }
}
