package jp.kz2601.metamon.feedback

import jp.kz2601.metamon.R
import jp.kz2601.metamon.services.http.ApiError
import jp.kz2601.metamon.services.http.MetamonHttp
import org.junit.Assert.assertEquals
import org.junit.Test
import java.io.IOException
import java.net.SocketTimeoutException

class FeedbackErrorMessageTest {
    private fun server(status: Int, body: String): ApiError = MetamonHttp.errorFrom(status, body, null)

    @Test
    fun mapsLikeIos() {
        val limit = server(429, """{"error":{"code":"rate_limited","message":"m","details":{"scope":"user","limit":5,"resetAt":"2026-11-12T00:00:00+09:00"}}}""")
        assertEquals(R.string.feedback_composer_error_rate_limited, errorMessage(limit))
        assertEquals(R.string.feedback_composer_error_offline, errorMessage(IOException("x")))
        assertEquals(R.string.feedback_composer_error_server_busy, errorMessage(SocketTimeoutException()))
        assertEquals(R.string.feedback_composer_error_server_busy, errorMessage(server(503, "{}")))
        assertEquals(R.string.feedback_composer_error_session, errorMessage(server(401, "{}")))
        assertEquals(R.string.feedback_composer_error_default, errorMessage(server(413, """{"error":{"code":"too_large","message":"m"}}""")))
    }
}
