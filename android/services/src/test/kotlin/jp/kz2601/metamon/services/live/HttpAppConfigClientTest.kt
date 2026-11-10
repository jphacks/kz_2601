package jp.kz2601.metamon.services.live

import jp.kz2601.metamon.services.RecordQuery
import jp.kz2601.metamon.services.http.ApiJson
import jp.kz2601.metamon.services.http.FailureKind
import jp.kz2601.metamon.services.http.MINI_DSL
import jp.kz2601.metamon.services.http.errorBody
import jp.kz2601.metamon.services.http.expectApiError
import jp.kz2601.metamon.services.http.json
import jp.kz2601.metamon.services.http.metamonHttp
import jp.kz2601.metamon.services.http.take
import kotlinx.coroutines.runBlocking
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.put
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import okhttp3.mockwebserver.SocketPolicy
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test

class HttpAppConfigClientTest {
    private lateinit var server: MockWebServer
    private lateinit var client: HttpAppConfigClient

    @Before
    fun setUp() {
        server = MockWebServer().apply { start() }
        client = HttpAppConfigClient(server.metamonHttp())
    }

    @After
    fun tearDown() {
        server.shutdown()
    }

    private fun envelope(version: Int) = """{"appId":"app1","version":$version,"dsl":$MINI_DSL,"source":{"kind":"ai"},"createdAt":"2026-11-11T00:00:00Z"}"""
    private fun record(id: String) = """{"id":"$id","entity":"expense","data":{"amount":1200},"createdAt":"x","updatedAt":"x"}"""
    private fun app(id: String) = """{"id":"$id","name":"$id","currentVersion":2,"status":"active","createdAt":"x","updatedAt":"x"}"""

    // MARK: DSL と ETag

    @Test
    fun fetchDocumentKeepsEnvelope() = runBlocking<Unit> {
        server.enqueue(json(200, envelope(7), "ETag" to "\"7\""))
        val doc = client.fetchDocument("app1")
        assertEquals(7, doc.version)
        assertEquals("家計簿", doc.document.dsl.app.name)
        assertTrue(doc.envelopeJson.contains("\"source\""))
        assertEquals("/v1/apps/app1/dsl", server.take().path)
    }

    @Test
    fun fetchIfChangedSendsIfNoneMatchAndTreats304AsUnchanged() = runBlocking<Unit> {
        server.enqueue(MockResponse().setResponseCode(304))
        assertNull(client.fetchDocumentIfChanged("app1", currentVersion = 7))
        assertEquals("\"7\"", server.take().getHeader("If-None-Match"))
    }

    @Test
    fun fetchIfChangedReturnsNewerVersionOnly() = runBlocking<Unit> {
        server.enqueue(json(200, envelope(8)))
        server.enqueue(json(200, envelope(7)))
        assertEquals(8, client.fetchDocumentIfChanged("app1", 7)?.version)
        assertNull(client.fetchDocumentIfChanged("app1", 7))
    }

    @Test
    fun putDocumentSendsIfMatchAndDsl() = runBlocking<Unit> {
        server.enqueue(json(201, """{"appId":"app1","version":8,"parentVersion":7,"createdAt":"x"}""", "ETag" to "\"8\""))
        val dsl = ApiJson.parseToJsonElement(MINI_DSL).jsonObject
        val v = client.putDocument("app1", dsl, baseVersion = 7, source = buildJsonObject { put("kind", "manual") })

        assertEquals(8, v.version)
        val req = server.take()
        assertEquals("PUT", req.method)
        assertEquals("\"7\"", req.getHeader("If-Match"))
        val sent = ApiJson.parseToJsonElement(req.body.readUtf8()).jsonObject
        assertEquals(dsl, sent["dsl"])
        assertEquals(JsonPrimitive("manual"), (sent["source"] as JsonObject)["kind"])
    }

    @Test
    fun putDocumentConflictExposesCurrentVersion() = runBlocking<Unit> {
        server.enqueue(json(409, errorBody("version_conflict", "current version is 9", """{"currentVersion":9}""")))
        val e = expectApiError { client.putDocument("app1", JsonObject(emptyMap()), 7) }
        assertTrue(e.isVersionConflict)
        assertEquals(9, e.currentVersion)
        assertEquals(FailureKind.Conflict(9), FailureKind.classify(e))
        assertEquals(1, server.requestCount) // PUT はやり直さない
    }

    @Test
    fun putDocumentPreconditionRequired() = runBlocking<Unit> {
        server.enqueue(json(412, errorBody("bad_request", "If-Match header is required")))
        val e = expectApiError { client.putDocument("app1", JsonObject(emptyMap()), 7) }
        assertTrue(e.isPreconditionRequired)
        assertTrue(FailureKind.classify(e) is FailureKind.Rejected)
    }

    // MARK: アプリ

    @Test
    fun listAppsFollowsPageTokens() = runBlocking<Unit> {
        server.enqueue(json(200, """{"items":[${app("a")}],"nextPageToken":"p2"}"""))
        server.enqueue(json(200, """{"items":[${app("b")}]}"""))
        assertEquals(listOf("a", "b"), client.listApps().map { it.id })
        assertEquals("/v1/apps?pageSize=200", server.take().path)
        assertEquals("/v1/apps?pageSize=200&pageToken=p2", server.take().path)
    }

