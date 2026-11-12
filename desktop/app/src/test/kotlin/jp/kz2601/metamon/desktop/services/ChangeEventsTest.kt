package jp.kz2601.metamon.desktop.services

import jp.kz2601.metamon.desktop.net.ClientPlatform
import jp.kz2601.metamon.desktop.net.DesktopApi
import jp.kz2601.metamon.services.http.SseFrame
import jp.kz2601.metamon.services.sync.ChangeEvent
import jp.kz2601.metamon.services.sync.DocumentFetcher
import jp.kz2601.metamon.services.sync.DocumentSync
import jp.kz2601.metamon.services.sync.HotReload
import jp.kz2601.metamon.services.sync.RealtimeDocumentSync
import jp.kz2601.metamon.services.sync.RecordsChange
import jp.kz2601.metamon.services.sync.SseChangeEventSource
import jp.kz2601.metamon.services.sync.StreamUnauthorized
import jp.kz2601.metamon.services.sync.SyncUpdate
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.CoroutineStart
import kotlinx.coroutines.async
import kotlinx.coroutines.cancel
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.flow.flowOf
import kotlinx.coroutines.flow.take
import kotlinx.coroutines.flow.toList
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.withTimeout
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test

/** 変化の通知(SSE)とホットリロードを android/services から取り込んだもの(#2807)。通信は MockWebServer だけ。 */
class ChangeEventsTest {
    private lateinit var server: MockWebServer

    @Before
    fun setUp() {
        server = MockWebServer().apply { start() }
    }

    @After
    fun tearDown() {
        server.shutdown()
    }

    private fun source() = SseChangeEventSource(
        DesktopApi.http(
            baseUrl = server.url("/").toString(),
            tokens = DesktopApi.devTokens("dev-token"),
            platform = ClientPlatform.LINUX,
            client = DesktopApi.okHttp(ClientPlatform.LINUX, "ua"),
        ),
    )

    private fun sse(vararg frames: Pair<String, String>) = MockResponse()
        .setHeader("Content-Type", "text/event-stream")
        .setBody(frames.joinToString("") { (event, data) -> "event: $event\ndata: $data\n\n" })

    /** 版は増えないので取り直さない */
    private val noFetch = DocumentFetcher { _, _ -> null }

    @Test
    fun decodesFramesAndSkipsUnknown() {
        assertEquals(ChangeEvent.Hello(3, listOf("posts", "todo")), ChangeEvent.decode(SseFrame("hello", """{"version":3,"entities":["posts","todo"]}""")))
        assertEquals(ChangeEvent.Records("posts"), ChangeEvent.decode(SseFrame("records", """{"entity":"posts"}""")))
        assertEquals(ChangeEvent.Dsl(4), ChangeEvent.decode(SseFrame("dsl", """{"version":4}""")))
        assertEquals(ChangeEvent.Bye, ChangeEvent.decode(SseFrame("bye", "")))
        assertNull(ChangeEvent.decode(SseFrame("ping", "{}")))
        assertNull(ChangeEvent.decode(SseFrame("records", "{}")))
    }

    @Test
    fun streamDeliversEventsUntilBye() = runBlocking {
        server.enqueue(sse("hello" to """{"version":2,"entities":["posts"]}""", "records" to """{"entity":"posts"}""", "bye" to "{}", "records" to """{"entity":"late"}"""))
        val got = mutableListOf<ChangeEvent>()
        val planned = source().stream("app1", forceRefresh = false) { got += it }

        assertTrue(planned)
        assertEquals(listOf(ChangeEvent.Hello(2, listOf("posts")), ChangeEvent.Records("posts")), got)
        val req = server.takeRequest()
        assertEquals("/v1/apps/app1/events", req.path)
        assertEquals("text/event-stream", req.getHeader("Accept"))
        assertEquals("Bearer dev-token", req.getHeader("Authorization"))
    }

    @Test
    fun unauthorizedThrows() = runBlocking {
        server.enqueue(MockResponse().setResponseCode(401))
        val failure = runCatching { source().stream("app1", forceRefresh = false) {} }.exceptionOrNull()
        assertTrue(failure is StreamUnauthorized)
    }

    @Test
    fun realtimeNotifiesAndReconnectsAfterBye() = runBlocking {
        server.enqueue(sse("hello" to """{"version":1,"entities":["posts","todo"]}""", "records" to """{"entity":"posts"}""", "bye" to "{}"))
        server.enqueue(sse("hello" to """{"version":1,"entities":["posts","todo"]}""", "records" to """{"entity":"todo"}"""))
        val sync = RealtimeDocumentSync(noFetch, source(), initialBackoffMillis = 10_000)

        val updates = withTimeout(10_000) { sync.events("app1", afterVersion = 1).take(3).toList() }

        assertEquals(
            listOf(SyncUpdate.Realtime(true), SyncUpdate.Records("posts"), SyncUpdate.Records("todo")),
            updates,
        )
        // bye の後はすぐ張り直す(切れた扱いにしない)
        assertEquals(2, server.requestCount)
    }

    @Test
    fun hotReloadPassesRecordsChange() = runBlocking {
        val scope = CoroutineScope(Dispatchers.Default)
        val sync = DocumentSync { _, _ -> flowOf(SyncUpdate.Realtime(true), SyncUpdate.Records("posts")) }
        val hot = HotReload(sync, scope)
        // 先に購読してから張る(SharedFlow は購読前の知らせを残さない)
        val change = scope.async(start = CoroutineStart.UNDISPATCHED) { hot.recordsChange.first() }
        hot.start("app1", 1)

        assertEquals(RecordsChange.Entity("posts"), withTimeout(5_000) { change.await() })
        scope.cancel()
    }
}
