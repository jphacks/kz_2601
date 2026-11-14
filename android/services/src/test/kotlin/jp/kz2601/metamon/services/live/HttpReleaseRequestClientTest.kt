package jp.kz2601.metamon.services.live

import jp.kz2601.metamon.services.release.MockReleaseRequestClient
import jp.kz2601.metamon.services.release.ReleaseRequest
import jp.kz2601.metamon.services.release.ReleaseStatus
import jp.kz2601.metamon.services.http.json
import jp.kz2601.metamon.services.http.metamonHttp
import jp.kz2601.metamon.services.http.take
import kotlinx.coroutines.runBlocking
import okhttp3.mockwebserver.MockWebServer
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Before
import org.junit.Test

/** リリース申請の API の口(#1254・openapi-appconfig.yaml `release-requests`)。 */
class HttpReleaseRequestClientTest {
    private lateinit var server: MockWebServer
    private lateinit var client: HttpReleaseRequestClient

    @Before
    fun setUp() {
        server = MockWebServer().apply { start() }
        client = HttpReleaseRequestClient(server.metamonHttp())
    }

    @After
    fun tearDown() {
        server.shutdown()
    }

    @Test
    fun requestPostsPurposeAndContact() = runBlocking {
        server.enqueue(
            json(201, """{"id":"rr_1","appId":"app 1","name":"家計簿","dslVersion":3,"purpose":"毎朝","contact":"a@b","status":"received","createdAt":"2026-11-11T00:00:00Z"}"""),
        )
        val r = client.request("app 1", "毎朝", "a@b")
        assertEquals("rr_1", r.id)
        assertEquals(ReleaseStatus.RECEIVED, r.kind)
        val req = server.take()
        assertEquals("POST", req.method)
        assertEquals("/v1/apps/app%201/release-requests", req.path)
        assertEquals("""{"purpose":"毎朝","contact":"a@b"}""", req.body.readUtf8())
    }

    @Test
    fun requestWithoutContactOmitsTheKey() = runBlocking {
        server.enqueue(json(200, """{"id":"rr_1","appId":"a","name":"","dslVersion":1,"purpose":"p","status":"reviewing"}"""))
        assertEquals(ReleaseStatus.REVIEWING, client.request("a", "p", null).kind)
        assertEquals("""{"purpose":"p"}""", server.take().body.readUtf8())
    }

    @Test
    fun myRequestsReadsItems() = runBlocking {
        server.enqueue(
            json(200, """{"items":[{"id":"rr_2","appId":"a","name":"","dslVersion":1,"purpose":"p","status":"planned"},{"id":"rr_1","appId":"a","name":"","dslVersion":1,"purpose":"p","status":"oops"}]}"""),
        )
        val items = client.myRequests()
        assertEquals(listOf(ReleaseStatus.PLANNED, ReleaseStatus.RECEIVED), items.map { it.kind })
        assertEquals("/v1/release-requests", server.take().path)
    }

    @Test
    fun latestByAppPrefersAnOpenRequest() {
        val items = listOf(
            ReleaseRequest("3", "a", status = "declined"),
            ReleaseRequest("2", "a", status = "reviewing"),
            ReleaseRequest("1", "a", status = "received"),
            ReleaseRequest("4", "b", status = "declined"),
        )
        val map = ReleaseRequest.latestByApp(items)
        assertEquals("2", map["a"]?.id)
        assertEquals("4", map["b"]?.id)
    }

    @Test
    fun mockReturnsTheOpenRequestAgain() = runBlocking {
        val mock = MockReleaseRequestClient { "家計簿" }
        val first = mock.request("a", " 毎朝 ", " ")
        assertEquals("毎朝", first.purpose)
        assertEquals("家計簿", first.name)
        assertEquals(first, mock.request("a", "別の", null))
        assertEquals(listOf(first), mock.myRequests())
    }
}
