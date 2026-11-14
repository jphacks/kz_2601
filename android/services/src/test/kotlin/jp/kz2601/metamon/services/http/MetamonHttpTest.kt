package jp.kz2601.metamon.services.http

import kotlinx.coroutines.runBlocking
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import okhttp3.mockwebserver.SocketPolicy
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Assert.fail
import org.junit.Before
import org.junit.Test
import java.io.IOException
import java.time.Instant

class MetamonHttpTest {
    private lateinit var server: MockWebServer

    @Before
    fun setUp() {
        server = MockWebServer().apply { start() }
    }

    @After
    fun tearDown() {
        server.shutdown()
    }

    private fun get(path: String = "v1/apps") = HttpRequest("GET", path.split("/"))
    private fun post(path: String = "v1/generate") = HttpRequest("POST", path.split("/"), body = "{}")

    @Test
    fun sendsClientHeadersOnEveryRequest() = runBlocking<Unit> {
        server.enqueue(json(200, "{}"))
        server.metamonHttp(appCheck = FakeAppCheck("ac-1")).send(get())

        val r = server.take()
        assertEquals("/v1/apps", r.path)
        assertEquals("Bearer token-1", r.getHeader("Authorization"))
        assertEquals("ac-1", r.getHeader("X-Firebase-AppCheck"))
        assertEquals("android", r.getHeader("X-Client-Platform"))
        assertEquals("0.1.0", r.getHeader("X-Client-Version"))
        assertEquals("7", r.getHeader("X-Client-Build"))
        assertEquals("1", r.getHeader("X-Client-DSL-Version"))
        assertEquals("Asia/Tokyo", r.getHeader("X-Time-Zone"))
        assertEquals("ja", r.getHeader("Accept-Language"))
    }

    @Test
    fun omitsAuthHeadersWhenNoTokens() = runBlocking<Unit> {
        server.enqueue(json(200, "{}"))
        server.metamonHttp(tokens = FakeTokens(null), appCheck = FakeAppCheck(null)).send(get())

        val r = server.take()
        assertNull(r.getHeader("Authorization"))
        assertNull(r.getHeader("X-Firebase-AppCheck"))
        assertEquals("android", r.getHeader("X-Client-Platform"))
    }

    @Test
    fun pathSegmentsAreEscaped() = runBlocking<Unit> {
        server.enqueue(json(200, "{}"))
        server.metamonHttp().send(HttpRequest("GET", listOf("v1", "apps", "a/b c")))
        assertEquals("/v1/apps/a%2Fb%20c", server.take().path)
    }

    @Test
    fun postWithoutBodySendsEmptyBody() = runBlocking<Unit> {
        server.enqueue(json(201, "{}"))
        server.metamonHttp().send(HttpRequest("POST", listOf("v1", "apps", "x", "share")))
        val r = server.take()
        assertEquals("POST", r.method)
        assertEquals(0L, r.bodySize)
    }

    @Test
    fun expiredTokenIsRefreshedAndResentOnce() = runBlocking<Unit> {
        server.enqueue(json(401, errorBody("unauthenticated")))
        server.enqueue(json(200, """{"ok":true}"""))
        val tokens = FakeTokens("old", "new")

        val body = server.metamonHttp(tokens = tokens).send(post())

        assertEquals("""{"ok":true}""", body)
        assertEquals("Bearer old", server.take().getHeader("Authorization"))
        assertEquals("Bearer new", server.take().getHeader("Authorization"))
        assertEquals(listOf(false, true), tokens.calls)
    }

    @Test
    fun sameTokenAfterRefreshIsNotResent() = runBlocking<Unit> {
        server.enqueue(json(401, errorBody("unauthenticated")))
        val e = expectApiError { server.metamonHttp(tokens = FakeTokens("only")).send(post()) }
        assertEquals(401, e.status)
        assertEquals(1, server.requestCount)
        assertEquals(FailureKind.SessionExpired, FailureKind.classify(e))
    }

    @Test
    fun appCheckRejectionRefreshesAppCheckNotIdToken() = runBlocking<Unit> {
        server.enqueue(json(401, errorBody("app_check_failed", details = """{"reason":"invalid"}""")))
        server.enqueue(json(200, "{}"))
        val tokens = FakeTokens("t1", "t2")
        val appCheck = FakeAppCheck("ac-old", "ac-new")

        server.metamonHttp(tokens = tokens, appCheck = appCheck).send(post())

        assertEquals("ac-old", server.take().getHeader("X-Firebase-AppCheck"))
        val second = server.take()
        assertEquals("ac-new", second.getHeader("X-Firebase-AppCheck"))
        assertEquals("Bearer t1", second.getHeader("Authorization"))
        assertEquals(listOf(false), tokens.calls)
    }

    @Test
    fun appCheckRejectionStillFailingIsClassified() = runBlocking<Unit> {
        server.enqueue(json(401, errorBody("app_check_failed")))
        server.enqueue(json(401, errorBody("app_check_failed")))
        val e = expectApiError { server.metamonHttp(appCheck = FakeAppCheck("a", "b")).send(post()) }
        assertEquals(FailureKind.AppCheckFailed, FailureKind.classify(e))
        assertEquals(2, server.requestCount)
    }

