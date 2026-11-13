package jp.kz2601.metamon.services.follows

import jp.kz2601.metamon.services.http.ApiError
import jp.kz2601.metamon.services.http.ClientInfo
import jp.kz2601.metamon.services.http.MetamonHttp
import jp.kz2601.metamon.services.http.TokenProvider
import jp.kz2601.metamon.services.live.HttpFollowsClient
import kotlinx.coroutines.runBlocking
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

/** フォローの API クライアントを android/services から取り込んだ物(#2802)。通信は MockWebServer だけ。 */
class DesktopFollowsClientTest {
    private lateinit var server: MockWebServer
    private lateinit var client: HttpFollowsClient

    @Before
    fun setUp() {
        server = MockWebServer().apply { start() }
        val tokens = object : TokenProvider {
            override suspend fun idToken(forceRefresh: Boolean): String? = "tok"
        }
        client = HttpFollowsClient(MetamonHttp(server.url("/").toString(), tokens, ClientInfo("0.1.0", "1"), sleep = {}))
    }

    @After
    fun tearDown() {
        server.shutdown()
    }

    private fun json(body: String, code: Int = 200) = MockResponse().setResponseCode(code).setHeader("Content-Type", "application/json").setBody(body)

    @Test
    fun listReadsUids() = runBlocking {
        server.enqueue(json("""{"uids":["u1","u2"]}"""))
        assertEquals(listOf("u1", "u2"), client.list("app1"))
        val req = server.takeRequest()
        assertEquals("GET", req.method)
        assertEquals("/v1/apps/app1/follows", req.path)
        assertEquals("Bearer tok", req.getHeader("Authorization"))
    }

    @Test
    fun followIsPut() = runBlocking {
        server.enqueue(json("""{"following":true}"""))
        assertTrue(client.setFollow("app1", "u9", on = true))
        val req = server.takeRequest()
        assertEquals("PUT", req.method)
        assertEquals("/v1/apps/app1/follows/u9", req.path)
    }

    @Test
    fun unfollowIsDelete() = runBlocking {
        server.enqueue(json("""{"following":false}"""))
        assertFalse(client.setFollow("app1", "u9", on = false))
        val req = server.takeRequest()
        assertEquals("DELETE", req.method)
        assertEquals("/v1/apps/app1/follows/u9", req.path)
    }

    @Test
    fun fullIsReadAsReason() = runBlocking {
        server.enqueue(json("""{"error":{"code":"conflict","message":"full","details":{"reason":"follows_full"}}}""", code = 409))
        try {
            client.setFollow("app1", "u9", on = true)
            fail("通ってしまった")
        } catch (e: ApiError) {
            assertEquals(409, e.status)
            assertEquals(FollowsReason.FULL, e.reason)
        }
    }

    @Test
    fun storeAppliesServerAnswerAndRollsBackOnFailure() = runBlocking {
        val store = FollowStore("app1", client)
        assertNull(store.uids)
        server.enqueue(json("""{"uids":["u1"]}"""))
        store.load()
        assertEquals(setOf("u1"), store.uids)

        server.enqueue(json("""{"following":true}"""))
        store.set("u2", on = true)
        assertTrue(store.isFollowing("u2"))

        // 失敗したら前の一覧に戻して throw
        server.enqueue(json("""{"error":{"code":"forbidden","message":"no","details":{"reason":"not_in_app"}}}""", code = 403))
        try {
            store.set("u3", on = true)
            fail("通ってしまった")
        } catch (e: ApiError) {
            assertEquals(FollowsReason.NOT_IN_APP, e.reason)
        }
        assertEquals(setOf("u1", "u2"), store.uids)
        assertTrue(store.snapshot.value.busy.isEmpty())
    }
}
