package jp.kz2601.metamon.services.sharing

import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.decode
import jp.kz2601.metamon.services.Share
import jp.kz2601.metamon.services.http.errorBody
import jp.kz2601.metamon.services.http.json
import jp.kz2601.metamon.services.http.metamonHttp
import jp.kz2601.metamon.services.http.take
import jp.kz2601.metamon.services.live.HttpAppConfigClient
import kotlinx.coroutines.runBlocking
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test

class ShareSheetModelTest {
    private lateinit var server: MockWebServer
    private lateinit var client: HttpAppConfigClient
    private val issued = IssuedShares()

    @Before
    fun setUp() {
        server = MockWebServer().apply { start() }
        client = HttpAppConfigClient(server.metamonHttp())
    }

    @After
    fun tearDown() {
        server.shutdown()
    }

    private fun shareJson(token: String) =
        """{"token":"$token","appId":"app1","name":"家計簿","dslVersion":3,"url":"tailorit://import/$token","webUrl":"https://appconfig.example/s/$token","revoked":false}"""

    private fun dsl(vararg scopes: String): AppDSL = AppDSL.decode(
        """{"schemaVersion":1,"app":{"name":"家計簿"},"initialScreen":"home","screens":[{"id":"home","title":"ホーム","components":[]}],
           "entities":[${scopes.mapIndexed { i, s -> """{"id":"e$i","name":"E$i","scope":"$s","fields":[]}""" }.joinToString(",")}]}""",
    )

    private fun model(dsl: AppDSL? = dsl("private")) = ShareSheetModel("app1", client, issued) { dsl }

    // MARK: 発行

    @Test
    fun loadIssuesLinkAndKeepsItForNextOpen() = runBlocking<Unit> {
        server.enqueue(json(201, shareJson("tok1")))
        val m = model()
        m.load()
        val s = m.state.value
        assertEquals("tok1", s.share?.token)
        assertEquals(ShareScope.COPY, s.scope)
        val req = server.take()
        assertEquals("POST", req.method)
        assertEquals("/v1/apps/app1/share", req.path)

        // 開き直してもトークンを増やさない
        val again = model()
        again.load()
        assertEquals("tok1", again.state.value.share?.token)
        assertEquals(1, server.requestCount)
    }

    @Test
    fun notMemberIsShownAsItsOwnFailure() = runBlocking<Unit> {
        server.enqueue(json(403, errorBody("forbidden", details = """{"reason":"not_member"}""")))
        val m = model()
        m.load()
        assertNull(m.state.value.share)
        assertEquals(ShareFailure.NOT_MEMBER, m.state.value.failure)
    }

    @Test
    fun blankAppAndOfflineFailures() = runBlocking<Unit> {
        server.enqueue(json(400, errorBody("bad_request")))
        val m = model()
        m.issue()
        assertEquals(ShareFailure.BLANK_APP, m.state.value.failure)

        // 圏外(つながらない)
        server.shutdown()
        m.issue()
        assertEquals(ShareFailure.OFFLINE, m.state.value.failure)
    }

    @Test
    fun retryAfterFailureIssues() = runBlocking<Unit> {
        server.enqueue(json(404, errorBody("not_found")))
        server.enqueue(json(201, shareJson("tok2")))
        val m = model()
        m.load()
        assertEquals(ShareFailure.OTHER, m.state.value.failure)
        m.issue()
        assertEquals("tok2", m.state.value.share?.token)
        assertNull(m.state.value.failure)
    }

    @Test
    fun withoutServerSharingIsUnavailable() = runBlocking<Unit> {
        val m = ShareSheetModel("app1", null, issued) { null }
        m.load()
        assertEquals(ShareFailure.UNAVAILABLE, m.state.value.failure)
        assertEquals(0, server.requestCount)
    }

    // MARK: 取り消し

    @Test
    fun revokeDeletesTokenAndPreparesNewLink() = runBlocking<Unit> {
        server.enqueue(json(201, shareJson("old")))
        server.enqueue(MockResponse().setResponseCode(204))
        server.enqueue(json(201, shareJson("new")))
        val m = model()
        m.load()
        m.revoke()

        assertEquals("/v1/apps/app1/share", server.take().path)
        val del = server.take()
        assertEquals("DELETE", del.method)
        assertEquals("/v1/shares/old", del.path)
        assertEquals("/v1/apps/app1/share", server.take().path)
        assertEquals("new", m.state.value.share?.token)
        assertEquals(ShareNotice.REVOKED, m.state.value.notice)
        assertEquals("new", issued["app1"]?.token)

        m.consumeNotice()
        assertNull(m.state.value.notice)
    }

