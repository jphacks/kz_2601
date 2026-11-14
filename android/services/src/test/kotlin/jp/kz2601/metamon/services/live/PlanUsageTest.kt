package jp.kz2601.metamon.services.live

import jp.kz2601.metamon.services.RemainingToday
import jp.kz2601.metamon.services.http.errorBody
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
import org.junit.Before
import org.junit.Test
import java.time.Instant

/** `GET /v1/me/usage` と「今日の残り」(#630・iOS の `RemainingTodayTests`)。 */
class PlanUsageTest {
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

    @Test
    fun readsRemainingTodayFromMyUsage() = runBlocking<Unit> {
        server.enqueue(
            json(
                200,
                """{"plan":"pro","tier":"pro","resetAt":"2026-11-11T15:00:00Z","ai":{"used":4,"limit":60,"remaining":56,"weights":{"chat":1}},"apps":null,"perApp":{}}""",
            ),
        )
        val usage = client.myUsage()!!
        assertEquals("/v1/me/usage", server.take().path)
        assertTrue(usage.isPro)
        val at = Instant.parse("2026-11-11T03:00:00Z")
        val today = usage.remainingToday(at)!!
        assertEquals(56, today.remaining)
        assertEquals(60, today.limit)
        assertEquals(Instant.parse("2026-11-11T15:00:00Z"), today.resetAt)
        assertEquals(at, today.observedAt)
    }

    @Test
    fun aiNullMeansNoRemaining() = runBlocking<Unit> {
        server.enqueue(json(200, """{"plan":"free","resetAt":"2026-11-11T15:00:00Z","ai":null,"apps":null,"perApp":{}}"""))
        assertNull(client.myUsage()!!.remainingToday(Instant.now()))
    }

    @Test
    fun oldServerWithoutUsageIsNull() = runBlocking<Unit> {
        server.enqueue(json(404, errorBody("NOT_FOUND")))
        assertNull(client.myUsage())
    }

    @Test
    fun readsAppCountFromMyUsage() = runBlocking<Unit> {
        server.enqueue(
            json(
                200,
                """{"plan":"free","tier":"signed_in","resetAt":"2026-11-11T15:00:00Z","ai":null,"apps":{"used":3,"limit":5,"remaining":2},"perApp":{"recordsPerApp":100,"photosPerApp":null}}""",
            ),
        )
        val apps = client.myUsage()!!.apps!!
        assertEquals(3, apps.used)
        assertEquals(5, apps.limit)
    }

    @Test
    fun readsAppUsage() = runBlocking<Unit> {
        server.enqueue(
            json(
                200,
                """{"appId":"a1","plan":"free","payer":"self","upgrade":true,"records":{"used":90,"limit":100,"remaining":10},"photos":{"used":2,"limit":null,"remaining":null}}""",
            ),
        )
        val usage = client.appUsage("a1")!!
        assertEquals("/v1/apps/a1/usage", server.take().path)
        assertTrue(usage.isMine)
        assertEquals(90, usage.records!!.used)
        assertNull(usage.photos!!.limit)
    }

    @Test
    fun appUsageWithoutCopyIsNull() = runBlocking<Unit> {
        server.enqueue(json(404, errorBody("NOT_FOUND")))
        assertNull(client.appUsage("missing"))
    }

    @Test
    fun staleAfterResetOrNextJstDay() {
        val reset = Instant.parse("2026-11-11T15:00:00Z")
        val value = RemainingToday(5, 30, reset, observedAt = Instant.parse("2026-11-11T03:00:00Z"))
        assertFalse(value.isStale(Instant.parse("2026-11-11T14:59:59Z")))
        assertTrue(value.isStale(reset))
        // 戻る時刻が無ければ JST の日付で見る(14:59Z = 23:59 JST / 15:00Z = 翌 0:00 JST)
        val noReset = value.copy(resetAt = null)
        assertFalse(noReset.isStale(Instant.parse("2026-11-11T14:59:00Z")))
        assertTrue(noReset.isStale(Instant.parse("2026-11-11T15:00:00Z")))
    }
}
