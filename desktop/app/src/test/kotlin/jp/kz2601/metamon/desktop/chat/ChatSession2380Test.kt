package jp.kz2601.metamon.desktop.chat

import jp.kz2601.metamon.services.ChatMode
import jp.kz2601.metamon.services.ChatOption
import jp.kz2601.metamon.services.ChatTurn
import jp.kz2601.metamon.services.http.ApiJson
import jp.kz2601.metamon.services.http.ClientInfo
import jp.kz2601.metamon.services.http.DeviceContext
import jp.kz2601.metamon.services.http.FailureKind
import jp.kz2601.metamon.services.http.MetamonHttp
import jp.kz2601.metamon.services.http.TokenProvider
import jp.kz2601.metamon.services.live.HttpAIClient
import kotlinx.coroutines.runBlocking
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import okhttp3.mockwebserver.RecordedRequest
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test
import java.net.InetAddress
import java.util.concurrent.TimeUnit

/** 会話の状態(#2380): `/v1/chat` を MockWebServer で往復する(本物の AI は呼ばない)。 */
class ChatSession2380Test {
    private lateinit var server: MockWebServer
    private lateinit var session: ChatSession

    @Before
    fun setUp() {
        server = MockWebServer().apply { start() }
        val base = server.url("/").newBuilder().host(InetAddress.getByName(server.hostName).hostAddress).build().toString()
        val http = MetamonHttp(
            baseUrl = base,
            tokens = TokenProvider { "token" },
            clientInfo = ClientInfo(version = "0.1.0", build = "1"),
            client = MetamonHttp.defaultClient().newBuilder().retryOnConnectionFailure(false).build(),
            device = DeviceContext(timeZone = { "Asia/Tokyo" }, language = { "ja" }),
            sleep = {},
        )
        session = ChatSession(HttpAIClient(http), appId = "new-app", mode = ChatMode.CREATE)
    }

    @After
    fun tearDown() {
        server.shutdown()
    }

    private fun json(status: Int, body: String) =
        MockResponse().setResponseCode(status).setBody(body).addHeader("Content-Type", "application/json")

    private fun error(status: Int, code: String, details: String? = null) =
        json(status, """{"error":{"code":"$code","message":"msg"${details?.let { ",\"details\":$it" } ?: ""}}}""")

    private fun take(): RecordedRequest = server.takeRequest(5, TimeUnit.SECONDS) ?: error("no request")

    private fun messages(r: RecordedRequest) = ApiJson.parseToJsonElement(r.body.readUtf8()).jsonObject["messages"]!!.jsonArray
        .map { it.jsonObject["role"]!!.jsonPrimitive.content to it.jsonObject["text"]!!.jsonPrimitive.content }

    @Test
    fun oneRoundTripAddsBothBubbles() = runBlocking {
        server.enqueue(json(200, """{"kind":"question","text":"何を記録しますか?","turn":1,"suggestions":["支出","収入"]}"""))

        assertTrue(session.send("  家計簿を作りたい  "))

        val req = take()
        assertEquals("POST", req.method)
        assertEquals("/v1/chat", req.path)
        assertEquals(listOf("user" to "家計簿を作りたい"), messages(req))
        val s = session.state.value
        assertEquals(listOf(ChatBubble(true, "家計簿を作りたい"), ChatBubble(false, "何を記録しますか?", ChatTurn.Kind.QUESTION)), s.bubbles)
        assertEquals(listOf("支出", "収入"), s.suggestions)
        assertFalse(s.pending)
        assertNull(s.error)
    }

    @Test
    fun secondSendCarriesWholeHistory() = runBlocking {
        server.enqueue(json(200, """{"kind":"question","text":"何を?","turn":1}"""))
        server.enqueue(json(200, """{"kind":"answer","text":"了解","turn":2}"""))
        session.send("家計簿")
        session.send("支出")
        take()
        assertEquals(listOf("user" to "家計簿", "assistant" to "何を?", "user" to "支出"), messages(take()))
    }

    @Test
    fun staleServerConversationIsResetAndResent() = runBlocking {
        server.enqueue(json(200, """{"kind":"answer","text":"古い","turn":4}"""))
        server.enqueue(json(204, ""))
        server.enqueue(json(200, """{"kind":"question","text":"新しい","turn":1}"""))

        session.send("家計簿")

        assertEquals("/v1/chat", take().path)
        assertEquals("DELETE", take().method)
        assertEquals("/v1/chat", take().path)
        assertEquals("新しい", session.state.value.bubbles.last().text)
    }

    @Test
    fun emptyTextIsNotSent() = runBlocking {
        assertFalse(session.send("   "))
        assertEquals(0, server.requestCount)
    }

