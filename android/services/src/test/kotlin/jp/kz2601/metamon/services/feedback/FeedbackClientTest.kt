package jp.kz2601.metamon.services.feedback

import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.AppDocument
import jp.kz2601.metamon.dsl.decode
import jp.kz2601.metamon.services.http.ApiError
import jp.kz2601.metamon.services.http.ApiJson
import jp.kz2601.metamon.services.http.FailureKind
import jp.kz2601.metamon.services.http.errorBody
import jp.kz2601.metamon.services.http.json
import jp.kz2601.metamon.services.http.metamonHttp
import jp.kz2601.metamon.services.http.take
import kotlinx.coroutines.runBlocking
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import okhttp3.mockwebserver.MockWebServer
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Assert.fail
import org.junit.Before
import org.junit.Test
import java.util.Base64

class FeedbackClientTest {
    private lateinit var server: MockWebServer
    private lateinit var sender: FeedbackSender

    private val device = FeedbackSubmission.Device("0.1.0", "7", "Android 15", "Google Pixel 9", "ja")
    private val doc = AppDocument(
        appId = "kakeibo-1",
        version = 3,
        dsl = AppDSL.decode(
            """{"schemaVersion":1,"app":{"name":"家計簿","voiceEntry":"floating"},"initialScreen":"home",
            "entities":[{"id":"expense","name":"支出","fields":[{"id":"amount","name":"金額","type":"number"},{"id":"memo","name":"メモ","type":"text"}]}],
            "screens":[{"id":"home","title":"ホーム","components":[]},{"id":"add","components":[]}]}""",
        ),
    )
    private val context = FeedbackContext(FeedbackContext.appSummary(doc, cachedEnvelope = null), device, hasScreenshot = true)
    private val jpeg = byteArrayOf(0xFF.toByte(), 0xD8.toByte(), 1, 2, 3)

    @Before
    fun setUp() {
        server = MockWebServer().apply { start() }
        sender = FeedbackSender(HttpFeedbackClient(server.metamonHttp()))
    }

    @After
    fun tearDown() {
        server.shutdown()
    }

    private fun receipt(screenshot: Boolean = false, dsl: Boolean = false, device: Boolean = false) = json(
        201,
        """{"id":"fb_1","status":"received","createdAt":"2026-11-11T00:00:00Z","attachments":{"screenshot":$screenshot,"dsl":$dsl,"device":$device}}""",
    )

    @Test
    fun withoutConsentNothingIsSent() = runBlocking {
        val draft = FeedbackDraft(message = "グラフが空のまま", attachScreenshot = true, attachApp = true, attachDevice = true)
        assertFalse(draft.canSend)
        var shotAsked = false
        val result = sender.send(draft, context) { shotAsked = true; jpeg }
        assertEquals(FeedbackSender.Result.NotReady, result)
        assertFalse("同意が無ければスクショも縮めない", shotAsked)
        assertEquals(0, server.requestCount)
    }

    @Test
    fun emptyOrTooLongMessageIsNotSent() = runBlocking {
        assertEquals(FeedbackSender.Result.NotReady, sender.send(FeedbackDraft(message = "  \n ", agreed = true), context) { jpeg })
        val long = FeedbackDraft(message = "あ".repeat(FeedbackConsent.MAX_MESSAGE_LENGTH + 1), agreed = true)
        assertTrue(long.isTooLong)
        assertEquals(FeedbackSender.Result.NotReady, sender.send(long, context) { jpeg })
        assertEquals(0, server.requestCount)
    }

    @Test
    fun defaultsSendOnlyTheTextAndConsentVersion() = runBlocking {
        server.enqueue(receipt())
        val result = sender.send(FeedbackDraft(message = "  グラフが空のまま \n", agreed = true), context) { error("スクショはオフ") }
        assertTrue(result is FeedbackSender.Result.Sent)
        val req = server.take()
        assertEquals("POST", req.method)
        assertEquals("/v1/feedback", req.path)
        assertEquals("Bearer token-1", req.getHeader("Authorization"))
        val body = ApiJson.parseToJsonElement(req.body.readUtf8()).jsonObject
        assertEquals(
            JsonObject(mapOf("kind" to JsonPrimitive("bug"), "message" to JsonPrimitive("グラフが空のまま"), "consentVersion" to JsonPrimitive("feedback-2026-11"))),
            body,
        )
    }

