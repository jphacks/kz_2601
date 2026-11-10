package jp.kz2601.metamon.services.live

import jp.kz2601.metamon.dsl.ai.AiTextError
import jp.kz2601.metamon.dsl.ai.AiTextRequest
import jp.kz2601.metamon.dsl.ai.AiTextShape
import jp.kz2601.metamon.services.AiTextGate
import jp.kz2601.metamon.services.http.ApiError
import jp.kz2601.metamon.services.http.ApiJson
import jp.kz2601.metamon.services.http.errorBody
import jp.kz2601.metamon.services.http.json
import jp.kz2601.metamon.services.http.metamonHttp
import jp.kz2601.metamon.services.http.take
import jp.kz2601.metamon.services.mock.MockAiText
import kotlinx.coroutines.runBlocking
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.jsonObject
import okhttp3.mockwebserver.MockWebServer
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Assert.fail
import org.junit.Before
import org.junit.Test

/** `POST /v1/text`(aiButton のオンライン版)の送り方・答えの確かめ方・失敗の分け方。live の AI は呼ばない(MockWebServer)。 */
class AiTextCallTest {
    private lateinit var server: MockWebServer
    private lateinit var client: HttpAIClient

    @Before
    fun setUp() {
        server = MockWebServer().apply { start() }
        client = HttpAIClient(server.metamonHttp())
    }

    @After
    fun tearDown() {
        server.shutdown()
    }

    private val summarize = AiTextRequest(instruction = "", shape = AiTextShape.LINE, input = "今日は晴れ", locale = "ja", task = "summarize", appId = "app1")

    @Test
    fun postsTheRequestAndNormalizesTheAnswer() = runBlocking<Unit> {
        server.enqueue(json(200, """{"shape":"line","task":"summarize","text":"「晴れの一日」","locale":"ja","meta":{}}"""))
        val out = client.text(summarize)
        assertEquals("晴れの一日", out.text)
        val r = server.take()
        assertEquals("POST", r.method)
        assertEquals("/v1/text", r.path)
        val body = ApiJson.parseToJsonElement(r.body.readUtf8()).jsonObject
        assertEquals(JsonPrimitive("summarize"), body["task"])
        assertEquals(JsonPrimitive("app1"), body["appId"])
        assertEquals(JsonPrimitive("今日は晴れ"), body["input"])
    }

    @Test
    fun answerThatDoesNotFitIsBadOutput() = runBlocking<Unit> {
        server.enqueue(json(200, """{"shape":"list","text":"","items":[],"locale":"ja","meta":{}}"""))
        expect<AiTextError.BadOutput> { client.text(summarize.copy(task = null, shape = AiTextShape.LIST)) }
    }

    @Test
    fun proRequiredSplitsByCreatorReason() = runBlocking<Unit> {
        server.enqueue(json(402, errorBody("pro_required")))
        expect<AiTextError.NeedsPro> { client.text(summarize) }
        server.enqueue(json(402, errorBody("pro_required", details = """{"reason":"creator_needs_pro"}""")))
        expect<AiTextError.CreatorNeedsPro> { client.text(summarize) }
    }

    @Test
    fun otherServerCodesMapToReasons() = runBlocking<Unit> {
        server.enqueue(json(422, errorBody("refused")))
        expect<AiTextError.Refused> { client.text(summarize) }
        server.enqueue(json(413, errorBody("payload_too_large")))
        expect<AiTextError.TooLarge> { client.text(summarize) }
        // 429 は通信の層が 1 回だけ再送する(Retry-After が長ければ再送しない)
        server.enqueue(json(429, errorBody("rate_limited", message = "今日の上限に達しました"), "Retry-After" to "3600"))
        val e = expect<AiTextError.Failed> { client.text(summarize) }
        assertEquals("今日の上限に達しました", e.detail)
        server.enqueue(json(400, errorBody("bad_request")))
        expect<ApiError> { client.text(summarize) }
    }

    @Test
    fun gateAsksConsentThenPro() {
        assertEquals(AiTextGate.Decision.NEEDS_CONSENT, AiTextGate.decide(consentGranted = false, isPro = true, enforced = true))
        assertEquals(AiTextGate.Decision.NEEDS_PRO, AiTextGate.decide(consentGranted = true, isPro = false, enforced = true))
        assertEquals(AiTextGate.Decision.SEND, AiTextGate.decide(consentGranted = true, isPro = false, enforced = false))
        assertEquals(AiTextGate.Decision.SEND, AiTextGate.decide(consentGranted = true, isPro = false, enforced = true, sharedApp = true))
        assertEquals(AiTextGate.Decision.SEND, AiTextGate.decide(consentGranted = true, isPro = true, enforced = true))
    }

    @Test
    fun mockAnswersFitTheRequestedShape() {
        assertEquals("嬉しい", MockAiText.answer(summarize.copy(task = "classify", options = listOf("嬉しい", "悲しい")))!!.text)
        assertEquals(3, MockAiText.answer(summarize.copy(task = null, shape = AiTextShape.LIST))!!.items!!.size)
        val fields = MockAiText.answer(summarize.copy(task = null, shape = AiTextShape.FIELDS, items = listOf("題")))!!
        assertEquals("題の案(モック)", fields.fields!!.single().value)
        assertTrue(MockAiText.answer(summarize.copy(task = null, length = "long"))!!.text.contains("\n\n"))
    }

    private suspend inline fun <reified T : Throwable> expect(crossinline body: suspend () -> Unit): T {
        try {
            body()
        } catch (e: Throwable) {
            if (e is T) return e
            fail("expected ${T::class.simpleName} but got $e")
        }
        fail("expected ${T::class.simpleName}")
        throw AssertionError()
    }
}
