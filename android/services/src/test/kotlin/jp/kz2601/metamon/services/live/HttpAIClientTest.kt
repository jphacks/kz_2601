package jp.kz2601.metamon.services.live

import jp.kz2601.metamon.services.ChatMessage
import jp.kz2601.metamon.services.ChatMode
import jp.kz2601.metamon.services.ChatTurn
import jp.kz2601.metamon.services.ClientCapabilities
import jp.kz2601.metamon.services.GenerateMode
import jp.kz2601.metamon.services.GenerateProgress
import jp.kz2601.metamon.services.StreamedComponent
import jp.kz2601.metamon.services.http.ApiError
import jp.kz2601.metamon.services.http.ApiJson
import jp.kz2601.metamon.services.http.FailureKind
import jp.kz2601.metamon.services.http.MINI_DSL
import jp.kz2601.metamon.services.http.SseFrame
import jp.kz2601.metamon.services.http.errorBody
import jp.kz2601.metamon.services.http.expectApiError
import jp.kz2601.metamon.services.http.json
import jp.kz2601.metamon.services.http.metamonHttp
import jp.kz2601.metamon.services.http.take
import kotlinx.coroutines.runBlocking
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.jsonObject
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test

class HttpAIClientTest {
    private lateinit var server: MockWebServer
    private lateinit var client: HttpAIClient

    @Before
    fun setUp() {
        server = MockWebServer().apply { start() }
        client = HttpAIClient(server.metamonHttp(), ClientCapabilities(supportedFeatures = listOf("recordRetention")))
    }

    @After
    fun tearDown() {
        server.shutdown()
    }

    private fun result(version: Int = 3, summary: String = "家計簿を作りました") =
        """{"appId":"app1","version":$version,"mode":"full","dsl":$MINI_DSL,"summary":"$summary","meta":{"publicPersonalData":[{"entity":"e","field":"phone"}]}}"""

    private fun sse(vararg frames: Pair<String, String>, truncated: String? = null): MockResponse {
        val text = frames.joinToString("") { (event, data) -> "event: $event\ndata: $data\n\n" } + (truncated ?: "")
        return MockResponse().setResponseCode(200).addHeader("Content-Type", "text/event-stream").setChunkedBody(text, 7)
    }

    private fun body(r: okhttp3.mockwebserver.RecordedRequest): JsonObject =
        ApiJson.parseToJsonElement(r.body.readUtf8()).jsonObject

    // MARK: SSE のイベント

    @Test
    fun decodesEveryIosEventName() {
        fun decode(event: String, data: String) = GenerateStreamEvent.decode(SseFrame(event, data))

        assertEquals(
            GenerateStreamEvent.Progress(GenerateProgress.App("家計簿", listOf("ホーム", "集計"), 1)),
            decode("app", """{"type":"app","attempt":1,"name":" 家計簿 ","tabs":["ホーム","集計"]}"""),
        )
        assertEquals(
            GenerateStreamEvent.Progress(GenerateProgress.Summary("メモ欄を消します", 0)),
            decode("summary", """{"type":"summary","summary":"メモ欄を消します"}"""),
        )
        assertEquals(
            GenerateStreamEvent.Progress(
                GenerateProgress.Component(StreamedComponent(1, "home", 0, "c1", "list", "main", "支出")),
            ),
            decode("component", """{"attempt":1,"screen":"home","component":{"index":0,"id":"c1","type":"list","area":"main","label":"支出"}}"""),
        )
        assertEquals(GenerateStreamEvent.Progress(GenerateProgress.Restart(2)), decode("restart", """{"attempt":2}"""))
        assertEquals(GenerateStreamEvent.Progress(GenerateProgress.Restart(0)), decode("restart", "broken"))
        assertEquals(GenerateStreamEvent.Result("{}"), decode("result", "{}"))

        val failure = decode("error", """{"error":{"code":"pro_required","message":"Pro","status":402,"details":{"features":[]}}}""")
        val error = (failure as GenerateStreamEvent.Failure).error
        assertEquals(402, error.status)
        assertTrue(FailureKind.classify(error) is FailureKind.ProRequiredKind)

        assertNull(decode("app", """{"name":"  "}"""))
        assertNull(decode("component", """{"component":{"type":"","index":0}}"""))
        assertNull(decode("ping", "{}"))
    }

