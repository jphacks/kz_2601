package jp.kz2601.metamon.services.discover

import jp.kz2601.metamon.services.http.ApiError
import jp.kz2601.metamon.services.http.ClientInfo
import jp.kz2601.metamon.services.http.MetamonHttp
import jp.kz2601.metamon.services.http.TokenProvider
import jp.kz2601.metamon.services.live.HttpDiscoverClient
import kotlinx.coroutines.runBlocking
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Assert.fail
import org.junit.Before
import org.junit.Test

/** 公開アプリを探す API クライアントを android/services から取り込んだ物(#2804)。通信は MockWebServer だけ。 */
class DesktopDiscoverClientTest {
    private lateinit var server: MockWebServer
    private lateinit var client: HttpDiscoverClient

    @Before
    fun setUp() {
        server = MockWebServer().apply { start() }
        val tokens = object : TokenProvider {
            override suspend fun idToken(forceRefresh: Boolean): String? = "tok"
        }
        client = HttpDiscoverClient(MetamonHttp(server.url("/").toString(), tokens, ClientInfo("0.1.0", "1"), sleep = {}))
    }

    @After
    fun tearDown() {
        server.shutdown()
    }

    private fun json(body: String, code: Int = 200) = MockResponse().setResponseCode(code).setHeader("Content-Type", "application/json").setBody(body)

    @Test
    fun searchSendsConditionsInBody() = runBlocking {
        server.enqueue(json("""{"items":[{"id":"p1","name":"家計簿","tags":["お金"],"installs":12}],"nextPageToken":"n2"}"""))
        val page = client.search("  家計簿 ", DiscoverSort.NEW, pageToken = "n1")
        assertEquals("家計簿", page.items.single().name)
        assertEquals(12, page.items.single().installs)
        assertEquals("n2", page.nextPageToken)

        val req = server.takeRequest()
        assertEquals("POST", req.method)
        // 検索語は URL に載せない
        assertEquals("/v1/public-apps/search", req.path)
        assertEquals("Bearer tok", req.getHeader("Authorization"))
        val body = Json.parseToJsonElement(req.body.readUtf8()).jsonObject
        assertEquals("家計簿", body.getValue("q").jsonPrimitive.content)
        assertEquals("new", body.getValue("sort").jsonPrimitive.content)
        assertEquals("n1", body.getValue("pageToken").jsonPrimitive.content)
        assertEquals("50", body.getValue("pageSize").jsonPrimitive.content)
    }

    @Test
    fun blankQueryIsNotSentAndEmptyResultIsEmpty() = runBlocking {
        server.enqueue(json("""{"items":[]}"""))
        val page = client.search("   ", DiscoverSort.POPULAR)
        assertTrue(page.items.isEmpty())
        assertNull(page.nextPageToken)
        val body = Json.parseToJsonElement(server.takeRequest().body.readUtf8()).jsonObject
        assertFalse(body.containsKey("q"))
        assertFalse(body.containsKey("pageToken"))
        assertEquals("popular", body.getValue("sort").jsonPrimitive.content)
    }

    @Test
    fun serverErrorIsThrownAndNotRetried() = runBlocking {
        server.enqueue(json("""{"error":{"code":"internal","message":"boom"}}""", code = 500))
        try {
            client.search("", DiscoverSort.POPULAR)
            fail("通ってしまった")
        } catch (e: ApiError) {
            assertEquals(500, e.status)
        }
        // POST は二重実行を避けて送り直さない
        assertEquals(1, server.requestCount)
    }

    @Test
    fun modelShowsItemsAndFailures() = runBlocking {
        val model = DiscoverModel(client)
        server.enqueue(json("""{"items":[{"id":"p1","tags":["#お金"]},{"id":"p2","tags":["料理"]}]}"""))
        model.search()
        assertEquals(listOf("p1", "p2"), model.state.value.items.map { it.id })
        assertNull(model.state.value.failure)

        // 5xx は「その他」。前の一覧は残す
        server.enqueue(MockResponse().setResponseCode(502).setBody("bad gateway"))
        model.search()
        assertEquals(DiscoverFailure.OTHER, model.state.value.failure)
        assertEquals(2, model.state.value.items.size)
        assertFalse(model.state.value.loading)

        // 「探す」が使えない構成
        val off = DiscoverModel(null)
        off.search()
        assertEquals(DiscoverFailure.UNAVAILABLE, off.state.value.failure)
    }

    @Test
    fun installPublishAndUnpublish() = runBlocking {
        server.enqueue(json("""{"id":"a9","name":"家計簿","dsl":{"version":1},"created":false}"""))
        val installed = client.install("p1")
        assertEquals("a9", installed.id)
        assertFalse(installed.created)
        server.takeRequest().let {
            assertEquals("POST", it.method)
            assertEquals("/v1/public-apps/p1/install", it.path)
        }

        server.enqueue(json("""{"id":"a1","name":"共有名","tags":["お金"]}"""))
        val listing = client.publish("a1", PublishRequest(name = "共有名", tags = listOf("お金")))
        assertEquals("共有名", listing.name)
        server.takeRequest().let {
            assertEquals("POST", it.method)
            assertEquals("/v1/apps/a1/publish", it.path)
            val body = Json.parseToJsonElement(it.body.readUtf8()).jsonObject
            assertEquals("共有名", body.getValue("name").jsonPrimitive.content)
            assertEquals("お金", body.getValue("tags").jsonArray.single().jsonPrimitive.content)
            // null のキーは送らない(サーバーが DSL の説明・「匿名」を使う)
            assertFalse(body.containsKey("description"))
            assertFalse(body.containsKey("authorName"))
        }

        server.enqueue(MockResponse().setResponseCode(204))
        client.unpublish("a1")
        server.takeRequest().let {
            assertEquals("DELETE", it.method)
            assertEquals("/v1/apps/a1/publish", it.path)
        }
    }
}
