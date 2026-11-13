package jp.kz2601.metamon.desktop.feedback

import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

/** 「直ったら知らせてほしい」: on の時だけ送る(偽の API・#3004)。 */
class FeedbackFixedOptInTest {
    private class FakeApi(private val fail: Boolean = false) : NotifyConsentApi {
        val calls = mutableListOf<Pair<String, Boolean>>()

        override suspend fun setNotifyConsent(appId: String, allowed: Boolean) {
            calls += appId to allowed
            if (fail) error("boom")
        }
    }

    @Test
    fun sendsConsentOnlyWhenOptedIn() = runTest {
        val api = FakeApi()
        assertTrue(FeedbackFixedOptIn(api).afterSend(true))
        assertEquals(listOf("feedback-fixed" to true), api.calls)
    }

    @Test
    fun sendsNothingWhenOff() = runTest {
        val api = FakeApi()
        assertTrue(FeedbackFixedOptIn(api).afterSend(false))
        assertTrue(api.calls.isEmpty())
    }

    @Test
    fun failureDoesNotThrow() = runTest {
        val api = FakeApi(fail = true)
        assertFalse(FeedbackFixedOptIn(api).afterSend(true))
        assertEquals(1, api.calls.size)
    }
}