    // MARK: generate(stream)

    @Test
    fun streamingDeliversProgressInOrderIncludingRestart() = runBlocking<Unit> {
        server.enqueue(
            sse(
                "app" to """{"attempt":0,"name":"家計簿","tabs":["ホーム"]}""",
                "component" to """{"attempt":0,"screen":"home","component":{"index":0,"type":"text"}}""",
                "restart" to """{"attempt":1}""",
                "app" to """{"attempt":1,"name":"家計簿","tabs":["ホーム"]}""",
                "result" to result(),
            ),
        )
        val seen = mutableListOf<GenerateProgress>()

        val r = client.morphStreaming("家計簿", "app1", currentVersion = 2, progress = { seen += it })

        assertEquals(
            listOf(
                GenerateProgress.App("家計簿", listOf("ホーム"), 0),
                GenerateProgress.Component(StreamedComponent(0, "home", 0, type = "text")),
                GenerateProgress.Restart(1),
                GenerateProgress.App("家計簿", listOf("ホーム"), 1),
            ),
            seen,
        )
        assertEquals(3, r.document.version)
        assertEquals("家計簿を作りました", r.summary)
        assertEquals(1, r.publicPersonalData.size)
        assertEquals(1, server.requestCount)

        val req = server.take()
        assertEquals("/v1/generate?stream=1", req.path)
        assertEquals("text/event-stream", req.getHeader("Accept"))
        val sent = body(req)
        assertEquals(JsonPrimitive(2), sent["baseVersion"])
        assertEquals(JsonPrimitive("家計簿"), sent["transcript"])
        assertEquals(JsonPrimitive("auto"), sent["mode"])
        assertNull(sent["locale"])
        assertNull(sent["allowedComponents"])
        assertEquals("""["recordRetention"]""", sent["supportedFeatures"].toString())
    }

    @Test
    fun streamErrorEventIsThrownWithoutFallback() = runBlocking<Unit> {
        server.enqueue(sse("error" to """{"error":{"code":"generation_failed","message":"だめ","status":422}}"""))
        val e = expectApiError { client.morphStreaming("x", "app1", 0, progress = {}) }
        assertEquals(422, e.status)
        assertEquals("generation_failed", e.code)
        assertEquals(1, server.requestCount)
    }

    @Test
    fun truncatedStreamRestartsAndFallsBackToPlainRequest() = runBlocking<Unit> {
        server.enqueue(sse("app" to """{"name":"家計簿"}""", truncated = "event: result\ndata: {\"partial"))
        server.enqueue(json(200, result(version = 4)))
        val seen = mutableListOf<GenerateProgress>()

        val r = client.morphStreaming("x", "app1", 0, progress = { seen += it })

        assertEquals(listOf(GenerateProgress.App("家計簿", emptyList(), 0), GenerateProgress.Restart(0)), seen)
        assertEquals(4, r.document.version)
        assertEquals("/v1/generate?stream=1", server.take().path)
        assertEquals("/v1/generate", server.take().path)
    }

    @Test
    fun rateLimitedStreamFallsBackToPlainRequestWhichRetries() = runBlocking<Unit> {
        server.enqueue(json(429, errorBody("rate_limited"), "Retry-After" to "1"))
        server.enqueue(json(429, errorBody("rate_limited"), "Retry-After" to "1"))
        server.enqueue(json(200, result()))
        val seen = mutableListOf<GenerateProgress>()

        client.morphStreaming("x", "app1", 0, progress = { seen += it })

        assertEquals(listOf(GenerateProgress.Restart(0)), seen)
        assertEquals(3, server.requestCount)
    }

    @Test
    fun jsonResponseToStreamRequestIsAccepted() = runBlocking<Unit> {
        server.enqueue(json(200, result(version = 9)))
        val r = client.morphStreaming("x", "app1", 0, progress = {})
        assertEquals(9, r.document.version)
        assertEquals(1, server.requestCount)
    }

