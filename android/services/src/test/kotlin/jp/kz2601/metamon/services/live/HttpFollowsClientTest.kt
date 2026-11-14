package jp.kz2601.metamon.services.live

import jp.kz2601.metamon.services.follows.FollowsReason
import jp.kz2601.metamon.services.http.ApiError
import jp.kz2601.metamon.services.http.errorBody
import jp.kz2601.metamon.services.http.json
import jp.kz2601.metamon.services.http.metamonHttp
import jp.kz2601.metamon.services.http.take
import kotlinx.coroutines.runBlocking
import okhttp3.mockwebserver.MockWebServer
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Assert.fail
import org.junit.Before
import org.junit.Test

/** フォローの API の口(docs/backend/openapi-appconfig.yaml `/apps/{appId}/follows`・#1028)。 */
class HttpFollowsClientTest {
    private lateinit var server: MockWebServer
    private lateinit var client: HttpFollowsClient

    @Before
    fun setUp() {
        server = MockWebServer().apply { start() }
        client = HttpFollowsClient(server.metamonHttp())
    }

    @After
    fun tearDown() {
        server.shutdown()
    }

    @Test
    fun listReadsUids() = runBlocking {
        server.enqueue(json(200, """{"uids":["a","b"]}"""))
        assertEquals(listOf("a", "b"), client.list("app 1"))
        val req = server.take()
        assertEquals("GET", req.method)
        assertEquals("/v1/apps/app%201/follows", req.path)
    }

    @Test
    fun followIsPutAndUnfollowIsDelete() = runBlocking {
        server.enqueue(json(200, """{"following":true,"count":1}"""))
        server.enqueue(json(200, """{"following":false,"count":0}"""))
        assertTrue(client.setFollow("app-1", "u/2", on = true))
        assertFalse(client.setFollow("app-1", "u3", on = false))
        server.take().let {
            assertEquals("PUT", it.method)
            assertEquals("/v1/apps/app-1/follows/u%2F2", it.path) // uid はエスケープする
        }
        server.take().let {
            assertEquals("DELETE", it.method)
            assertEquals("/v1/apps/app-1/follows/u3", it.path)
        }
    }

    @Test
    fun fullCarriesTheReason() = runBlocking {
        server.enqueue(json(403, errorBody("forbidden", details = """{"reason":"follows_full"}""")))
        try {
            client.setFollow("app-1", "u2", on = true)
            fail("throws")
        } catch (e: ApiError) {
            assertEquals(403, e.status)
            assertEquals(FollowsReason.FULL, e.reason)
        }
    }
}
