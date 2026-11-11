package jp.kz2601.metamon.services.http

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.IOException
import java.net.SocketTimeoutException
import java.net.UnknownHostException
import java.time.Instant
import kotlinx.coroutines.CancellationException

/** 応答 → [ApiError] → [FailureKind] の表(iOS の FailureKind.classify と同じ並び)。 */
class ErrorMappingTest {
    private val now = Instant.parse("2026-11-11T00:00:00Z")

    private data class Case(
        val name: String,
        val status: Int,
        val body: String,
        val retryAfter: Double? = null,
        val expected: FailureKind,
    )

    private val cases = listOf(
        Case("401 token", 401, errorBody("unauthenticated"), expected = FailureKind.SessionExpired),
        Case("401 app check", 401, errorBody("app_check_failed"), expected = FailureKind.AppCheckFailed),
        Case(
            "402 pro_required",
            402,
            errorBody("pro_required", details = """{"features":[{"id":"sharedData","title":"共有","reason":"費用"}],"reason":"creator_needs_pro"}"""),
            expected = FailureKind.ProRequiredKind(ProRequired(listOf(PaidFeature("sharedData", "共有", "費用")), creatorNeedsPro = true)),
        ),
        Case(
            "403 plan_limit_reached apps (アプリの数の上限)",
            403,
            errorBody("plan_limit_reached", details = """{"resource":"apps","plan":"free","limit":3,"upgrade":true}"""),
            expected = FailureKind.PlanLimitReached(PlanLimit(PlanLimit.Resource.APPS, "free", 3, upgrade = true)),
        ),
        Case(
            "403 plan_limit_reached by creator",
            403,
            errorBody("plan_limit_reached", details = """{"resource":"recordsPerApp","reason":"creator_plan_limit"}"""),
            expected = FailureKind.PlanLimitReached(PlanLimit(PlanLimit.Resource.RECORDS_PER_APP, byCreator = true)),
        ),
        Case("403 app_stopped", 403, errorBody("app_stopped"), expected = FailureKind.AppStopped),
        Case("403 forbidden", 403, errorBody("forbidden", "だめ"), expected = FailureKind.Rejected("だめ")),
        Case("409 version_conflict", 409, errorBody("version_conflict", details = """{"currentVersion":7}"""), expected = FailureKind.Conflict(7)),
        Case("409 duplicate_record", 409, errorBody("duplicate_record"), expected = FailureKind.Conflict(null)),
        Case("412 If-Match missing", 412, errorBody("bad_request", "If-Match required"), expected = FailureKind.Rejected("If-Match required")),
        Case("422 invalid_record", 422, errorBody("invalid_record", "金額が不正"), expected = FailureKind.Rejected("金額が不正")),
        Case(
            "429 rate_limited (今日の上限)",
            429,
            errorBody("rate_limited", details = """{"scope":"ip","tier":"anonymous","resetAt":"2026-11-11T15:00:00Z"}"""),
            expected = FailureKind.RateLimited(
                RateLimitInfo(RateLimitInfo.Scope.IP, tier = "anonymous", resetAt = Instant.parse("2026-11-11T15:00:00Z")),
            ),
        ),
        Case("429 busy (scope なし)", 429, errorBody("rate_limited"), retryAfter = 5.0, expected = FailureKind.Busy(5.0)),
        Case("500 internal", 500, errorBody("internal"), expected = FailureKind.Busy(null)),
        Case("503 ai_paused", 503, errorBody("ai_paused"), expected = FailureKind.AIPaused),
        Case(
            "503 free_plan_paused",
            503,
            errorBody("free_plan_paused", details = """{"resumeAt":"2026-11-12T00:00:00Z"}"""),
            expected = FailureKind.FreePlanPaused(Instant.parse("2026-11-12T00:00:00Z")),
        ),
        Case("503 upstream", 503, errorBody("upstream_unavailable"), retryAfter = 2.0, expected = FailureKind.Busy(2.0)),
        Case("504 upstream_timeout", 504, errorBody("upstream_timeout"), expected = FailureKind.Timeout),
        Case("502 not envelope", 502, "<html>bad gateway</html>", expected = FailureKind.Busy(null)),
    )

    @Test
    fun statusAndCodeMapToFailureKind() {
        for (c in cases) {
            val error = MetamonHttp.errorFrom(c.status, c.body, c.retryAfter)
            assertEquals(c.name, c.expected, FailureKind.classify(error, now))
        }
    }

    @Test
    fun transportErrors() {
        assertEquals(FailureKind.Timeout, FailureKind.classify(SocketTimeoutException("read timed out")))
        assertEquals(FailureKind.Offline, FailureKind.classify(UnknownHostException("no dns")))
        assertEquals(FailureKind.Offline, FailureKind.classify(IOException("reset")))
        assertEquals(FailureKind.Cancelled, FailureKind.classify(CancellationException("user")))
        assertTrue(FailureKind.classify(IllegalStateException("x")) is FailureKind.Other)
    }

    @Test
    fun retryableKinds() {
        assertTrue(FailureKind.Offline.isRetryable)
        assertTrue(FailureKind.Busy(null).isRetryable)
        assertFalse(FailureKind.AIPaused.isRetryable)
        assertFalse(FailureKind.Conflict(1).isRetryable)
    }

    @Test
    fun serverErrorKeepsCodeMessageAndDetails() {
        val e = MetamonHttp.errorFrom(422, errorBody("text_not_allowed", "表示できない言葉", """{"field":"title"}"""), null)
        e as ApiError.Server
        assertEquals("text_not_allowed", e.code)
        assertEquals("表示できない言葉", e.message)
        assertEquals("title", e.field)
        assertTrue(e.isTextNotAllowed)
        assertNull(e.proRequired)
    }

    @Test
    fun rateLimitResetFallsBackToRetryAfter() {
        val e = MetamonHttp.errorFrom(429, errorBody("rate_limited", details = """{"scope":"global"}"""), 60.0)
        val info = e.rateLimit(now)!!
        assertEquals(RateLimitInfo.Scope.GLOBAL, info.scope)
        assertEquals(now.plusSeconds(60), info.resetAt)
    }
}
