package jp.kz2601.metamon.services.live

import jp.kz2601.metamon.dsl.feed.FeedQuery
import jp.kz2601.metamon.services.http.ApiError
import jp.kz2601.metamon.services.http.errorBody
import jp.kz2601.metamon.services.http.json
import jp.kz2601.metamon.services.http.metamonHttp
import jp.kz2601.metamon.services.http.take
import kotlinx.coroutines.runBlocking
import okhttp3.mockwebserver.MockWebServer
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Assert.fail
import org.junit.Before
import org.junit.Test

class HttpFeedClientTest {
    private lateinit var server: MockWebServer
    private lateinit var client: HttpFeedClient

    @Before
    fun setUp() {
        server = MockWebServer().apply { start() }
        client = HttpFeedClient(server.metamonHttp())
    }

    @After
    fun tearDown() {
        server.shutdown()
    }

    @Test
    fun postsTheParametersInTheBodyNotTheUrl() = runBlocking {
        server.enqueue(
            json(
                200,
                """{"provider":"diet","title":"国会会議録","items":[{"date":"2026-11-05","title":"山田太郎(自民)","subtitle":"衆議院 予算委員会 第3号","url":"https://kokkai.ndl.go.jp/txt/1"}],"source":{"name":"国会会議録検索システム","url":"https://kokkai.ndl.go.jp/","license":"利用規約"},"fetchedAt":"2026-11-12T03:00:00Z","stale":true}""",
            ),
        )
        val r = client.feed(FeedQuery("diet", q = "予算 委員会", limit = 5))
        val req = server.take()
        assertEquals("POST", req.method)
        assertEquals("/v1/feeds/diet", req.path)
        assertEquals("""{"q":"予算 委員会","limit":5}""", req.body.readUtf8())
        assertEquals("https://kokkai.ndl.go.jp/txt/1", r.items.single().link)
        assertTrue(r.stale)
    }

    @Test
    fun disabledProviderIsAnApiError() = runBlocking {
        server.enqueue(json(503, errorBody("upstream_unavailable", details = """{"reason":"disabled"}""")))
        try {
            client.feed(FeedQuery("weather", region = "170000"))
            fail("throws")
        } catch (e: ApiError) {
            assertEquals(503, e.status)
            assertEquals("upstream_unavailable", e.code)
            assertEquals("disabled", e.reason)
        }
    }
}
