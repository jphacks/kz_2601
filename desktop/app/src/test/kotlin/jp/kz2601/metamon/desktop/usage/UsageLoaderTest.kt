package jp.kz2601.metamon.desktop.usage

import jp.kz2601.metamon.desktop.i18n.Lang
import jp.kz2601.metamon.desktop.i18n.Strings
import jp.kz2601.metamon.desktop.net.DesktopApi
import jp.kz2601.metamon.services.live.HttpAppConfigClient
import kotlinx.coroutines.runBlocking
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test
import java.time.Instant

/** 設定の「使っている量」を読む(#2814): 200 で出す・5xx は前回の値と「読めませんでした」・初回の失敗は値なし。 */
class UsageLoaderTest {
    private lateinit var server: MockWebServer
    private val now = Instant.parse("2026-11-13T03:00:00Z")

    @Before
    fun setUp() {
        server = MockWebServer().apply { start() }
    }

    @After
    fun tearDown() {
        server.shutdown()
    }

    private fun loader(): UsageLoader {
        val http = DesktopApi.http(baseUrl = server.url("/").toString(), tokens = DesktopApi.devTokens("dev-token"))
        return UsageLoader(HttpAppConfigClient(http)) { now }
    }

    private fun json(body: String) = MockResponse().setHeader("Content-Type", "application/json").setBody(body)

    private val body = """{"plan":"free","resetAt":"2026-11-13T15:00:00Z","ai":{"used":57,"limit":60,"remaining":3},"apps":{"used":3,"limit":5}}"""

    @Test
    fun okShowsUsage() = Strings.withLang(Lang.JA) {
        server.enqueue(json(body))
        val l = loader()
        assertEquals(UsageLoader.State.Loading, l.state)
        runBlocking { l.refresh() }
        val s = (l.state as UsageLoader.State.Loaded).summary!!
        assertEquals("今日の残り 3", s.remaining)
        assertEquals("3 / 5", s.apps?.amount)
        val req = server.takeRequest()
        assertEquals("GET", req.method)
        assertEquals("/v1/me/usage", req.path)
        assertEquals("Bearer dev-token", req.getHeader("Authorization"))
    }

    @Test
    fun serverErrorKeepsLastValue() = Strings.withLang(Lang.JA) {
        server.enqueue(json(body))
        server.enqueue(MockResponse().setResponseCode(500).setBody("""{"error":{"code":"internal","message":"x"}}"""))
        val l = loader()
        runBlocking { l.refresh() }
        runBlocking { l.refresh() }
        val failed = l.state as UsageLoader.State.Failed
        assertEquals("今日の残り 3", failed.last?.remaining)
        assertEquals("使っている量を読めませんでした", Strings["desktop_usage_failed"])
    }

    @Test
    fun firstServerErrorHasNoValue() {
        server.enqueue(MockResponse().setResponseCode(500))
        val l = loader()
        runBlocking { l.refresh() }
        assertEquals(UsageLoader.State.Failed(null), l.state)
        assertTrue(Strings.withLang(Lang.EN) { Strings["desktop_usage_failed"] } == "Couldn't load your usage")
    }

    @Test
    fun missingEndpointHidesSection() {
        server.enqueue(MockResponse().setResponseCode(404).setBody("""{"error":{"code":"not_found","message":"x"}}"""))
        val l = loader()
        runBlocking { l.refresh() }
        assertEquals(UsageLoader.State.Loaded(null), l.state)
    }
}
