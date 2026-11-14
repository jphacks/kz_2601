package jp.kz2601.metamon.services.live

import jp.kz2601.metamon.dsl.ai.AiTextError
import jp.kz2601.metamon.dsl.ai.AiTextMessage
import jp.kz2601.metamon.dsl.ai.AiTextRequest
import jp.kz2601.metamon.dsl.ai.AiTextSaveField
import jp.kz2601.metamon.dsl.ai.AiTextSaveTo
import jp.kz2601.metamon.dsl.ai.AiTextShape
import jp.kz2601.metamon.services.http.ApiJson
import jp.kz2601.metamon.services.http.errorBody
import jp.kz2601.metamon.services.http.json
import jp.kz2601.metamon.services.http.metamonHttp
import jp.kz2601.metamon.services.http.take
import jp.kz2601.metamon.services.mock.MockAiText
import kotlinx.coroutines.runBlocking
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.jsonObject
import okhttp3.mockwebserver.MockWebServer
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.fail
import org.junit.Before
import org.junit.Test

/** aiChat(task chat)の `POST /v1/text`: 会話・saveTo を送り、返事と記録の案を受け取る。live の AI は呼ばない(MockWebServer)。 */
class AiChatTextCallTest {
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

    private val saveTo = AiTextSaveTo("meal", "食事", "2026-01-01", listOf(AiTextSaveField("food", "料理", "string")))

    private fun chat(saveTo: AiTextSaveTo? = null) = AiTextRequest(
        instruction = "先生",
        shape = AiTextShape.LINE,
        locale = "ja",
        task = "chat",
        appId = "app1",
        messages = listOf(AiTextMessage("user", "今日カレー食べた")),
        saveTo = saveTo,
    )

    @Test
    fun sendsTheConversationAndReadsTheSuggestion() = runBlocking<Unit> {
        server.enqueue(json(200, """{"shape":"line","text":"おいしそう!","suggest":{"food":"カレー"}}"""))
        val out = client.text(chat(saveTo))
        assertEquals("おいしそう!", out.text)
        assertEquals(JsonPrimitive("カレー"), out.suggest!!["food"])
        val body = ApiJson.parseToJsonElement(server.take().body.readUtf8()).jsonObject
        assertEquals(JsonPrimitive("chat"), body["task"])
        assertEquals(1, body["messages"]!!.jsonArray.size)
        assertEquals(JsonPrimitive("meal"), body["saveTo"]!!.jsonObject["entity"])
        assertNull(body["material"])
    }

    @Test
    fun ignoresSuggestionWithoutSaveTo() = runBlocking<Unit> {
        server.enqueue(json(200, """{"shape":"line","text":"はい","suggest":{"food":"カレー"}}"""))
        assertNull(client.text(chat()).suggest)
    }

    @Test
    fun chatPayloadTooLargeHasItsOwnReason() = runBlocking<Unit> {
        server.enqueue(json(413, errorBody("payload_too_large")))
        try {
            client.text(chat())
            fail()
        } catch (e: AiTextError) {
            assertEquals(AiTextError.ChatTooLong, e)
        }
    }

    @Test
    fun mockReplyWithSuggestion() {
        val out = MockAiText.answer(chat(saveTo))!!
        assertEquals(JsonPrimitive("今日カレー食べた"), out.suggest!!["food"])
        assertNull(MockAiText.answer(chat())!!.suggest)
    }
}
