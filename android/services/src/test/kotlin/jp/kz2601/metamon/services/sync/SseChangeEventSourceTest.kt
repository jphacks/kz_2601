package jp.kz2601.metamon.services.sync

import jp.kz2601.metamon.services.http.FakeTokens
import jp.kz2601.metamon.services.http.SseFrame
import jp.kz2601.metamon.services.http.metamonHttp
import jp.kz2601.metamon.services.http.take
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.withTimeout
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test
import java.util.concurrent.TimeUnit

class SseChangeEventSourceTest {
    private lateinit var server: MockWebServer

    @Before
    fun setUp() {
        server = MockWebServer().apply { start() }
    }

    @After
    fun tearDown() {
        server.shutdown()
    }

    private fun sse(body: String) = MockResponse().setResponseCode(200).addHeader("Content-Type", "text/event-stream").setBody(body)

    @Test
    fun readsEventsUntilBye() = runBlocking {
        server.enqueue(
            sse(
                "retry: 2000\n\n" +
                    "event: hello\ndata: {\"version\":7,\"entities\":[\"post\"]}\n\n" +
                    ": ping\n\n" +
                    "event: records\ndata: {\"entity\":\"post\"}\n\n" +
                    "event: dsl\ndata: {\"version\":8}\n\n" +
                    "event: bye\ndata: {}\n\n",
            ),
        )
        val events = mutableListOf<ChangeEvent>()
        val planned = withTimeout(5_000) {
            SseChangeEventSource(server.metamonHttp()).stream("app 1", false) { events += it }
        }
        assertTrue(planned)
        assertEquals(
            listOf(ChangeEvent.Hello(7, listOf("post")), ChangeEvent.Records("post"), ChangeEvent.Dsl(8)),
            events,
        )
        val req = server.take()
        assertEquals("/v1/apps/app%201/events", req.path)
        assertEquals("text/event-stream", req.getHeader("Accept"))
        assertEquals("Bearer token-1", req.getHeader("Authorization"))
    }

    @Test
    fun endWithoutByeIsNotPlanned() = runBlocking {
        server.enqueue(sse("event: hello\ndata: {\"version\":1}\n\nevent: records\ndata: {\"entity\":\"po"))
        val events = mutableListOf<ChangeEvent>()
        val planned = withTimeout(5_000) {
            SseChangeEventSource(server.metamonHttp()).stream("app1", false) { events += it }
        }
        assertFalse(planned)
        assertEquals(listOf(ChangeEvent.Hello(1, emptyList())), events) // 区切りの無い最後の 1 件は捨てる
    }

    @Test
    fun unauthorizedThrowsAndForceRefreshTakesNewToken() = runBlocking<Unit> {
        server.enqueue(MockResponse().setResponseCode(401).setBody("{}"))
        server.enqueue(sse("event: bye\ndata: {}\n\n"))
        val tokens = FakeTokens("old", "new")
        val source = SseChangeEventSource(server.metamonHttp(tokens = tokens))
        val first = runCatching { source.stream("app1", false) {} }
        assertTrue(first.exceptionOrNull() is StreamUnauthorized)
        assertTrue(source.stream("app1", true) {})
        assertEquals("Bearer old", server.take().getHeader("Authorization"))
        assertEquals("Bearer new", server.take().getHeader("Authorization"))
    }

    @Test
    fun serverErrorThrowsStatus() = runBlocking<Unit> {
        server.enqueue(MockResponse().setResponseCode(503))
        val e = runCatching { SseChangeEventSource(server.metamonHttp()).stream("app1", false) {} }.exceptionOrNull()
        assertEquals(503, (e as StreamStatus).status)
    }

    @Test
    fun silentStreamTimesOut() = runBlocking<Unit> {
        // 1 秒に 1 バイトしか届かない(心拍が途絶えた)
        server.enqueue(sse("event: hello\ndata: {}\n\n").throttleBody(1, 1, TimeUnit.SECONDS))
        val result = runCatching {
            withTimeout(5_000) { SseChangeEventSource(server.metamonHttp(), idleTimeoutMillis = 300).stream("app1", false) {} }
        }
        assertTrue(result.exceptionOrNull() is java.io.IOException) // 心拍が途絶えたら切れたとみなす
    }

    @Test
    fun decodesUnknownAndBrokenFramesAsNull() {
        assertNull(ChangeEvent.decode(SseFrame("unknown", "{}")))
        assertNull(ChangeEvent.decode(SseFrame("records", "not json")))
        assertEquals(ChangeEvent.Dsl(null), ChangeEvent.decode(SseFrame("dsl", "{}")))
    }
}
