package jp.kz2601.metamon.desktop.net

import jp.kz2601.metamon.services.http.MetamonHttp
import jp.kz2601.metamon.services.http.RetryPolicy
import jp.kz2601.metamon.services.live.HttpAppConfigClient
import jp.kz2601.metamon.services.local.DocumentCache
import kotlinx.coroutines.runBlocking
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Rule
import org.junit.Test
import org.junit.rules.TemporaryFolder

/** サーバーのアプリの一覧と写し(#2378): 成功・圏外・5xx・空・サインインしていない(通信は MockWebServer だけ)。 */
class ServerApps2378Test {
    @get:Rule
    val tmp = TemporaryFolder()

    private lateinit var server: MockWebServer
    private lateinit var cache: DocumentCache

    @Before
    fun setUp() {
        server = MockWebServer().apply { start() }
        cache = DocumentCache(tmp.newFolder("documents"))
    }

    @After
    fun tearDown() {
        runCatching { server.shutdown() }
    }

    private fun apps(signedIn: Boolean = true): ServerApps {
        val http = MetamonHttp(
            baseUrl = server.url("/").toString(),
            tokens = DesktopApi.devTokens("dev-token"),
            clientInfo = DesktopApi.clientInfo(),
            client = DesktopApi.okHttp(ClientPlatform.LINUX, "ua"),
            retry = RetryPolicy(idempotentAttempts = 1),
        )
        return ServerApps(HttpAppConfigClient(http), cache) { signedIn }
    }

    private fun json(body: String, status: Int = 200) = MockResponse().setResponseCode(status).setHeader("Content-Type", "application/json").setBody(body)

    private val page = """{"items":[
        {"id":"a","name":"家計簿","currentVersion":3,"updatedAt":"2026-11-01T00:00:00Z"},
        {"id":"b","name":"買い物","currentVersion":5,"sortIndex":2,"updatedAt":"2026-10-01T00:00:00Z"},
        {"id":"c","name":"日記","currentVersion":4,"sortIndex":1,"updatedAt":"2026-09-01T00:00:00Z"},
        {"id":"d","name":"しまった","currentVersion":4,"status":"archived"},
        {"id":"e","name":"作りかけ","currentVersion":1}
    ]}"""

    @Test
    fun successOrdersAndWritesACopy() = runBlocking {
        server.enqueue(json(page))
        val r = apps().load() as ServerAppsResult.Loaded
        // 並べていない物(新しい順)→ 並べた順。archived と未生成は出さない
        assertEquals(listOf("a", "c", "b"), r.apps.map { it.id })
        assertEquals(false, r.fromCache)
        val req = server.takeRequest()
        assertEquals("GET", req.method)
        assertTrue(req.path!!.startsWith("/v1/apps"))
        assertTrue("写しを置いた", cache.loadJson(ServerApps.CACHE_KEY)!!.contains("家計簿"))
    }

    @Test
    fun offlineReturnsThePreviousCopy() = runBlocking {
        server.enqueue(json(page))
        apps().load()
        server.shutdown()
        val r = apps().load() as ServerAppsResult.Loaded
        assertTrue(r.fromCache)
        assertEquals(listOf("a", "c", "b"), r.apps.map { it.id })
    }

    @Test
    fun serverErrorReturnsThePreviousCopy() = runBlocking {
        server.enqueue(json(page))
        apps().load()
        server.enqueue(json("""{"error":{"code":"internal","message":"x"}}""", 500))
        val r = apps().load() as ServerAppsResult.Loaded
        assertTrue(r.fromCache)
        assertEquals(3, r.apps.size)
    }

    @Test
    fun newerListReplacesTheCopy() = runBlocking {
        server.enqueue(json(page))
        apps().load()
        server.enqueue(json("""{"items":[{"id":"z","name":"新しい","currentVersion":2}]}"""))
        apps().load()
        server.enqueue(json("", 503).setHeader("Retry-After", "0"))
        val r = apps().load() as ServerAppsResult.Loaded
        assertTrue(r.fromCache)
        assertEquals(listOf("z"), r.apps.map { it.id })
    }

    @Test
    fun emptyListIsLoadedAndCopied() = runBlocking {
        server.enqueue(json(page))
        apps().load()
        server.enqueue(json("""{"items":[]}"""))
        val r = apps().load() as ServerAppsResult.Loaded
        assertEquals(emptyList<Any>(), r.apps)
        assertEquals(false, r.fromCache)
        server.enqueue(json("", 500))
        assertEquals("空の写しを返す", ServerAppsResult.Loaded(emptyList(), fromCache = true), apps().load())
    }

    @Test
    fun failsWithoutACopy() = runBlocking {
        server.enqueue(json("", 500))
        assertTrue(apps().load() is ServerAppsResult.Failed)
    }

    @Test
    fun clientErrorDoesNotUseTheCopy() = runBlocking {
        server.enqueue(json(page))
        apps().load()
        server.enqueue(json("""{"error":{"code":"not_found","message":"x"}}""", 404))
        assertTrue(apps().load() is ServerAppsResult.Failed)
    }

    @Test
    fun signedOutDoesNotCall() = runBlocking {
        assertEquals(ServerAppsResult.SignedOut, apps(signedIn = false).load())
        assertEquals(ServerAppsResult.SignedOut, ServerApps(null, cache) { true }.load())
        assertEquals(0, server.requestCount)
    }

    @Test
    fun wipeDuringTheReadDoesNotWriteTheCopy() = runBlocking {
        val s = apps()
        // 読んでいる間にサインアウトの片付けで全部消した(epoch が進む)
        server.dispatcher = object : okhttp3.mockwebserver.Dispatcher() {
            override fun dispatch(request: okhttp3.mockwebserver.RecordedRequest): MockResponse {
                runBlocking { cache.removeAll() }
                return json(page)
            }
        }
        s.load()
        assertEquals(null, cache.loadJson(ServerApps.CACHE_KEY))
    }
}
