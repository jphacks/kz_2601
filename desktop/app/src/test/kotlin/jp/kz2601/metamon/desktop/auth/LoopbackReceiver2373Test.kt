package jp.kz2601.metamon.desktop.auth

import kotlinx.coroutines.async
import kotlinx.coroutines.runBlocking
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertThrows
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.IOException
import java.net.ConnectException
import java.net.InetSocketAddress
import java.net.Socket
import java.net.URI
import java.net.http.HttpClient
import java.net.http.HttpRequest
import java.net.http.HttpResponse
import java.time.Duration
import kotlin.time.Duration.Companion.milliseconds
import kotlin.time.Duration.Companion.seconds

/** 127.0.0.1 の受け口(#2373)。ブラウザの代わりに HttpClient から投げる。 */
class LoopbackReceiver2373Test {
    private val http = HttpClient.newBuilder().connectTimeout(Duration.ofSeconds(2)).build()
    private val state = "s-" + GooglePkce.newState()
    private val opened = mutableListOf<LoopbackReceiver>()

    @After
    fun tearDown() = opened.forEach { it.close() }

    private fun start() = LoopbackReceiver.start(state).also { opened += it }

    private fun get(r: LoopbackReceiver, pathAndQuery: String): HttpResponse<String> =
        http.send(
            HttpRequest.newBuilder(URI.create(r.redirectUri + pathAndQuery)).timeout(Duration.ofSeconds(5)).GET().build(),
            HttpResponse.BodyHandlers.ofString(),
        )

    private fun assertClosed(port: Int) {
        assertThrows(IOException::class.java) {
            Socket().use { it.connect(InetSocketAddress("127.0.0.1", port), 1000) }
        }
    }

    @Test
    fun bindsOnlyToLoopbackWithRandomPort() {
        val a = start()
        val b = start()
        assertTrue(a.redirectUri.startsWith("http://127.0.0.1:"))
        assertTrue(a.port in 1..65535)
        assertFalse(a.port == b.port)
        assertEquals(GooglePkce.redirectUri(a.port), a.redirectUri)
    }

    @Test
    fun codeWithMatchingStateThenCloses() = runBlocking<Unit> {
        val r = start()
        val result = async { r.await(10.seconds) }
        val res = get(r, "/?state=$state&code=4%2Ffake-code&scope=email")
        assertEquals(200, res.statusCode())
        assertEquals("no-store", res.headers().firstValue("Cache-Control").orElse(null))
        assertTrue(res.headers().firstValue("Content-Type").orElse("").startsWith("text/html"))
        assertTrue(res.body().contains("lang=\"ja\"") && res.body().contains("lang=\"en\""))
        assertTrue(res.body().contains("Close this tab"))
        val got = result.await()
        assertTrue(got is LoopbackResult.Code)
        assertEquals("4/fake-code", (got as LoopbackResult.Code).code)
        assertFalse(got.toString().contains("fake-code"))
        // 2 回目は繋がらない
        assertClosed(r.port)
        assertThrows(ConnectException::class.java) { get(r, "/?state=$state&code=again") }
    }

    @Test
    fun stateMismatch() = runBlocking<Unit> {
        val r = start()
        val result = async { r.await(10.seconds) }
        val res = get(r, "/?state=other&code=fake-code")
        assertEquals(200, res.statusCode())
        assertTrue(res.body().contains("try again"))
        assertEquals(LoopbackResult.StateMismatch, result.await())
        assertClosed(r.port)
    }

    @Test
    fun missingStateIsMismatch() = runBlocking<Unit> {
        val r = start()
        val result = async { r.await(10.seconds) }
        get(r, "/?code=fake-code")
        assertEquals(LoopbackResult.StateMismatch, result.await())
    }

    @Test
    fun errorFromProvider() = runBlocking<Unit> {
        val r = start()
        val result = async { r.await(10.seconds) }
        get(r, "/?state=$state&error=access_denied")
        assertEquals(LoopbackResult.Error("access_denied"), result.await())
        assertClosed(r.port)
    }

    @Test
    fun errorIsSanitized() = runBlocking<Unit> {
        val r = start()
        val result = async { r.await(10.seconds) }
        get(r, "/?state=$state&error=%3Cscript%3Ebad" + "x".repeat(100))
        val got = result.await() as LoopbackResult.Error
        assertTrue(got.error, got.error.matches(Regex("^[A-Za-z0-9_.-]{1,64}$")))
    }

    @Test
    fun missingCode() = runBlocking<Unit> {
        val r = start()
        val result = async { r.await(10.seconds) }
        get(r, "/?state=$state")
        assertEquals(LoopbackResult.Error("missing_code"), result.await())
    }

    @Test
    fun otherPathsDoNotConsume() = runBlocking<Unit> {
        val r = start()
        val result = async { r.await(10.seconds) }
        assertEquals(404, get(r, "/favicon.ico").statusCode())
        get(r, "/?state=$state&code=fake-code")
        assertTrue(result.await() is LoopbackResult.Code)
    }

    @Test
    fun timeout() = runBlocking<Unit> {
        val r = start()
        assertEquals(LoopbackResult.Timeout, r.await(200.milliseconds))
        assertClosed(r.port)
    }

    @Test
    fun cancelled() = runBlocking<Unit> {
        val r = start()
        val result = async { r.await(10.seconds) }
        r.cancel()
        assertEquals(LoopbackResult.Cancelled, result.await())
        assertClosed(r.port)
    }

    @Test
    fun defaultTimeoutIsFiveMinutes() {
        assertEquals(5L * 60, LoopbackReceiver.DEFAULT_TIMEOUT.inWholeSeconds)
    }

    @Test
    fun parseQueryKeepsFirstAndDecodes() {
        assertEquals(
            mapOf("a" to "1 2", "b" to ""),
            LoopbackReceiver.parseQuery("a=1+2&a=3&b&&"),
        )
        assertEquals(emptyMap<String, String>(), LoopbackReceiver.parseQuery(null))
    }
}
