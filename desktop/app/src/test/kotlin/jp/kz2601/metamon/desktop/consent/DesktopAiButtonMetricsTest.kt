package jp.kz2601.metamon.desktop.consent

import jp.kz2601.metamon.desktop.net.DesktopApi
import jp.kz2601.metamon.desktop.net.DesktopApiConfig
import jp.kz2601.metamon.desktop.store.JsonKeyValueStore
import jp.kz2601.metamon.services.consent.AiButtonMetricsRecorder
import jp.kz2601.metamon.services.consent.AiConsentStore
import jp.kz2601.metamon.services.consent.HttpAiButtonMetricsSender
import kotlinx.coroutines.runBlocking
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.int
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Before
import org.junit.Test
import java.time.Instant
import java.time.ZoneId
import java.util.concurrent.TimeUnit

/** aiButton の集計(#2817): 前の日の分を 1 回にまとめて送る・同じ日に 2 回送らない・AI の同意が無ければ数えず送らない。 */
class DesktopAiButtonMetricsTest {
    private lateinit var server: MockWebServer
    private var now = Instant.parse("2026-11-12T03:00:00Z")
    private val consent = AiConsentStore(JsonKeyValueStore(null))
    private val recorder = AiButtonMetricsRecorder(JsonKeyValueStore(null), clock = { now }, zone = { ZoneId.of("Asia/Tokyo") }, newReportId = { "r-1" })

    @Before
    fun setUp() {
        server = MockWebServer().apply { start() }
    }

    @After
    fun tearDown() {
        server.shutdown()
    }

    private fun metrics(): DesktopAiButtonMetrics {
        val http = DesktopApi.http(baseUrl = server.url("/").toString(), tokens = DesktopApi.devTokens("dev-token"))
        return DesktopAiButtonMetrics(recorder, consent, HttpAiButtonMetricsSender(http))
    }

    @Test
    fun sendsYesterdayOnceInOneRequest() = runBlocking {
        consent.grant()
        val m = metrics()
        m.shown("available")
        m.ran("summarize", null)
        m.ran("summarize", "rateLimited")
        m.ran("customTask", null)
        // 今日の分はまだ送らない
        m.flush()
        assertEquals(0, server.requestCount)

        now = Instant.parse("2026-11-13T03:00:00Z")
        server.enqueue(MockResponse().setResponseCode(204))
        m.flush()
        m.flush() // 同じ日の 2 回目は送らない
        assertEquals(1, server.requestCount)
        val req = server.takeRequest(1, TimeUnit.SECONDS)!!
        assertEquals("POST", req.method)
        assertEquals("/v1/metrics/ai-button", req.path)
        val day = Json.parseToJsonElement(req.body.readUtf8()).jsonObject["days"]!!.jsonArray.single().jsonObject
        assertEquals("2026-11-12", day["day"]!!.jsonPrimitive.content)
        assertEquals("available", day["availability"]!!.jsonPrimitive.content)
        val tasks = day["tasks"]!!.jsonArray.associate { it.jsonObject["task"]!!.jsonPrimitive.content to it.jsonObject }
        assertEquals(2, tasks.getValue("summarize")["attempts"]!!.jsonPrimitive.int)
        assertEquals(1, tasks.getValue("summarize")["failures"]!!.jsonObject["rate_limited"]!!.jsonPrimitive.int)
        assertEquals(1, tasks.getValue("other")["attempts"]!!.jsonPrimitive.int)
        assertNull(recorder.pendingReport())
    }

    @Test
    fun withoutConsentNothingIsCountedOrSent() = runBlocking {
        val m = metrics()
        m.shown("available")
        m.ran("summarize", null)
        now = Instant.parse("2026-11-13T03:00:00Z")
        m.flush()
        assertEquals(0, server.requestCount)
        assertNull(recorder.pendingReport())
    }

    @Test
    fun consentRevokedBeforeSendingKeepsCountsUnsent() = runBlocking {
        consent.grant()
        val m = metrics()
        m.ran("summarize", null)
        consent.revoke()
        now = Instant.parse("2026-11-13T03:00:00Z")
        m.flush()
        assertEquals(0, server.requestCount)
    }

    @Test
    fun mockConfigHasNoSender() {
        assertNull(DesktopApi.aiButtonMetricsSender(DesktopApiConfig()))
        assertNull(DesktopApi.aiButtonMetricsSender(DesktopApiConfig(mode = DesktopApiConfig.Mode.STG, devToken = null)))
    }
}
