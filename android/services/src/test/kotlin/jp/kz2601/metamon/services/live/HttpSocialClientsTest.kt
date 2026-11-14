package jp.kz2601.metamon.services.live

import jp.kz2601.metamon.dsl.records.MatchBox
import jp.kz2601.metamon.services.http.ApiError
import jp.kz2601.metamon.services.http.json
import jp.kz2601.metamon.services.http.metamonHttp
import jp.kz2601.metamon.services.http.take
import jp.kz2601.metamon.services.social.BlockedUser
import jp.kz2601.metamon.services.social.MatchingReason
import jp.kz2601.metamon.services.social.ReportReason
import jp.kz2601.metamon.services.social.ReportTarget
import kotlinx.coroutines.runBlocking
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Assert.fail
import org.junit.Before
import org.junit.Test

/** 仲間探し・通報・ブロックの API の口(docs/backend/openapi-appconfig.yaml の matching / moderation・#877)。 */
class HttpSocialClientsTest {
    private lateinit var server: MockWebServer
    private lateinit var matching: HttpMatchingClient
    private lateinit var moderation: HttpModerationClient

    @Before
    fun setUp() {
        server = MockWebServer().apply { start() }
        val http = server.metamonHttp()
        matching = HttpMatchingClient(http)
        moderation = HttpModerationClient(http)
    }

    @After
    fun tearDown() {
        server.shutdown()
    }

    @Test
    fun sendInterestPostsTheTwoProfilesAndTheAgeConfirmation() = runBlocking {
        server.enqueue(json(200, """{"uid":"u2","profileRecordId":"p2","status":"matched","updatedAt":"2026-11-11T00:00:00Z"}"""))
        val r = matching.sendInterest("app 1", "p2", "p1", ageConfirmed = true)
        assertTrue(r.isMatched)
        val req = server.take()
        assertEquals("POST", req.method)
        assertEquals("/v1/apps/app%201/interests", req.path)
        assertEquals("""{"recordId":"p2","fromRecordId":"p1","ageConfirmed":true}""", req.body.readUtf8())
    }

    @Test
    fun listsUseInterestsWithBoxOrMatches() = runBlocking {
        server.enqueue(json(200, """{"items":[{"uid":"a","status":"pending","updatedAt":"x"},{"uid":"b","status":"pending","updatedAt":"y"}]}"""))
        server.enqueue(json(200, """{"items":[]}"""))
        assertEquals(listOf("a", "b"), matching.list("app-1", MatchBox.OUTGOING).map { it.uid })
        assertEquals(emptyList<Any>(), matching.list("app-1", MatchBox.MATCHES))
        assertEquals("/v1/apps/app-1/interests?box=outgoing", server.take().path)
        assertEquals("/v1/apps/app-1/matches", server.take().path)
    }

    @Test
    fun acceptDeclineUnmatchPaths() = runBlocking {
        server.enqueue(json(200, """{"uid":"u/2","status":"matched","updatedAt":"x"}"""))
        server.enqueue(MockResponse().setResponseCode(204))
        server.enqueue(MockResponse().setResponseCode(204))
        matching.accept("app-1", "u/2", ageConfirmed = true)
        matching.decline("app-1", "u3")
        matching.unmatch("app-1", "u4")
        val accept = server.take()
        assertEquals("/v1/apps/app-1/interests/u%2F2/accept", accept.path) // uid はエスケープする
        assertEquals("""{"ageConfirmed":true}""", accept.body.readUtf8())
        server.take().let {
            assertEquals("DELETE", it.method)
            assertEquals("/v1/apps/app-1/interests/u3", it.path)
        }
        server.take().let {
            assertEquals("DELETE", it.method)
            assertEquals("/v1/apps/app-1/matches/u4", it.path)
        }
    }

    @Test
    fun ageUnconfirmedReasonIsReadable() = runBlocking {
        server.enqueue(json(400, """{"error":{"code":"bad_request","message":"age","details":{"reason":"age_unconfirmed"}}}"""))
        try {
            matching.sendInterest("app-1", "p2", "p1", ageConfirmed = false)
            fail("throw するはず")
        } catch (e: ApiError) {
            assertEquals(MatchingReason.AGE_UNCONFIRMED, e.reason)
        }
    }

    @Test
    fun reportARecordSendsOnlyTheIdsAndTheReason() = runBlocking {
        server.enqueue(json(201, """{"created":true}"""))
        moderation.report(ReportTarget.RecordTarget("app-1", "posts", "r1"), ReportReason.HARASSMENT)
        val req = server.take()
        assertEquals("POST", req.method)
        assertEquals("/v1/reports", req.path)
        assertEquals("""{"kind":"record","appId":"app-1","entityId":"posts","recordId":"r1","reason":"harassment"}""", req.body.readUtf8())
    }

    @Test
    fun reportAListing() = runBlocking {
        server.enqueue(json(200, """{"created":false}"""))
        moderation.report(ReportTarget.Listing("l1"), ReportReason.SPAM)
        assertEquals("""{"kind":"listing","id":"l1","reason":"spam"}""", server.take().body.readUtf8())
    }

    @Test
    fun blockListAndUnblock() = runBlocking {
        server.enqueue(json(201, """{"uid":"u2","name":"はなこ","createdAt":"x"}"""))
        server.enqueue(json(200, """{"items":[{"uid":"u2","name":"はなこ","createdAt":"x"}]}"""))
        server.enqueue(MockResponse().setResponseCode(204))
        server.enqueue(json(201, """{"uid":"u9","name":"匿名","createdAt":"x"}"""))
        assertEquals(BlockedUser("u2", "はなこ"), moderation.block("u2", "はなこ"))
        assertEquals(listOf(BlockedUser("u2", "はなこ")), moderation.blockedUsers())
        moderation.unblock("u2")
        moderation.block("u9", null)

        server.take().let {
            assertEquals("/v1/me/blocks", it.path)
            assertEquals("""{"uid":"u2","name":"はなこ"}""", it.body.readUtf8())
        }
        assertEquals("GET", server.take().method)
        server.take().let {
            assertEquals("DELETE", it.method)
            assertEquals("/v1/me/blocks/u2", it.path)
        }
        assertEquals("""{"uid":"u9"}""", server.take().body.readUtf8()) // 名前が無ければ送らない(サーバーが「匿名」)
    }
}