    @Test
    fun revokeFailureKeepsLink() = runBlocking<Unit> {
        server.enqueue(json(201, shareJson("tok")))
        server.enqueue(json(403, errorBody("forbidden")))
        val m = model()
        m.load()
        m.revoke()
        assertEquals("tok", m.state.value.share?.token)
        assertEquals(ShareNotice.REVOKE_FAILED, m.state.value.notice)
        assertEquals(false, m.state.value.busy)
        assertEquals("tok", issued["app1"]?.token)
    }

    // MARK: 範囲

    @Test
    fun scopeFollowsEntities() {
        assertEquals(ShareScope.COPY, ShareScope.of(dsl()))
        assertEquals(ShareScope.COPY, ShareScope.of(dsl("private")))
        assertEquals(ShareScope.PUBLIC, ShareScope.of(dsl("private", "public")))
        assertEquals(ShareScope.MEMBERS, ShareScope.of(dsl("public", "shared")))
    }

    @Test
    fun unreadableDslLeavesScopeUnknown() = runBlocking<Unit> {
        server.enqueue(json(201, shareJson("tok")))
        val m = ShareSheetModel("app1", client, issued) { error("offline") }
        m.load()
        assertEquals("tok", m.state.value.share?.token)
        assertNull(m.state.value.scope)
    }

    // MARK: リンクの形

    @Test
    fun webLinkForms() {
        // iOS の SharingTests.testShareURLTokenFromUniversalLink と同じ例
        assertEquals("AbC_12-x", ShareLinks.token("https://tailor-it.app/s/AbC_12-x"))
        assertEquals("tok", ShareLinks.token("https://Tailor-It.app/s/tok"))
        assertEquals("tok", ShareLinks.token("HTTPS://tailor-it.app/s/tok"))
        assertEquals("tok", ShareLinks.token("https://tailor-it.app/s/tok/"))
        assertEquals("tok", ShareLinks.token("https://tailor-it.app/s/tok?utm_source=x"))
        assertEquals("tok", ShareLinks.token("https://tailor-it.app/s/tok#x"))
        listOf(
            "https://tailor-it.app/s/",
            "https://tailor-it.app/s",
            "https://tailor-it.app/s/tok/extra",
            "https://tailor-it.app/terms",
            "https://tailor-it.app/x/tok",
            "https://tailor-it.app:8443/s/tok",
            "http://tailor-it.app/s/tok",
            "https://www.tailor-it.app/s/tok",
            "https://tailor-it.app.evil.example/s/tok",
            "https://user@tailor-it.app/s/tok",
            "https://evil.example/s/tok",
            "https://tailor-it.app/s/a%2Fb",
            "https://tailor-it.app/s/a%20b",
            "https://tailor-it.app/s/..",
        ).forEach { assertNull(it, ShareLinks.token(it)) }
    }

    @Test
    fun linkForms() {
        assertEquals("abc-_9", ShareLinks.token("tailorit://import/abc-_9"))
        assertEquals("abc", ShareLinks.token("TAILORIT://Import/abc"))
        assertNull(ShareLinks.token("https://example.com/s/abc"))
        assertNull(ShareLinks.token("tailorit://other/abc"))
        assertNull(ShareLinks.token("tailorit://import/"))
        assertNull(ShareLinks.token("tailorit://import/a b"))
        assertNull(ShareLinks.token("tailorit://import/.."))
        assertNull(ShareLinks.token("tailorit://import/."))
        assertEquals("a.b", ShareLinks.token("tailorit://import/a.b"))
        assertNull(ShareLinks.token("tailorit://import/" + "a".repeat(129)))
        assertEquals("tailorit://import/tok", ShareLinks.appUrl("tok"))

        val share = Share(token = "tok", appId = "app1", url = "tailorit://import/tok", webUrl = "https://x.example/s/tok")
        assertEquals("tailorit://import/tok", ShareLinks.appUrl(share))
        // サーバーの url が形に合わなければトークンから作る
        assertEquals("tailorit://import/tok", ShareLinks.appUrl(share.copy(url = "")))
        assertEquals("tailorit://import/tok", ShareLinks.appUrl(share.copy(url = "tailorit://import/other")))

        assertEquals("見て\nhttps://x.example/s/tok", ShareLinks.sendText("見て", share))
        assertEquals("見て", ShareLinks.sendText("見て", share.copy(webUrl = "")))
        assertTrue(share.webUrl.contains("/s/tok"))
    }
}
