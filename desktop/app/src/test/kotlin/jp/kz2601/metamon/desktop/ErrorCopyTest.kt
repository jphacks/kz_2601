package jp.kz2601.metamon.desktop

import jp.kz2601.metamon.desktop.i18n.Lang
import jp.kz2601.metamon.desktop.i18n.Strings
import jp.kz2601.metamon.services.http.ApiError
import jp.kz2601.metamon.services.http.FailureKind
import jp.kz2601.metamon.services.http.RateLimitInfo
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotEquals
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.IOException
import java.net.SocketTimeoutException
import java.time.Instant
import java.time.ZoneId

/** 失敗 → 知らせの文言(#2984)。Android の VoiceErrorCopy と同じ分け方・同じキー。 */
class ErrorCopyTest {
    private fun ja(error: Throwable) = Strings.withLang(Lang.JA) { ErrorCopy.of(error, zone = ZoneId.of("UTC")) }
    private fun en(error: Throwable) = Strings.withLang(Lang.EN) { ErrorCopy.of(error, zone = ZoneId.of("UTC")) }

    @Test
    fun offline() {
        val j = ja(IOException("no route"))
        assertEquals("通信が切れました", j.title)
        assertEquals("接続を確認して、もう一度お試しください", j.body)
        assertTrue(j.retry)
        val e = en(IOException("no route"))
        assertEquals("Connection lost", e.title)
        assertEquals("Check your connection and try again", e.body)
    }

    @Test
    fun timeout() {
        val j = ja(SocketTimeoutException("read"))
        assertEquals("時間がかかりすぎました", j.title)
        assertTrue(j.retry)
        assertEquals("That took too long", en(SocketTimeoutException("read")).title)
        // 504 も時間切れ
        assertEquals("時間がかかりすぎました", ja(ApiError.Http(504, "")).title)
    }

    @Test
    fun tooManyRequestsAndServerErrorsAreBusy() {
        for (status in listOf(429, 500, 502, 503)) {
            val j = ja(ApiError.Http(status, ""))
            assertEquals("$status", "混み合っています", j.title)
            assertEquals("$status", "少し時間をおいて、もう一度お試しください", j.body)
            assertTrue(j.retry)
            assertEquals("$status", "We're busy right now", en(ApiError.Http(status, "")).title)
        }
    }

    @Test
    fun dailyLimitSaysWhenItComesBack() {
        val at = Instant.parse("2026-11-13T09:30:00Z")
        val kind = FailureKind.RateLimited(RateLimitInfo(RateLimitInfo.Scope.USER, resetAt = at))
        val j = Strings.withLang(Lang.JA) { ErrorCopy.of(kind, zone = ZoneId.of("UTC")) }
        assertEquals("今日はここまでです", j.title)
        assertTrue(j.body, j.body.startsWith("今日の上限に達しました。"))
        assertTrue(j.body, j.body.contains("9:30"))
        assertFalse(j.retry)
        val noTime = Strings.withLang(Lang.EN) { ErrorCopy.of(FailureKind.RateLimited(RateLimitInfo(RateLimitInfo.Scope.IP))) }
        assertEquals("You've reached today's limit. You can continue from tomorrow", noTime.body)
    }

    @Test
    fun unknownFailureIsGenericAndRetryable() {
        val j = ja(IllegalStateException("boom"))
        assertEquals("うまく作れませんでした", j.title)
        assertEquals("少し待ってから、もう一度お試しください", j.body)
        assertTrue(j.retry)
        assertEquals("Couldn't make the app", en(IllegalStateException("boom")).title)
        // サーバーの文・例外の文は見せない
        assertFalse(j.line.contains("boom"))
    }

    @Test
    fun chatOriginChangesOnlyTheTitle() {
        val kind = FailureKind.Other("x")
        val chat = Strings.withLang(Lang.JA) { ErrorCopy.of(kind, ErrorCopy.Origin.CHAT) }
        val gen = Strings.withLang(Lang.JA) { ErrorCopy.of(kind, ErrorCopy.Origin.GENERATE) }
        assertEquals("質問をうまく用意できませんでした", chat.title)
        assertNotEquals(gen.title, chat.title)
        assertEquals(gen.body, chat.body)
    }

    @Test
    fun sessionExpiredAsksToSignInWithoutRetry() {
        val j = ja(ApiError.Http(401, ""))
        assertEquals("サインインの有効期限が切れました", j.title)
        assertFalse(j.retry)
    }

    @Test
    fun everyKindHasTextInBothLanguages() {
        val kinds = listOf(
            FailureKind.Offline, FailureKind.Timeout, FailureKind.SessionExpired, FailureKind.AppCheckFailed,
            FailureKind.AIPaused, FailureKind.FreePlanPaused(null), FailureKind.AppStopped, FailureKind.Busy(null),
            FailureKind.RateLimited(RateLimitInfo(RateLimitInfo.Scope.USER)), FailureKind.Conflict(1),
            FailureKind.Cancelled, FailureKind.Rejected("x"), FailureKind.Other("x"),
        )
        for (lang in Lang.entries) for (k in kinds) {
            val c = Strings.withLang(lang) { ErrorCopy.of(k) }
            // 引けないキーはキーそのものが返る
            assertFalse("$lang $k: ${c.title}", c.title.isBlank() || c.title.contains("_"))
            assertFalse("$lang $k: ${c.body}", c.body.isBlank() || c.body.startsWith("voice_") || c.body.startsWith("desktop_"))
        }
    }

    @Test
    fun appCheckBodyDoesNotPointToGooglePlay() {
        for (lang in Lang.entries) {
            val c = Strings.withLang(lang) { ErrorCopy.of(FailureKind.AppCheckFailed) }
            assertFalse(c.body, c.body.contains("Google Play"))
        }
    }
}
