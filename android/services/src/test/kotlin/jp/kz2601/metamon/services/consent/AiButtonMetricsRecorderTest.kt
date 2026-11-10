package jp.kz2601.metamon.services.consent

import jp.kz2601.metamon.services.http.ApiError
import jp.kz2601.metamon.services.http.json
import jp.kz2601.metamon.services.http.metamonHttp
import jp.kz2601.metamon.services.http.take
import kotlinx.coroutines.runBlocking
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.int
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import okhttp3.mockwebserver.MockWebServer
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.IOException
import java.time.Instant
import java.time.ZoneId

/** aiButton の集計を貯めて、前の日までの分を 1 日 1 回送る(iOS の AIButtonMetricsRecorderTests と同じ筋)。 */
class AiButtonMetricsRecorderTest {
    private val tokyo = ZoneId.of("Asia/Tokyo")
    private var now = Instant.parse("2026-11-10T03:00:00Z") // 東京の 11/10 12:00
    private val prefs = InMemoryKeyValueStore()
    private var ids = 0
    private fun recorder() = AiButtonMetricsRecorder(prefs, clock = { now }, zone = { tokyo }, newReportId = { "report-${++ids}" })

    private class Sender(var fail: Exception? = null) : AiButtonMetricsSender {
        val sent = mutableListOf<AiButtonMetricsReport>()
        override suspend fun send(report: AiButtonMetricsReport) {
            fail?.let { throw it }
            sent += report
        }
    }

    private fun nextDay() {
        now = now.plusSeconds(24 * 3600)
    }

    @Test
    fun sendsPreviousDaysOnceADay() = runBlocking {
        val r = recorder()
        val sender = Sender()
        r.recordShown("deviceNotEligible")
        r.recordRan("summarize", null)
        r.recordRan("summarize", "invalidResponse")
        r.flush(sender)
        assertTrue("今日の分はまだ送らない", sender.sent.isEmpty())

        nextDay()
        r.flush(sender)
        r.flush(sender) // 同じ日の 2 回目は送らない
        assertEquals(1, sender.sent.size)
        val day = sender.sent.single().days.single()
        assertEquals("2026-11-10", day.day)
        assertEquals("device_not_eligible", day.availability)
        assertEquals(listOf(AiButtonMetricsReport.TaskCount("summarize", 2, mapOf("invalid_response" to 1))), day.tasks)
        assertEquals("report-1", day.reportId)
    }

    @Test
    fun labelsAreBackendSnakeCaseAndUnknownBecomesOther() {
        val r = recorder()
        r.recordShown("appleIntelligenceOff")
        r.recordRan("my-own-task", "badConfig")
        r.recordRan(null, "somethingNew")
        nextDay()
        val day = r.pendingReport()!!.days.single()
        assertEquals("apple_intelligence_off", day.availability)
        assertEquals(listOf(AiButtonMetricsReport.TaskCount("other", 2, mapOf("bad_config" to 1, "model_error" to 1))), day.tasks)
    }

    @Test
    fun keepsOnFailureAndDropsOn400() = runBlocking {
        val r = recorder()
        r.recordRan("classify", null)
        nextDay()
        val sender = Sender(fail = IOException("offline"))
        r.flush(sender)
        assertTrue(r.pendingReport() != null) // 圏外は次回に回す
        sender.fail = ApiError.Http(400, "bad")
        r.flush(sender)
        assertNull(r.pendingReport()) // 形の誤りは捨てる
    }

    @Test
    fun survivesRestart() {
        recorder().recordRan("rewrite", null)
        nextDay()
        val day = recorder().pendingReport()!!.days.single()
        assertEquals("available", day.availability) // 端末内で動いた = 使えた
        assertEquals("rewrite", day.tasks.single().task)
    }

    @Test
    fun turningOffClearsAndStopsCounting() {
        val r = recorder()
        r.recordRan("rewrite", null)
        r.setEnabled(false)
        r.recordRan("rewrite", null)
        nextDay()
        assertNull(r.pendingReport())
        assertNull(prefs.getString(AiButtonMetricsRecorder.BUCKETS_KEY))
    }

    @Test
    fun offDoesNotSend() = runBlocking {
        val r = recorder()
        r.recordRan("rewrite", null)
        nextDay()
        r.setEnabled(false)
        val sender = Sender()
        r.flush(sender)
        assertTrue(sender.sent.isEmpty())
    }

    @Test
    fun dropsDaysOlderThanKeepDays() = runBlocking {
        val r = recorder()
        r.recordRan("extract", null)
        repeat(AiButtonMetricsRecorder.KEEP_DAYS + 1) { nextDay() }
        val sender = Sender()
        r.flush(sender)
        assertTrue(sender.sent.isEmpty())
        assertNull(r.pendingReport())
    }

    @Test
    fun httpSenderPostsToMetricsPath() = runBlocking {
        val server = MockWebServer().apply { start() }
        try {
            server.enqueue(json(200, "{}"))
            val r = recorder()
            r.recordShown("available")
            r.recordRan("suggest", null)
            nextDay()
            r.flush(HttpAiButtonMetricsSender(server.metamonHttp()))
            val req = server.take()
            assertEquals("POST", req.method)
            assertEquals("/v1/metrics/ai-button", req.path)
            val day = Json.parseToJsonElement(req.body.readUtf8()).jsonObject["days"]!!.jsonArray.single().jsonObject
            assertEquals("2026-11-10", day["day"]!!.jsonPrimitive.content)
            assertEquals(1, day["tasks"]!!.jsonArray.single().jsonObject["attempts"]!!.jsonPrimitive.int)
            assertNull(r.pendingReport())
        } finally {
            server.shutdown()
        }
    }
}