    @Test
    fun rateLimitedWaitsRetryAfterAndResendsOnce() = runBlocking<Unit> {
        server.enqueue(json(429, errorBody("rate_limited"), "Retry-After" to "3"))
        server.enqueue(json(200, "{}"))
        val sleeps = mutableListOf<Long>()

        server.metamonHttp(sleeps = sleeps).send(post())

        assertEquals(listOf(3000L), sleeps)
        assertEquals(2, server.requestCount)
    }

    @Test
    fun rateLimitedWithLongRetryAfterFailsWithoutWaiting() = runBlocking<Unit> {
        server.enqueue(json(429, errorBody("rate_limited", details = """{"scope":"user","tier":"signed_in","limit":20,"remaining":0}"""), "Retry-After" to "3600"))
        val sleeps = mutableListOf<Long>()

        val e = expectApiError { server.metamonHttp(sleeps = sleeps).send(post()) }

        assertTrue(sleeps.isEmpty())
        assertEquals(3600.0, e.retryAfterSeconds)
        val kind = FailureKind.classify(e) as FailureKind.RateLimited
        assertEquals(RateLimitInfo.Scope.USER, kind.info.scope)
        assertEquals(20, kind.info.limit)
        assertFalse(kind.isRetryable)
    }

    @Test
    fun idempotentGetRetriesTransient5xxWithBackoff() = runBlocking<Unit> {
        server.enqueue(json(503, errorBody("upstream_unavailable")))
        server.enqueue(json(502, "bad gateway"))
        server.enqueue(json(200, "{}"))
        val sleeps = mutableListOf<Long>()

        server.metamonHttp(sleeps = sleeps).send(get())

        assertEquals(3, server.requestCount)
        assertEquals(listOf(500L, 1000L), sleeps)
    }

    @Test
    fun idempotentGetRetriesDroppedConnection() = runBlocking<Unit> {
        server.enqueue(MockResponse().setSocketPolicy(SocketPolicy.DISCONNECT_AT_START))
        server.enqueue(json(200, "{}"))
        val sleeps = mutableListOf<Long>()

        server.metamonHttp(sleeps = sleeps).send(get())

        assertEquals(listOf(500L), sleeps)
    }

    @Test
    fun idempotentGetGivesUpAfterMaxAttempts() = runBlocking<Unit> {
        repeat(3) { server.enqueue(json(503, errorBody("upstream_unavailable"))) }
        val e = expectApiError { server.metamonHttp().send(get()) }
        assertEquals(503, e.status)
        assertEquals(3, server.requestCount)
    }

    @Test
    fun postIsNeverRetriedOn5xx() = runBlocking<Unit> {
        server.enqueue(json(503, errorBody("upstream_unavailable")))
        server.enqueue(json(200, "{}"))
        val e = expectApiError { server.metamonHttp().send(post()) }
        assertEquals(503, e.status)
        assertEquals(1, server.requestCount)
    }

    @Test
    fun postConnectionDropIsThrownAsIOException() = runBlocking<Unit> {
        server.enqueue(MockResponse().setSocketPolicy(SocketPolicy.DISCONNECT_AT_START))
        try {
            server.metamonHttp().send(post())
            fail("expected IOException")
        } catch (e: IOException) {
            assertEquals(FailureKind.Offline, FailureKind.classify(e))
        }
    }

    @Test
    fun aiPausedIsNotRetriedEvenForGet() = runBlocking<Unit> {
        server.enqueue(json(503, errorBody("ai_paused")))
        val e = expectApiError { server.metamonHttp().send(get()) }
        assertEquals(1, server.requestCount)
        assertEquals(FailureKind.AIPaused, FailureKind.classify(e))
    }

    @Test
    fun nonEnvelopeErrorBecomesHttpError() = runBlocking<Unit> {
        server.enqueue(MockResponse().setResponseCode(404).setBody("<html>nope</html>"))
        val e = expectApiError { server.metamonHttp().send(post()) }
        assertTrue(e is ApiError.Http)
        assertNull(e.code)
    }

    @Test
    fun retryAfterAcceptsSecondsAndHttpDate() {
        val now = Instant.parse("2026-11-11T00:00:00Z")
        assertEquals(120.0, MetamonHttp.parseRetryAfter("120", now))
        assertEquals(30.0, MetamonHttp.parseRetryAfter("Wed, 11 Nov 2026 00:00:30 GMT", now))
        assertEquals(0.0, MetamonHttp.parseRetryAfter("Tue, 10 Nov 2026 00:00:00 GMT", now))
        assertNull(MetamonHttp.parseRetryAfter("soon", now))
        assertNull(MetamonHttp.parseRetryAfter(null, now))
    }
}

inline fun expectApiError(block: () -> Unit): ApiError {
    try {
        block()
    } catch (e: ApiError) {
        return e
    }
    throw AssertionError("expected ApiError")
}