    @Test
    fun conflictRestartsAndRetriesWithoutBaseVersion() = runBlocking<Unit> {
        server.enqueue(json(409, errorBody("version_conflict", details = """{"currentVersion":5}""")))
        server.enqueue(sse("result" to result(version = 6)))
        val seen = mutableListOf<GenerateProgress>()

        val r = client.morphStreaming("x", "app1", currentVersion = 4, progress = { seen += it })

        assertEquals(6, r.document.version)
        assertEquals(listOf(GenerateProgress.Restart(0)), seen)
        assertEquals(JsonPrimitive(4), body(server.take())["baseVersion"])
        assertNull(body(server.take())["baseVersion"])
    }

    // MARK: generate(通常)

    @Test
    fun plainMorphSendsModeAndLocaleAndBuildsEnvelope() = runBlocking<Unit> {
        server.enqueue(json(200, result(version = 5)))
        val r = client.morph("メモ欄を消して", "app1", currentVersion = 4, locale = "ja", mode = GenerateMode.EDIT)

        val sent = body(server.take())
        assertEquals(JsonPrimitive("edit"), sent["mode"])
        assertEquals(JsonPrimitive("ja"), sent["locale"])
        val env = ApiJson.parseToJsonElement(r.envelopeJson).jsonObject
        assertEquals(JsonPrimitive("app1"), env["appId"])
        assertEquals(JsonPrimitive(5), env["version"])
        assertTrue(env["dsl"] is JsonObject)
        assertFalse(r.unchanged)
    }

    @Test
    fun plainMorphConflictRetriesOnce() = runBlocking<Unit> {
        server.enqueue(json(409, errorBody("version_conflict")))
        server.enqueue(json(409, errorBody("version_conflict")))
        val e = expectApiError { client.morph("x", "app1", currentVersion = 4) }
        assertEquals(409, e.status)
        assertEquals(2, server.requestCount)
    }

    @Test
    fun proRequiredIsTyped() = runBlocking<Unit> {
        server.enqueue(json(402, errorBody("pro_required", details = """{"features":[{"id":"serverRules","title":"自動の動き"}]}""")))
        val e = expectApiError { client.morph("x", "app1", 0) }
        assertEquals("serverRules", e.proRequired!!.features.single().id)
        assertFalse(e.proRequired!!.creatorNeedsPro)
    }

    // MARK: chat

    @Test
    fun chatSendsHistoryAndDecodesTurn() = runBlocking<Unit> {
        server.enqueue(
            json(
                200,
                """{"kind":"proposal","text":"こんな案は","turn":1,"mode":"edit","options":[{"title":"大きく","instruction":"大きくして"},{"id":"o2","title":"グラフ"}],"multiSelect":true,"future":1}""",
            ),
        )
        val turn = client.chat("app1", listOf(ChatMessage(ChatMessage.Role.USER, "見やすく")), "ja", ChatMode.EDIT)

        assertEquals(ChatTurn.Kind.PROPOSAL, turn.kind)
        assertEquals(ChatMode.EDIT, turn.resolvedMode)
        assertTrue(turn.isMultiSelect)
        assertEquals("大きく", turn.options!![0].optionId)
        assertEquals("グラフ", turn.options[1].instructionText)

        val sent = body(server.take())
        assertEquals("""[{"role":"user","text":"見やすく"}]""", sent["messages"].toString())
        assertEquals(JsonPrimitive("edit"), sent["mode"])
        assertEquals("""["answer","proposal"]""", sent["capabilities"].toString())
    }

    @Test
    fun unknownChatKindFallsBackToAnswer() = runBlocking<Unit> {
        server.enqueue(json(200, """{"kind":"poll","text":"?","turn":2}"""))
        assertEquals(ChatTurn.Kind.ANSWER, client.chat("app1", emptyList()).kind)
    }

    @Test
    fun resetChatSwallowsErrors() = runBlocking<Unit> {
        server.enqueue(json(403, errorBody("forbidden")))
        client.resetChat("app 1")
        val r = server.take()
        assertEquals("DELETE", r.method)
        assertEquals("/v1/chat/app%201", r.path)
    }
}
