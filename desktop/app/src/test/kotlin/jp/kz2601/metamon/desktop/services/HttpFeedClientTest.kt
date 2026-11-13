package jp.kz2601.metamon.desktop.services

import jp.kz2601.metamon.desktop.net.ClientPlatform
import jp.kz2601.metamon.desktop.net.DesktopApi
import jp.kz2601.metamon.dsl.feed.FeedQuery
import jp.kz2601.metamon.services.feeds.FeedCache
import jp.kz2601.metamon.services.live.HttpFeedClient
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.runBlocking
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.int
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Rule
import org.junit.Test
import org.junit.rules.TemporaryFolder

/** 外の公開データの API クライアントと前回の写しを android/services から取り込んだもの(#2806)。通信は MockWebServer だけ。 */
class HttpFeedClientTest {
    @get:Rule
    val dir = TemporaryFolder()

    private lateinit var server: MockWebServer

    @Before
    fun setUp() {
        server = MockWebServer().apply { start() }
    }

    @After
    fun tearDown() {
        server.shutdown()
    }

    private fun client(): HttpFeedClient = HttpFeedClient(
        DesktopApi.http(
            baseUrl = server.url("/").toString(),
            tokens = DesktopApi.devTokens("dev-token"),
            platform = ClientPlatform.WINDOWS,
            client = DesktopApi.okHttp(ClientPlatform.WINDOWS, "ua"),
        ),
    )

    private fun json(body: String, code: Int = 200) =
        MockResponse().setResponseCode(code).setHeader("Content-Type", "application/json").setBody(body)

    private val weather = FeedQuery(provider = "weather", region = "130000", limit = 3)

    private val weatherBody = """
        {"provider":"weather","title":"東京の天気","items":[{"date":"2026-11-13","title":"晴れ","max":18.5,"min":9.0,"pop":10}],
         "source":{"name":"気象庁","url":"https://www.jma.go.jp/","license":"政府標準利用規約"},"stale":false,"extra":1}
    """.trimIndent()

    @Test
    fun readsOneFeedByPost() = runBlocking {
        server.enqueue(json(weatherBody))
        val result = client().feed(weather)

        assertEquals("weather", result.provider)
        assertEquals("東京の天気", result.title)
        assertEquals("晴れ", result.items.single().title)
        assertEquals(18.5, result.items.single().max!!, 0.0)
        assertEquals("気象庁", result.source.name)
        assertFalse(result.stale)

        val req = server.takeRequest()
        assertEquals("POST", req.method)
        assertEquals("/v1/feeds/weather", req.path)
        val body = Json.parseToJsonElement(req.body.readUtf8()).jsonObject
        assertEquals("130000", body["region"]!!.jsonPrimitive.content)
        assertEquals(3, body["limit"]!!.jsonPrimitive.int)
        assertFalse(body.containsKey("q"))
    }

    @Test
    fun failureReturnsPreviousCopyAsStale() = runBlocking {
        var clock = 1_000L
        val cache = FeedCache(dir.root.resolve("feeds"), now = { clock }, io = Dispatchers.Unconfined)
        server.enqueue(json(weatherBody))
        assertFalse(cache.load(weather, force = false, client = client()).stale)

        // 10 分を過ぎて取りに行き、サーバーが落ちていたら前の行を stale で返す
        clock += FeedCache.TTL_MILLIS + 1
        server.enqueue(json("""{"error":{"code":"unavailable","message":"down"}}""", code = 500))
        val stale = cache.load(weather, force = false, client = client())
        assertTrue(stale.stale)
        assertEquals("晴れ", stale.items.single().title)
    }

    @Test
    fun copySurvivesRestartOnDisk() = runBlocking {
        val place = dir.root.resolve("feeds")
        server.enqueue(json(weatherBody))
        FeedCache(place, io = Dispatchers.Unconfined).load(weather, force = false, client = client())

        // 起動し直し(メモリは空)でも端末の写しから出す。取りに行かない
        val again = FeedCache(place, io = Dispatchers.Unconfined)
        assertNotNull(again.cached(weather))
        assertEquals("東京の天気", again.load(weather, force = false, client = { error("取りに行かない") }).title)
        assertEquals(1, server.requestCount)
    }

    @Test
    fun failureWithoutCopyThrows() = runBlocking {
        val cache = FeedCache(null)
        server.enqueue(json("""{"error":{"code":"unavailable","message":"down"}}""", code = 500))
        assertTrue(runCatching { cache.load(weather, force = false, client = client()) }.isFailure)
    }
}
