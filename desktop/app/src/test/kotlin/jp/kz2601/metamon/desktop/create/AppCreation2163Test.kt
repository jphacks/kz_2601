package jp.kz2601.metamon.desktop.create

import jp.kz2601.metamon.desktop.chat.ChatBuild
import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.services.http.ApiJson
import jp.kz2601.metamon.services.http.ClientInfo
import jp.kz2601.metamon.services.http.DeviceContext
import jp.kz2601.metamon.services.http.FailureKind
import jp.kz2601.metamon.services.http.MetamonHttp
import jp.kz2601.metamon.services.http.TokenProvider
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.runBlocking
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertSame
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test
import java.io.IOException
import java.net.InetAddress
import java.util.concurrent.TimeUnit

/** 作る流れ(#2163): 作り物のストリーム(途中・完成・失敗・切断)を流し、表示の状態の移り変わりを見る。HTTP は MockWebServer だけ。 */
class AppCreation2163Test {
    private lateinit var server: MockWebServer

    @Before
    fun setUp() {
        server = MockWebServer().apply { start() }
    }

    @After
    fun tearDown() {
        server.shutdown()
    }

    private fun lines(name: String): List<String> =
        javaClass.getResourceAsStream("/generate-stream/$name.sse")!!.bufferedReader(Charsets.UTF_8).readLines()

    /** 行を流す作り物。[failAfter] 行を流した後に通信が切れる。 */
    private fun fake(all: List<String>, failAfter: Int? = null) = GenerateSource { _, onLine ->
        all.forEachIndexed { i, l ->
            if (i == failAfter) throw IOException("connection reset")
            if (onLine(l)) return@GenerateSource
        }
    }

    /** 1 行ごとに描いている値を残す(変わった時だけ)。 */
    private fun recording(source: GenerateSource, creation: () -> AppCreation, seen: MutableList<AppDSL>) = GenerateSource { r, onLine ->
        source.stream(r) { l ->
            val stop = onLine(l)
            creation().state.value.preview?.let { if (seen.lastOrNull() != it) seen += it }
            stop
        }
    }

    private fun build() = ChatBuild("家計簿", appId = "app1")

    @Test
    fun streamGrowsThenCompletes() = runBlocking {
        val seen = mutableListOf<AppDSL>()
        lateinit var c: AppCreation
        c = AppCreation(recording(fake(lines("ok")), { c }, seen), build(), "app1")
        assertTrue(c.state.value.isBuilding)

        val end = c.run()

        // 名前 → 部品 1 つ → 2 つ。完成したら結果の DSL に置き換わる
        assertEquals(3, seen.size)
        assertEquals(listOf(0, 1, 2), seen.map { d -> d.screens.first { it.id == d.initialScreen }.components.size })
        assertEquals(CreatePhase.DONE, end.phase)
        assertEquals(2, end.result!!.document.version)
        assertEquals(end.result!!.document.dsl, end.preview)
        assertFalse(end.canRetry)
    }

    @Test
    fun brokenStreamKeepsLastDrawableThenFails() = runBlocking {
        val c = AppCreation(fake(lines("broken")), build(), "app1")
        val end = c.run()
        assertEquals(CreatePhase.FAILED, end.phase)
        assertTrue("${end.failure}", end.failure is FailureKind.Busy)
        assertTrue(end.canRetry)
        // 壊れた行の前に描けた画面は残る
        assertNotNull(end.preview)
    }

    @Test
    fun cutStreamOffersRetry() = runBlocking {
        val c = AppCreation(fake(lines("cut")), build(), "app1")
        val end = c.run()
        assertEquals(CreatePhase.CUT, end.phase)
        assertNotNull(end.preview)
        assertTrue(end.canRetry)
    }

    @Test
    fun disconnectAfterSomeLinesIsCut() = runBlocking {
        val c = AppCreation(fake(lines("ok"), failAfter = 5), build(), "app1")
        val end = c.run()
        assertEquals(CreatePhase.CUT, end.phase)
        assertEquals("家計簿", end.preview?.app?.name)
    }

    @Test
    fun disconnectBeforeAnythingIsOffline() = runBlocking {
        val c = AppCreation(fake(lines("ok"), failAfter = 0), build(), "app1")
        val end = c.run()
        assertEquals(CreatePhase.FAILED, end.phase)
        assertEquals(FailureKind.Offline, end.failure)
        assertTrue(end.canRetry)
        assertNull(end.preview)
    }

    @Test
    fun retryKeepsPreviewUntilNextValue() = runBlocking {
        var calls = 0
        lateinit var c: AppCreation
        var previewAtRetry: AppDSL? = null
        val source = GenerateSource { r, onLine ->
            calls++
            if (calls == 2) previewAtRetry = c.state.value.preview
            fake(lines(if (calls == 1) "cut" else "ok")).stream(r, onLine)
        }
        c = AppCreation(source, build(), "app1")
        assertEquals(CreatePhase.CUT, c.run().phase)
        val shown = c.state.value.preview
        assertEquals(CreatePhase.DONE, c.run().phase)
        assertEquals(shown, previewAtRetry)
    }

