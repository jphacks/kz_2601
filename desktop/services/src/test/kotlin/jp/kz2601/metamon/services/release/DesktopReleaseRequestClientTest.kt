package jp.kz2601.metamon.services.release

import jp.kz2601.metamon.services.http.ApiError
import jp.kz2601.metamon.services.http.ClientInfo
import jp.kz2601.metamon.services.http.MetamonHttp
import jp.kz2601.metamon.services.http.TokenProvider
import jp.kz2601.metamon.services.live.HttpReleaseRequestClient
import kotlinx.coroutines.runBlocking
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Assert.fail
import org.junit.Before
import org.junit.Test

/** リリース申請の API クライアントを android/services から取り込んだ物(#2803)。通信は MockWebServer だけ。 */
class DesktopReleaseRequestClientTest {
    private lateinit var server: MockWebServer
    private lateinit var client: HttpReleaseRequestClient

    @Before
    fun setUp() {
        server = MockWebServer().apply { start() }
        val tokens = object : TokenProvider {
            override suspend fun idToken(forceRefresh: Boolean): String? = "tok"
        }
        client = HttpReleaseRequestClient(MetamonHttp(server.url("/").toString(), tokens, ClientInfo("0.1.0", "1"), sleep = {}))
    }

    @After
    fun tearDown() {
        server.shutdown()
    }

    private fun json(body: String, code: Int = 200) = MockResponse().setResponseCode(code).setHeader("Content-Type", "application/json").setBody(body)

    @Test
    fun requestPostsPurposeAndContact() = runBlocking {
        server.enqueue(json("""{"id":"rr1","appId":"app1","name":"家計簿","dslVersion":3,"purpose":"配りたい","contact":"a@b","status":"received"}"""))
        val r = client.request("app1", "配りたい", "a@b")
        assertEquals("rr1", r.id)
        assertEquals(ReleaseStatus.RECEIVED, r.kind)

        val req = server.takeRequest()
        assertEquals("POST", req.method)
        assertEquals("/v1/apps/app1/release-requests", req.path)
        assertEquals("Bearer tok", req.getHeader("Authorization"))
        val body = req.body.readUtf8()
        assertTrue(body, body.contains("\"purpose\":\"配りたい\""))
        assertTrue(body, body.contains("\"contact\":\"a@b\""))
    }

    @Test
    fun noContactIsNotSent() = runBlocking {
        server.enqueue(json("""{"id":"rr1","appId":"app1"}"""))
        client.request("app1", "p", null)
        val body = server.takeRequest().body.readUtf8()
        // backend は未知・余計なキーを 400 にするので、contact が無ければキーごと送らない
        assertTrue(body, !body.contains("contact"))
    }

    @Test
    fun duplicateConflictIsThrown() = runBlocking {
        server.enqueue(json("""{"error":{"code":"conflict","message":"already requested","details":{"reason":"already_requested"}}}""", code = 409))
        try {
            client.request("app1", "p", null)
            fail("通ってしまった")
        } catch (e: ApiError) {
            assertEquals(409, e.status)
            assertEquals("conflict", e.code)
        }
        // POST は二重実行を避けて送り直さない
        assertEquals(1, server.requestCount)
    }

    @Test
    fun myRequestsReadsStatuses() = runBlocking {
        server.enqueue(
            json(
                """{"items":[
                  {"id":"rr3","appId":"a","status":"declined"},
                  {"id":"rr2","appId":"a","status":"reviewing"},
                  {"id":"rr1","appId":"b","status":"something_new"}
                ]}""",
            ),
        )
        val items = client.myRequests()
        val req = server.takeRequest()
        assertEquals("GET", req.method)
        assertEquals("/v1/release-requests", req.path)

        assertEquals(listOf(ReleaseStatus.DECLINED, ReleaseStatus.REVIEWING, ReleaseStatus.RECEIVED), items.map { it.kind })
        // 申請中があればそれを見せる。未知の状況は受付の扱い
        val latest = ReleaseRequest.latestByApp(items)
        assertEquals("rr2", latest.getValue("a").id)
        assertEquals("rr1", latest.getValue("b").id)
    }

    @Test
    fun emptyListIsEmpty() = runBlocking {
        server.enqueue(json("{}"))
        assertTrue(client.myRequests().isEmpty())
    }
}
