package jp.kz2601.metamon.voice

import jp.kz2601.metamon.R
import jp.kz2601.metamon.services.TranscriberFailure
import jp.kz2601.metamon.services.http.FailureKind
import jp.kz2601.metamon.services.http.RateLimitInfo
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test
import java.time.Instant

/** 録音・依頼の失敗 → 状態カードの文言(iOS の ErrorCopy と同じ分け方)。 */
class VoiceErrorCopyTest {
    @Test
    fun transcriberReasonsMatchIOS() {
        fun t(f: TranscriberFailure) = VoiceErrorCopy.transcriber(f)
        assertEquals(R.string.voice_t_mic_denied, t(TranscriberFailure.MicrophoneDenied).title)
        assertTrue(t(TranscriberFailure.MicrophoneDenied).openSettings)
        assertEquals(R.string.voice_b_too_short, t(TranscriberFailure.TooShort(0.2)).body)
        assertEquals(R.string.voice_t_not_heard, t(TranscriberFailure.TooShort(0.2)).title)
        assertEquals(R.string.voice_b_silent, t(TranscriberFailure.Silent(3.0)).body)
        assertEquals(R.string.voice_b_not_heard, t(TranscriberFailure.NotHeard).body)
        assertEquals(R.string.voice_t_record_failed, t(TranscriberFailure.NoInputAvailable).title)
        assertEquals(R.string.voice_t_record_failed, t(TranscriberFailure.Other(42)).title)
        assertEquals(R.string.voice_t_unavailable, t(TranscriberFailure.OnDeviceRecognitionUnavailable).title)
        assertEquals(R.string.voice_t_not_ready, t(TranscriberFailure.NotReady).title)
        // 録音の失敗は「もう一度」ではなく話し直す
        assertFalse(t(TranscriberFailure.Silent(3.0)).retry)
    }

    @Test
    fun requestKinds() {
        fun r(k: FailureKind, o: VoiceError.Origin = VoiceError.Origin.GENERATE) = VoiceErrorCopy.request(k, o)
        assertTrue(r(FailureKind.Offline).retry)
        assertEquals(R.string.voice_offline_title, r(FailureKind.Offline).title)
        assertEquals(R.string.voice_busy_title, r(FailureKind.Busy(null)).title)
        assertFalse(r(FailureKind.SessionExpired).retry)
        assertEquals(R.string.voice_rejected_body, r(FailureKind.Rejected("x")).body)
        assertEquals(R.string.voice_failed_title, r(FailureKind.Other("x")).title)
        assertEquals(R.string.voice_chat_failed_title, r(FailureKind.Other("x"), VoiceError.Origin.CHAT).title)
        val reset = Instant.parse("2026-11-12T00:00:00Z")
        val rate = r(FailureKind.RateLimited(RateLimitInfo(RateLimitInfo.Scope.USER, resetAt = reset)))
        assertEquals(R.string.voice_rate_body, rate.body)
        assertEquals(reset, rate.resetAt)
        assertFalse(rate.retry)
    }

    @Test
    fun timeOfDay() {
        assertEquals(R.string.voice_hello_morning, Greeting.timeOfDay(5))
        assertEquals(R.string.voice_hello_day, Greeting.timeOfDay(11))
        assertEquals(R.string.voice_hello_evening, Greeting.timeOfDay(18))
        assertEquals(R.string.voice_hello_evening, Greeting.timeOfDay(3))
    }
}
