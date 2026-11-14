package jp.kz2601.metamon.desktop.services

import jp.kz2601.metamon.desktop.net.ClientPlatform
import jp.kz2601.metamon.desktop.net.DesktopApi
import jp.kz2601.metamon.services.http.ApiError
import jp.kz2601.metamon.services.live.HttpModerationClient
import jp.kz2601.metamon.services.social.BlockedUser
import jp.kz2601.metamon.services.social.ReportReason
import jp.kz2601.metamon.services.social.ReportTarget
import kotlinx.coroutines.runBlocking
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test

/** 通報・ブロックの API クライアントを android/services から取り込んだもの(#2805)。通信は MockWebServer だけ。 */
class HttpModerationClientTest {
    private lateinit var server: MockWebServer

    @Before
    fun setUp() {
        server = MockWebServer().apply { start() }
    }

    @After
    fun tearDown() {
        server.shutdown()
    }

    private fun client(): HttpModerationClient = HttpModerationClient(
        DesktopApi.http(
            baseUrl = server.url("/").toString(),
            tokens = DesktopApi.devTokens("dev-token"),
            platform = ClientPlatform.LINUX,
            client = DesktopApi.okHttp(ClientPlatform.LINUX, "ua"),
        ),
    )

    private fun json(body: String, code: Int = 200) =
        MockResponse().setResponseCode(code).setHeader("Content-Type", "application/json").setBody(body)

    private fun bodyOf(raw: String) = Json.parseToJsonElement(raw).jsonObject

    @Test
    fun reportsRecord() = runBlocking {
        server.enqueue(json("{}"))
        client().report(ReportTarget.RecordTarget("app1", "posts", "r1"), ReportReason.SPAM)

        val req = server.takeRequest()
        assertEquals("POST", req.method)
        assertEquals("/v1/reports", req.path)
        val body = bodyOf(req.body.readUtf8())
        assertEquals("record", body["kind"]!!.jsonPrimitive.content)
        assertEquals("app1", body["appId"]!!.jsonPrimitive.content)
        assertEquals("posts", body["entityId"]!!.jsonPrimitive.content)
        assertEquals("r1", body["recordId"]!!.jsonPrimitive.content)
        assertEquals("spam", body["reason"]!!.jsonPrimitive.content)
    }

    @Test
    fun reportsListing() = runBlocking {
        server.enqueue(json("{}"))
        client().report(ReportTarget.Listing("L1"), ReportReason.OTHER)

        val body = bodyOf(server.takeRequest().body.readUtf8())
        assertEquals("listing", body["kind"]!!.jsonPrimitive.content)
        assertEquals("L1", body["id"]!!.jsonPrimitive.content)
        assertEquals("other", body["reason"]!!.jsonPrimitive.content)
        assertFalse(body.containsKey("appId"))
    }

    @Test
    fun blocksWithNameCutTo64() = runBlocking {
        server.enqueue(json("""{"uid":"u1","name":"太郎"}"""))
        val blocked = client().block("u1", "あ".repeat(80))

        assertEquals(BlockedUser("u1", "太郎"), blocked)
        val req = server.takeRequest()
        assertEquals("POST", req.method)
        assertEquals("/v1/me/blocks", req.path)
        val body = bodyOf(req.body.readUtf8())
        assertEquals("u1", body["uid"]!!.jsonPrimitive.content)
        assertEquals(64, body["name"]!!.jsonPrimitive.content.length)
    }

    @Test
    fun blocksWithoutEmptyName() = runBlocking {
        server.enqueue(json("""{"uid":"u1"}"""))
        client().block("u1", "")

        assertFalse(bodyOf(server.takeRequest().body.readUtf8()).containsKey("name"))
    }

    @Test
    fun blocksListing() = runBlocking {
        server.enqueue(json("""{"uid":"u9","name":"出品者"}"""))
        val blocked = client().blockListing("L1")

        assertEquals("u9", blocked.uid)
        val body = bodyOf(server.takeRequest().body.readUtf8())
        assertEquals("L1", body["listingId"]!!.jsonPrimitive.content)
    }

    @Test
    fun listsBlockedUsers() = runBlocking {
        server.enqueue(json("""{"items":[{"uid":"u1","name":"太郎"},{"uid":"u2"}],"extra":1}"""))
        val users = client().blockedUsers()

        assertEquals(listOf(BlockedUser("u1", "太郎"), BlockedUser("u2", "")), users)
        val req = server.takeRequest()
        assertEquals("GET", req.method)
        assertEquals("/v1/me/blocks", req.path)
    }

    @Test
    fun unblocksWithEscapedUid() = runBlocking {
        server.enqueue(MockResponse().setResponseCode(204))
        client().unblock("u/1")

        val req = server.takeRequest()
        assertEquals("DELETE", req.method)
        assertEquals("/v1/me/blocks/u%2F1", req.path)
    }

    @Test
    fun serverErrorThrows() = runBlocking {
        server.enqueue(json("""{"error":{"code":"forbidden","message":"no"}}""", code = 403))
        val failure = runCatching { client().report(ReportTarget.Listing("L1"), ReportReason.SPAM) }.exceptionOrNull()
        assertTrue(failure is ApiError)
    }
}