    @Test
    fun onlyTheAttachmentsTurnedOnAreSent() = runBlocking {
        server.enqueue(receipt(screenshot = true, dsl = true, device = true))
        server.enqueue(receipt(dsl = true))
        val all = FeedbackDraft(kind = FeedbackSubmission.Kind.IDEA, message = "欲しい", attachScreenshot = true, attachApp = true, attachDevice = true, agreed = true)
        val sent = sender.send(all, context) { jpeg } as FeedbackSender.Result.Sent
        assertEquals(FeedbackReceipt.Attachments(screenshot = true, dsl = true, device = true), sent.receipt.attachments)
        val a = ApiJson.parseToJsonElement(server.take().body.readUtf8()).jsonObject
        assertEquals("idea", a["kind"]!!.jsonPrimitive.content)
        val att = a["attachments"]!!.jsonObject
        assertEquals(setOf("screenshot", "app", "device"), att.keys)
        assertTrue(Base64.getDecoder().decode(att["screenshot"]!!.jsonPrimitive.content).contentEquals(jpeg))
        // DSL は参照だけ(中身・記録は送らない)
        assertEquals(JsonObject(mapOf("appId" to JsonPrimitive("kakeibo-1"), "dslVersion" to JsonPrimitive(3))), att["app"])
        assertEquals(setOf("appVersion", "build", "os", "model", "locale"), att["device"]!!.jsonObject.keys)

        sender.send(FeedbackDraft(message = "x", attachApp = true, agreed = true), context) { error("スクショはオフ") }
        val b = ApiJson.parseToJsonElement(server.take().body.readUtf8()).jsonObject
        assertEquals(setOf("app"), b["attachments"]!!.jsonObject.keys)
    }

    @Test
    fun noAppSummaryMeansNoAppAttachmentEvenIfToggled() {
        val noApp = context.copy(app = null, hasScreenshot = false)
        val built = FeedbackDraft(message = "x", attachScreenshot = true, attachApp = true, agreed = true).build(noApp) { error("スクショは無い") }
        val s = (built as FeedbackDraft.Built.Ready).submission
        assertNull(s.app)
        assertNull(s.screenshotJpeg)
    }

    @Test
    fun screenshotThatCannotShrinkIsNotSent() = runBlocking {
        val draft = FeedbackDraft(message = "x", attachScreenshot = true, agreed = true)
        assertEquals(FeedbackSender.Result.ScreenshotTooLarge, sender.send(draft, context) { null })
        assertEquals(FeedbackSender.Result.ScreenshotTooLarge, sender.send(draft, context) { ByteArray(FeedbackConsent.MAX_SCREENSHOT_BYTES + 1) })
        assertEquals(0, server.requestCount)
    }

    @Test
    fun deviceFieldsAreClippedTo64() {
        val long = device.copy(model = "m".repeat(100))
        val body = ApiJson.parseToJsonElement(HttpFeedbackClient.bodyJson(FeedbackSubmission(message = "x", device = long))).jsonObject
        assertEquals(64, body["attachments"]!!.jsonObject["device"]!!.jsonObject["model"]!!.jsonPrimitive.content.length)
    }

    @Test
    fun dailyLimitIsRateLimited() = runBlocking {
        server.enqueue(
            json(429, errorBody("rate_limited", details = """{"scope":"user","limit":5,"resetAt":"2026-11-12T00:00:00+09:00","retryAfterSec":36000}"""), "Retry-After" to "36000"),
        )
        try {
            sender.send(FeedbackDraft(message = "x", agreed = true), context) { null }
            fail("429 は失敗")
        } catch (e: ApiError) {
            assertTrue(FailureKind.classify(e).toString(), FailureKind.classify(e) is FailureKind.RateLimited)
        }
        assertEquals("翌日まで待つ 429 は再送しない", 1, server.requestCount)
    }

    @Test
    fun appSummaryShowsScreensAndFieldsWithoutRecords() {
        val s = FeedbackContext.appSummary(doc, cachedEnvelope = null)!!
        assertEquals("家計簿", s.name)
        assertEquals(listOf("ホーム", "add"), s.screens)
        assertEquals(listOf("支出(金額, メモ)"), s.entities)
        assertNull(s.json)
        // 初期状態(版 0)は送らない
        assertNull(FeedbackContext.appSummary(doc.copy(version = 0), null))
    }

    @Test
    fun cachedDslIsShownOnlyForTheSameVersion() {
        val envelope = """{"appId":"kakeibo-1","version":3,"dsl":{"schemaVersion":1,"app":{"name":"家計簿"},"initialScreen":"home"}}"""
        val shown = FeedbackContext.appSummary(doc, envelope)!!.json!!
        assertTrue(shown, shown.indexOf("\"app\"") < shown.indexOf("\"initialScreen\"") && shown.indexOf("\"initialScreen\"") < shown.indexOf("\"schemaVersion\""))
        assertFalse(shown.contains("kakeibo-1"))
        assertNull(FeedbackContext.appSummary(doc, envelope.replace("\"version\":3", "\"version\":2"))!!.json)
        assertNull(FeedbackContext.appSummary(doc, "not json")!!.json)
    }

    @Test
    fun mockRemembersWhatWasSent() = runBlocking {
        val mock = MockFeedbackClient()
        val r = FeedbackSender(mock).send(FeedbackDraft(message = "x", attachDevice = true, agreed = true), context) { null }
        assertEquals(FeedbackReceipt.Attachments(screenshot = false, dsl = false, device = true), (r as FeedbackSender.Result.Sent).receipt.attachments)
        assertEquals(device, mock.sent.single().device)
    }
}
