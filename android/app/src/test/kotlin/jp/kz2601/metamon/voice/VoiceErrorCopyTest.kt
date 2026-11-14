package jp.kz2601.metamon.voice

import jp.kz2601.metamon.R
import jp.kz2601.metamon.purchase.PaywallCopy
import jp.kz2601.metamon.purchase.PaywallReason
import jp.kz2601.metamon.services.TranscriberFailure
import jp.kz2601.metamon.services.http.FailureKind
import jp.kz2601.metamon.services.http.PaidFeature
import jp.kz2601.metamon.services.http.PlanLimit
import jp.kz2601.metamon.services.http.RateLimitInfo
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
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

    /** 上限のカードの「Pro にする」(#1103・iOS の `ErrorPresentation.primary` の upgradePro) */
    @Test
    fun upgradeProOnlyWhereProHelps() {
        fun r(k: FailureKind) = VoiceErrorCopy.request(k, VoiceError.Origin.GENERATE)
        fun rate(scope: RateLimitInfo.Scope, tier: String?) = r(FailureKind.RateLimited(RateLimitInfo(scope, tier = tier))).upgrade
        // 今日の上限: サインインした無料の人だけ。匿名・IP・全体の停止・Pro は出さない
        assertEquals(PaywallReason.LIMIT_REACHED, rate(RateLimitInfo.Scope.USER, "signed_in"))
        assertNull(rate(RateLimitInfo.Scope.USER, "anonymous"))
        assertNull(rate(RateLimitInfo.Scope.USER, "pro"))
        assertNull(rate(RateLimitInfo.Scope.IP, "signed_in"))
        assertNull(rate(RateLimitInfo.Scope.GLOBAL, "signed_in"))
        // プランの上限: 上がる時・作成者本人だけ。設定と同じ見出しで開く(#317)
        val own = r(FailureKind.PlanLimitReached(PlanLimit(PlanLimit.Resource.RECORDS_PER_APP, upgrade = true)))
        assertEquals(PaywallReason.SETTINGS, own.upgrade)
        assertEquals(R.string.voice_plan_limit_body_pro, own.body)
        val byCreator = r(FailureKind.PlanLimitReached(PlanLimit(PlanLimit.Resource.RECORDS_PER_APP, upgrade = true, byCreator = true)))
        assertNull(byCreator.upgrade)
        assertEquals(R.string.voice_plan_limit_body, byCreator.body)
        assertNull(r(FailureKind.PlanLimitReached(PlanLimit(PlanLimit.Resource.APPS, upgrade = false))).upgrade)
        assertNull(r(FailureKind.Offline).upgrade)
    }

    @Test
    fun paywallCopyForNewReasons() {
        assertEquals(R.string.paywall_headline_limit_reached, PaywallCopy.headline(PaywallReason.LIMIT_REACHED))
        assertEquals(R.string.paywall_headline_paid_feature, PaywallCopy.headline(PaywallReason.PaidFeatures()))
        assertNull(PaywallCopy.closeNote(PaywallReason.LIMIT_REACHED))
        val features = listOf(PaidFeature("sharedData", "共有のデータ", "費用がかかる"), PaidFeature("online", "", ""))
        assertEquals("共有のデータ", PaywallCopy.featureNames(features))
        assertNull(PaywallCopy.featureNames(listOf(PaidFeature("x", ""))))
        assertEquals(listOf("費用がかかる"), PaywallCopy.featureReasons(features))
    }

    @Test
    fun timeOfDay() {
        assertEquals(R.string.voice_hello_morning, Greeting.timeOfDay(5))
        assertEquals(R.string.voice_hello_day, Greeting.timeOfDay(11))
        assertEquals(R.string.voice_hello_evening, Greeting.timeOfDay(18))
        assertEquals(R.string.voice_hello_evening, Greeting.timeOfDay(3))
    }
}