    @Test
    fun creatorAddsAppAndOpensItsWindow() = runBlocking {
        val opened = mutableListOf<String>()
        val cached = mutableListOf<String>()
        val creator = AppCreator(fake(lines("ok")), this, cache = { cached += it.document.appId }, onOpen = { opened += it })

        assertTrue(creator.start(build()))
        assertNotNull(creator.current)
        creator.job!!.join()

        assertEquals(listOf("app1"), opened)
        assertEquals(listOf("app1"), cached)
        assertEquals(listOf("app1" to "家計簿"), creator.created.all.toList())
        assertNotNull(creator.created.dsl("app1"))
        assertNull(creator.current)
    }

    @Test
    fun creatorAsksBeforeClosingWhileBuilding() = runBlocking {
        val gate = CompletableDeferred<Unit>()
        val opened = mutableListOf<String>()
        val source = GenerateSource { r, onLine ->
            gate.await()
            fake(lines("ok")).stream(r, onLine)
        }
        val creator = AppCreator(source, this, onOpen = { opened += it })
        assertTrue(creator.start(build()))
        kotlinx.coroutines.yield()
        // 作っている間は重ねて始めない
        assertFalse(creator.start(build()))

        creator.requestClose()
        assertTrue(creator.confirmingClose)
        assertNotNull(creator.current)
        creator.keepBuilding()
        assertFalse(creator.confirmingClose)

        creator.requestClose()
        creator.stop()
        assertNull(creator.current)
        assertTrue(creator.job!!.isCancelled)
        gate.complete(Unit)
        creator.job!!.join()
        assertTrue(opened.isEmpty())
        assertTrue(creator.created.all.isEmpty())
    }

    @Test
    fun creatorRetriesAfterCutAndClosesWithoutConfirm() = runBlocking {
        var calls = 0
        val source = GenerateSource { r, onLine -> fake(lines(if (++calls == 1) "cut" else "ok")).stream(r, onLine) }
        val opened = mutableListOf<String>()
        val creator = AppCreator(source, this, onOpen = { opened += it })
        creator.start(build())
        creator.job!!.join()
        val c = creator.current!!
        assertEquals(CreatePhase.CUT, c.state.value.phase)

        assertTrue(creator.retry())
        creator.job!!.join()
        assertEquals(listOf("app1"), opened)

        // 作っていない時の「閉じる」は確認なし
        creator.start(ChatBuild("x", appId = "app1"))
        creator.job!!.join()
        creator.requestClose()
        assertFalse(creator.confirmingClose)
        assertNull(creator.current)
    }

    // --- HTTP(MockWebServer) ---

    private fun http(): MetamonHttp = MetamonHttp(
        baseUrl = server.url("/").newBuilder().host(InetAddress.getByName(server.hostName).hostAddress).build().toString(),
        tokens = TokenProvider { "token" },
        clientInfo = ClientInfo(version = "0.1.0", build = "1"),
        client = MetamonHttp.defaultClient().newBuilder().retryOnConnectionFailure(false).build(),
        device = DeviceContext(timeZone = { "Asia/Tokyo" }, language = { "ja" }),
        sleep = {},
    )

    @Test
    fun httpStreamCompletes() = runBlocking {
        server.enqueue(MockResponse().setHeader("Content-Type", "text/event-stream").setBody(lines("ok").joinToString("\n") + "\n"))
        val c = AppCreation(HttpGenerateSource(http()), ChatBuild("家計簿を作りたい"), "app1", locale = { "ja" })

        assertEquals(CreatePhase.DONE, c.run().phase)

        val req = server.takeRequest(5, TimeUnit.SECONDS)!!
        assertEquals("/v1/generate?stream=1", req.path)
        assertEquals("text/event-stream", req.getHeader("Accept"))
        val body = ApiJson.parseToJsonElement(req.body.readUtf8()).jsonObject
        assertEquals("app1", body["appId"]!!.jsonPrimitive.content)
        assertEquals("家計簿を作りたい", body["transcript"]!!.jsonPrimitive.content)
        assertEquals("auto", body["mode"]!!.jsonPrimitive.content)
        assertNull(body["baseVersion"])
    }

    @Test
    fun httpJsonAnswerStillCompletes() = runBlocking {
        val result = lines("ok").first { it.startsWith("data: {\"appId\"") }.removePrefix("data: ")
        server.enqueue(MockResponse().setHeader("Content-Type", "application/json").setBody(result.replace(",", ",\n")))
        val c = AppCreation(HttpGenerateSource(http()), build(), "app1")
        val end = c.run()
        assertEquals(CreatePhase.DONE, end.phase)
        assertEquals("家計簿を作りました", end.result!!.summary)
    }

    @Test
    fun httpErrorFails() = runBlocking {
        server.enqueue(
            MockResponse().setResponseCode(503).setHeader("Content-Type", "application/json")
                .setBody("""{"error":{"code":"unavailable","message":"busy"}}"""),
        )
        val end = AppCreation(HttpGenerateSource(http()), build(), "app1").run()
        assertEquals(CreatePhase.FAILED, end.phase)
        assertTrue("${end.failure}", end.failure is FailureKind.Busy)
    }

    @Test
    fun noConsentSendsNothing() = runBlocking {
        val end = AppCreation(HttpGenerateSource(http(), isGranted = { false }), build(), "app1").run()
        assertEquals(CreatePhase.FAILED, end.phase)
        assertEquals(0, server.requestCount)
    }

    @Test
    fun createdAppsReplaceSameId() {
        val apps = CreatedApps()
        val a = jp.kz2601.metamon.desktop.Examples.load("shopping")
        apps.add("x", a)
        apps.add("x", a)
        assertEquals(1, apps.all.size)
        assertSame(a, apps.dsl("x"))
    }
}
