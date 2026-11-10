package jp.kz2601.metamon.services.live

import jp.kz2601.metamon.services.http.MINI_DSL
import jp.kz2601.metamon.services.http.errorBody
import jp.kz2601.metamon.services.http.expectApiError
import jp.kz2601.metamon.services.http.json
import jp.kz2601.metamon.services.http.metamonHttp
import jp.kz2601.metamon.services.local.DocumentCache
import kotlinx.coroutines.runBlocking
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Before
import org.junit.Rule
import org.junit.Test
import org.junit.rules.TemporaryFolder

/** 生成の結果も DocumentCache に入れる(#1020・iOS の `GenerateStream.finish`)。 */
class HttpAIClientCacheTest {
    @get:Rule
    val tmp = TemporaryFolder()

    private lateinit var server: MockWebServer
    private lateinit var dir: java.io.File
    private lateinit var client: HttpAIClient

    @Before
    fun setUp() {
        server = MockWebServer().apply { start() }
        dir = tmp.newFolder()
        client = HttpAIClient(server.metamonHttp(), cache = DocumentCache(dir))
    }

    @After
    fun tearDown() {
        server.shutdown()
    }

    private fun result(version: Int, appId: String = "app1") =
        """{"appId":"$appId","version":$version,"mode":"full","dsl":$MINI_DSL,"summary":"s"}"""

    private fun sseResult(version: Int) = MockResponse().setResponseCode(200)
        .addHeader("Content-Type", "text/event-stream")
        .setBody("event: result\ndata: ${result(version)}\n\n")

    @Test
    fun plainMorphStoresEnvelopeThatSurvivesRestart() = runBlocking<Unit> {
        server.enqueue(json(200, result(version = 5)))
        client.morph("x", "app1", 0)

        val loaded = DocumentCache(dir).load("app1")!! // 別インスタンス = 再起動後(機内モードの起動)
        assertEquals(5, loaded.document.version)
        assertEquals("app1", loaded.document.appId)
    }

    @Test
    fun streamingMorphStoresUnderServerAppId() = runBlocking<Unit> {
        server.enqueue(sseResult(version = 7))
        client.morphStreaming("x", "app1", 0, progress = {})

        assertEquals(7, DocumentCache(dir).load("app1")!!.document.version)
    }

    @Test
    fun conflictRetryStoresOnlyFinalResult() = runBlocking<Unit> {
        server.enqueue(json(409, errorBody("version_conflict")))
        server.enqueue(json(200, result(version = 6)))
        client.morph("x", "app1", currentVersion = 4)

        assertEquals(6, DocumentCache(dir).load("app1")!!.document.version)
    }

    @Test
    fun failedGenerateKeepsPreviousEnvelope() = runBlocking<Unit> {
        server.enqueue(json(200, result(version = 3)))
        client.morph("x", "app1", 0)
        server.enqueue(json(422, errorBody("generation_failed")))
        expectApiError { client.morph("y", "app1", 3) }

        assertEquals(3, DocumentCache(dir).load("app1")!!.document.version)
    }

    @Test
    fun withoutCacheNothingIsWritten() = runBlocking<Unit> {
        server.enqueue(json(200, result(version = 2)))
        HttpAIClient(server.metamonHttp()).morph("x", "app1", 0)

        assertNull(DocumentCache(dir).load("app1"))
    }
}
