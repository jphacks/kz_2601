package jp.kz2601.metamon.services.live

import jp.kz2601.metamon.services.books.isBookLookupOff
import jp.kz2601.metamon.services.http.ApiError
import jp.kz2601.metamon.services.http.json
import jp.kz2601.metamon.services.http.metamonHttp
import jp.kz2601.metamon.services.http.take
import kotlinx.coroutines.runBlocking
import okhttp3.mockwebserver.MockWebServer
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Assert.fail
import org.junit.Before
import org.junit.Test

/** lookupBook の口(#911・`POST /v1/lookup/book`)。 */
class HttpBookLookupClientTest {
    private lateinit var server: MockWebServer
    private lateinit var client: HttpBookLookupClient

    @Before
    fun setUp() {
        server = MockWebServer().apply { start() }
        client = HttpBookLookupClient(server.metamonHttp())
    }

    @After
    fun tearDown() {
        server.shutdown()
    }

    @Test
    fun postsIsbnInBodyAndReadsBook() = runBlocking {
        server.enqueue(
            json(
                200,
                """{"isbn":"9784780802047","title":"本","authors":"著者","publishedDate":"2014-04","source":{"name":"openBD","url":"https://openbd.jp"},"fetchedAt":"2026-11-11T00:00:00Z"}""",
            ),
        )
        val book = client.lookup("9784780802047")
        assertEquals("本", book.title)
        assertEquals("著者", book.authors)
        assertNull(book.publisher)
        assertEquals("2014-04", book.publishedDate)
        assertEquals("openBD", book.source.name)
        val req = server.take()
        assertEquals("POST", req.method)
        assertEquals("ISBN は URL に載せない", "/v1/lookup/book", req.path)
        assertEquals("""{"isbn":"9784780802047"}""", req.body.readUtf8())
    }

    @Test
    fun disabledIsToldApartFrom404() = runBlocking {
        server.enqueue(json(503, """{"error":{"code":"unavailable","message":"off","details":{"reason":"disabled"}}}"""))
        try {
            client.lookup("9784780802047")
            fail("throws")
        } catch (e: ApiError) {
            assertTrue(e.isBookLookupOff)
        }
        server.enqueue(json(404, """{"error":{"code":"not_found","message":"no"}}"""))
        try {
            client.lookup("9784780802047")
            fail("throws")
        } catch (e: ApiError) {
            assertEquals(404, e.status)
            assertFalse(e.isBookLookupOff)
        }
    }
}
