package jp.kz2601.metamon.services.http

import jp.kz2601.metamon.services.RemainingToday
import jp.kz2601.metamon.services.RemainingTodayStore
import kotlinx.coroutines.runBlocking
import okhttp3.Headers.Companion.toHeaders
import okhttp3.mockwebserver.MockWebServer
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Before
import org.junit.Test
import java.time.Instant

/** #1203: ai の応答の `X-RateLimit-*` から「今日の残り」を読む(iOS の `RemainingTodayTests`)。 */
class RemainingTodayHeadersTest {
    private val now = Instant.parse("2026-11-14T03:00:00Z") // 11/14 12:00 JST
    private val reset = Instant.parse("2026-11-14T15:00:00Z") // 11/15 0:00 JST

    private lateinit var server: MockWebServer

    @Before
    fun setUp() {
        server = MockWebServer().apply { start() }
    }

    @After
    fun tearDown() {
        server.shutdown()
    }

    private fun parse(headers: Map<String, String>, status: Int = 200, body: String = "{}"): RemainingToday? {
        val h = headers.toHeaders()
        return RemainingToday.parse({ h[it] }, status, body, now)
    }

    @Test
    fun readsAllHeadersCaseInsensitively() {
        // Go はヘッダー名を X-Ratelimit-Remaining に正規化して返す
        val v = parse(mapOf("X-Ratelimit-Remaining" to "24", "x-ratelimit-limit" to "60", "X-RateLimit-Reset" to "${reset.epochSecond}"))
        assertEquals(RemainingToday(24, 60, reset, now), v)
    }

    @Test
    fun missingRemainingIsNull() {
        assertNull("appconfig など、ヘッダーの無い応答では何もしない", parse(emptyMap()))
        assertNull(parse(mapOf("X-RateLimit-Limit" to "60")))
    }

    @Test
    fun brokenValuesAreIgnored() {
        assertNull(parse(mapOf("X-RateLimit-Remaining" to "abc")))
        assertNull(parse(mapOf("X-RateLimit-Remaining" to "-1")))
        assertNull(parse(mapOf("X-RateLimit-Remaining" to "2.5")))
        assertNull("上限より多い残りは壊れている", parse(mapOf("X-RateLimit-Remaining" to "61", "X-RateLimit-Limit" to "60")))
        val v = parse(mapOf("X-RateLimit-Remaining" to " 5 ", "X-RateLimit-Limit" to "x", "X-RateLimit-Reset" to "soon"))
        assertEquals(5, v?.remaining)
        assertNull("壊れた上限は捨てて残りだけ使う", v?.limit)
        assertNull(v?.resetAt)
    }

    @Test
    fun rateLimitedOnlyForUserScope() {
        val user = errorBody("rate_limited", details = """{"scope":"user","remaining":0}""")
        val global = errorBody("rate_limited", details = """{"scope":"global","remaining":3}""")
        assertEquals(0, parse(mapOf("X-RateLimit-Remaining" to "0", "X-RateLimit-Limit" to "60"), 429, user)?.remaining)
        assertNull("全体の上限の数は本人の残りではない", parse(mapOf("X-RateLimit-Remaining" to "3", "X-RateLimit-Limit" to "2000"), 429, global))
        assertNull(parse(mapOf("X-RateLimit-Remaining" to "0"), 429, "busy"))
    }

    @Test
    fun storeKeepsNewestAndClears() {
        val store = RemainingTodayStore()
        store.record(RemainingToday(20, 60, reset, observedAt = now))
        store.record(RemainingToday(30, 60, reset, observedAt = now.minusSeconds(5)))
        assertEquals("後から届いた前の依頼の分では上書きしない", 20, store.value.value?.remaining)
        assertNotNull(store.current(now))
        assertNull("戻る時刻を過ぎたら出さない", store.current(reset))
        store.clear()
        assertNull(store.value.value)
    }

    @Test
    fun responsesFeedTheSink() = runBlocking {
        val seen = mutableListOf<Int>()
        val http = server.metamonHttp(remainingSink = { seen += it.remaining })
        server.enqueue(json(200, "{}", "X-RateLimit-Remaining" to "57", "X-RateLimit-Limit" to "60"))
        server.enqueue(json(200, "{}"))
        http.send(HttpRequest("POST", listOf("v1", "chat")))
        http.send(HttpRequest("GET", listOf("v1", "apps")))
        assertEquals("ヘッダーの無い応答は数えない", listOf(57), seen)
    }

    @Test
    fun streamHeadersFeedTheSink() = runBlocking {
        val seen = mutableListOf<Int>()
        val http = server.metamonHttp(remainingSink = { seen += it.remaining })
        server.enqueue(json(200, "event: done\ndata: {}\n\n", "X-RateLimit-Remaining" to "12"))
        http.openStream(HttpRequest("POST", listOf("v1", "generate"))).close()
        assertEquals(listOf(12), seen)
    }
}
