package jp.kz2601.metamon.services.live

import jp.kz2601.metamon.services.ChatMessage
import jp.kz2601.metamon.services.http.ApiJson
import jp.kz2601.metamon.services.http.DeviceContext
import jp.kz2601.metamon.services.http.FakeTokens
import jp.kz2601.metamon.services.http.MetamonHttp
import jp.kz2601.metamon.services.http.baseUrl
import jp.kz2601.metamon.services.http.json
import jp.kz2601.metamon.services.http.take
import jp.kz2601.metamon.services.http.testClientInfo
import kotlinx.coroutines.runBlocking
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.jsonObject
import okhttp3.mockwebserver.MockWebServer
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Before
import org.junit.Test

/** サーバーへ送る言語がアプリで選んだ言語に従う(#843)。 */
class LocaleDefaultTest {
    private lateinit var server: MockWebServer
    private var language = "en"

    @Before
    fun setUp() {
        server = MockWebServer().apply { start() }
    }

    @After
    fun tearDown() {
        server.shutdown()
    }

    private fun client() = HttpAIClient(
        MetamonHttp(
            baseUrl = server.baseUrl(),
            tokens = FakeTokens("token-1"),
            clientInfo = testClientInfo,
            device = DeviceContext(timeZone = { "Asia/Tokyo" }, language = { language }),
            sleep = {},
        ),
    )

    @Test
    fun chatWithoutLocaleSendsSelectedLanguageInBodyAndHeader() = runBlocking<Unit> {
        server.enqueue(json(200, """{"kind":"answer","text":"ok","turn":1}"""))
        client().chat("app1", listOf(ChatMessage(ChatMessage.Role.USER, "hi")))

        val req = server.take()
        assertEquals("en", req.getHeader("Accept-Language"))
        assertEquals(JsonPrimitive("en"), ApiJson.parseToJsonElement(req.body.readUtf8()).jsonObject["locale"])
    }

    @Test
    fun languageIsReadPerRequestSoSwitchingTakesEffectImmediately() = runBlocking<Unit> {
        val c = client()
        server.enqueue(json(200, """{"kind":"answer","text":"ok","turn":1}"""))
        server.enqueue(json(200, """{"kind":"answer","text":"ok","turn":2}"""))

        c.chat("app1", emptyList())
        language = "ja"
        c.chat("app1", emptyList())

        assertEquals("en", server.take().getHeader("Accept-Language"))
        val second = server.take()
        assertEquals("ja", second.getHeader("Accept-Language"))
        assertEquals(JsonPrimitive("ja"), ApiJson.parseToJsonElement(second.body.readUtf8()).jsonObject["locale"])
    }

    @Test
    fun explicitLocaleWins() = runBlocking<Unit> {
        server.enqueue(json(200, """{"kind":"answer","text":"ok","turn":1}"""))
        client().chat("app1", emptyList(), locale = "ja")

        assertEquals(JsonPrimitive("ja"), ApiJson.parseToJsonElement(server.take().body.readUtf8()).jsonObject["locale"])
    }
}