    @Test
    fun multiSelectChipsJoinIntoOneAnswer() = runBlocking {
        server.enqueue(json(200, """{"kind":"question","text":"何を?","turn":1,"suggestions":["支出","収入","貯金"],"multiSelect":true}"""))
        server.enqueue(json(200, """{"kind":"answer","text":"了解","turn":2}"""))
        session.send("家計簿")
        session.toggle("収入")
        session.toggle("支出")
        session.toggle("貯金")
        session.toggle("貯金")
        assertEquals(listOf("収入", "支出"), session.state.value.selected)

        assertTrue(session.submitSelection())

        take()
        assertEquals("user" to "収入、支出", messages(take()).last())
        assertEquals(emptyList<String>(), session.state.value.selected)
    }

    @Test
    fun singleSelectIgnoresToggle() = runBlocking {
        server.enqueue(json(200, """{"kind":"question","text":"何を?","turn":1,"suggestions":["支出"]}"""))
        session.send("家計簿")
        session.toggle("支出")
        assertEquals(emptyList<String>(), session.state.value.selected)
    }

    @Test
    fun proposalChoiceBecomesBuildRequest() = runBlocking {
        server.enqueue(
            json(
                200,
                """{"kind":"proposal","text":"どれにしますか","turn":1,"multiSelect":true,"options":[
                    {"id":"a","title":"シンプル","instruction":"支出だけ記録"},
                    {"id":"b","title":"通知つき","instruction":"毎晩通知","requiresPro":true,"paidFeatures":[{"id":"serverRules","title":"自動の動き"}],"freeInstruction":""}
                ]}""",
            ),
        )
        session.send("家計簿")
        val options = session.state.value.options
        assertEquals(listOf("a", "b"), options.map(ChatOption::optionId))

        assertEquals(ChatBuild("支出だけ記録"), session.choose(options[0]))

        session.toggle("b")
        session.toggle("a")
        val build = session.chooseSelected()!!
        assertEquals("・支出だけ記録\n・毎晩通知", build.instruction)
        assertTrue(build.requiresPro)
        assertEquals(listOf("serverRules"), build.paidFeatures.map { it.id })
        assertEquals("支出だけ記録", build.freeInstruction)
    }

    @Test
    fun readyUsesTranscript() = runBlocking {
        server.enqueue(json(200, """{"kind":"ready","text":"作れます","turn":1,"transcript":"支出を記録する家計簿"}"""))
        session.send("家計簿")
        assertTrue(session.state.value.isReady)
        assertEquals(ChatBuild("支出を記録する家計簿"), session.build())
    }

    @Test
    fun buildWithoutReadyJoinsUnansweredUtterances() {
        val chat = listOf(
            ChatBubble(true, "家計簿"),
            ChatBubble(false, "何を?", ChatTurn.Kind.QUESTION),
            ChatBubble(true, "それって何?"),
            ChatBubble(false, "説明です", ChatTurn.Kind.ANSWER),
            ChatBubble(true, "支出"),
        )
        assertEquals("家計簿。支出", ChatSession.fallbackTranscript(chat))
    }

    @Test
    fun proRequiredIsKeptAsState() = runBlocking {
        server.enqueue(error(402, "pro_required", """{"features":[{"id":"serverRules","title":"自動の動き"}]}"""))

        assertFalse(session.send("家計簿"))

        val s = session.state.value
        assertTrue(s.error is FailureKind.ProRequiredKind)
        assertFalse(s.pending)
        // 送った文は残る
        assertEquals(listOf(ChatBubble(true, "家計簿")), s.bubbles)
    }

    @Test
    fun busyThenRetrySucceeds() = runBlocking {
        server.enqueue(error(503, "unavailable"))
        assertFalse(session.send("家計簿"))
        assertTrue(session.state.value.error is FailureKind.Busy)
        assertTrue(session.state.value.error!!.isRetryable)

        server.enqueue(json(200, """{"kind":"question","text":"何を?","turn":1}"""))
        assertTrue(session.retry())

        take()
        assertEquals(listOf("user" to "家計簿"), messages(take()))
        assertNull(session.state.value.error)
        assertEquals(2, session.state.value.bubbles.size)
    }

    @Test
    fun rateLimitedIsMapped() = runBlocking {
        // 429 は 1 回だけ自動で送り直すので 2 つ並べる
        repeat(2) { server.enqueue(error(429, "rate_limited", """{"scope":"user","limit":20,"resetAt":"2099-01-01T00:00:00Z"}""")) }
        assertFalse(session.send("家計簿"))
        val e = session.state.value.error
        assertTrue("$e", e is FailureKind.RateLimited)
    }

    @Test
    fun restartClearsAndDeletesServerConversation() = runBlocking {
        server.enqueue(json(200, """{"kind":"question","text":"何を?","turn":1}"""))
        server.enqueue(json(204, ""))
        session.send("家計簿")
        session.restart()
        take()
        val del = take()
        assertEquals("DELETE", del.method)
        assertEquals("/v1/chat/new-app", del.path)
        assertEquals(ChatState(), session.state.value)
    }
}