    @Test
    fun appLimitReachedOnCreateIsTyped() = runBlocking<Unit> {
        server.enqueue(json(403, errorBody("plan_limit_reached", details = """{"resource":"apps","plan":"free","limit":3,"upgrade":true}""")))
        val e = expectApiError { client.createApp(name = "新しい") }
        val kind = FailureKind.classify(e) as FailureKind.PlanLimitReached
        assertEquals(3, kind.info.limit)
        assertEquals("""{"name":"新しい"}""", server.take().body.readUtf8())
    }

    @Test
    fun duplicateDeleteReorder() = runBlocking<Unit> {
        server.enqueue(json(201, """{"id":"copy","name":"コピー","currentVersion":2,"status":"active","dsl":$MINI_DSL}"""))
        server.enqueue(MockResponse().setResponseCode(204))
        server.enqueue(MockResponse().setResponseCode(204))

        assertEquals("copy", client.duplicateApp("app1", "コピー").summary.id)
        client.deleteApp("app1")
        client.reorderApps(listOf("b", "a"))

        val dup = server.take()
        assertEquals("/v1/apps/app1/duplicate", dup.path)
        assertEquals("""{"name":"コピー"}""", dup.body.readUtf8())
        assertEquals("DELETE", server.take().method)
        val order = server.take()
        assertEquals("/v1/apps/order", order.path)
        assertEquals("""{"ids":["b","a"]}""", order.body.readUtf8())
    }

    // MARK: 記録

    @Test
    fun queryRecordsPostsFilterInBody() = runBlocking<Unit> {
        server.enqueue(json(200, """{"items":[${record("r1")}],"nextPageToken":"n"}"""))
        val page = client.queryRecords("app1", "expense", RecordQuery(filterField = "author", filterOp = "eq", filterValue = "uid-1"))

        assertEquals("r1", page.items.single().id)
        assertEquals("n", page.nextPageToken)
        val req = server.take()
        assertEquals("POST", req.method)
        assertEquals("/v1/apps/app1/entities/expense/records/query", req.path)
        assertEquals("""{"filterField":"author","filterOp":"eq","filterValue":"uid-1"}""", req.body.readUtf8())
    }

    @Test
    fun queryAllRecordsPagesWithToken() = runBlocking<Unit> {
        server.enqueue(json(200, """{"items":[${record("r1")}],"nextPageToken":"n"}"""))
        server.enqueue(json(200, """{"items":[${record("r2")}]}"""))
        assertEquals(listOf("r1", "r2"), client.queryAllRecords("app1", "expense").map { it.id })
        assertEquals("""{"pageSize":200}""", server.take().body.readUtf8())
        assertEquals("""{"pageSize":200,"pageToken":"n"}""", server.take().body.readUtf8())
    }

    @Test
    fun createRecordResendsSameClientIdAfterConnectionDrop() = runBlocking<Unit> {
        server.enqueue(MockResponse().setSocketPolicy(SocketPolicy.DISCONNECT_AFTER_REQUEST))
        server.enqueue(json(201, record("r1")))

        val r = client.createRecord("app1", "expense", buildJsonObject { put("amount", 1200) }, clientId = "cid-1")

        assertEquals("r1", r.id)
        val first = server.take().body.readUtf8()
        val second = server.take().body.readUtf8()
        assertEquals(first, second)
        assertTrue(first.contains("\"clientId\":\"cid-1\""))
    }

    @Test
    fun setMarkUsesPutAndDelete() = runBlocking<Unit> {
        server.enqueue(json(200, """{"count":3,"mine":true}"""))
        server.enqueue(json(200, """{"count":2,"mine":false}"""))
        assertEquals(3, client.setMark("app1", "posts", "r1", "like", on = true).count)
        assertEquals(false, client.setMark("app1", "posts", "r1", "like", on = false).mine)
        assertEquals("PUT", server.take().method)
        assertEquals("DELETE", server.take().method)
    }

    // MARK: 共有

    @Test
    fun shareAndImport() = runBlocking<Unit> {
        server.enqueue(json(201, """{"token":"tok","appId":"app1","name":"家計簿","dslVersion":3,"url":"tailorit://import/tok","webUrl":"https://x/s/tok"}"""))
        server.enqueue(json(201, """{"id":"app1","name":"家計簿","currentVersion":2,"status":"active","dsl":$MINI_DSL,"created":true,"joined":true,"member":true}"""))

        assertEquals("tok", client.share("app1").token)
        val installed = client.importShare("tok", displayName = "たろう")
        assertTrue(installed.joined)
        assertTrue(installed.member)

        assertEquals("/v1/apps/app1/share", server.take().path)
        val imp = server.take()
        assertEquals("/v1/shares/tok/import", imp.path)
        assertEquals("""{"displayName":"たろう"}""", imp.body.readUtf8())
    }
}
