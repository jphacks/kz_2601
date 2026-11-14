package jp.kz2601.metamon

import jp.kz2601.metamon.renderer.registry.DeviceCapabilities
import jp.kz2601.metamon.voice.PromptTips
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

/** Play の審査用の版(#1275)。 */
class ReviewBuildTest {
    /** iOS ClientCapabilities.reviewExcludedComponents の写し(変えたら両方を揃える) */
    private val iosReviewExcluded = setOf(
        "healthStat", "locationTracker", "nearbyShare", "dashcam", "sensorValue", "audioRecorder",
        "webView", "chatThread", "matchList", "feed", "aiChat", "callButton",
    )

    /** iOS の通常の版にあって審査用の版の supportedFeatures に無いもの(docs/backend/openapi-ai.yaml にも書いてある) */
    private val iosReviewWithheld = setOf(
        "matching", "dataRoles", "addressedMessages", "photoField.illustrate", "component.callButton", "field.encrypted",
    )

    @Test
    fun normalBuildIsUnchanged() {
        assertFalse(BuildConfig.PLAY_REVIEW) // 既定はオフ
        val base = DeviceCapabilities.of()
        val caps = deviceCapabilities(review = false)
        assertEquals(base.allowedComponents, caps.allowedComponents)
        assertEquals(base.disabledFeatures, caps.disabledFeatures)
        assertEquals(base.supportedFeatures, caps.supportedFeatures)
        assertFalse(ReviewBuild.SHARED_DATA in caps.disabledFeatures.orEmpty())
        assertTrue(ReviewBuild.canShare(review = false, mock = false, mockSharing = false))
        assertTrue(ReviewBuild.canShare(review = false, mock = true, mockSharing = true))
        assertFalse(ReviewBuild.canShare(review = false, mock = true, mockSharing = false))
        assertEquals(PromptTips.Feature.entries.toSet(), PromptTips.available(review = false))
    }

    @Test
    fun reviewBuildTurnsOffSharing() {
        val disabled = deviceCapabilities(review = true).disabledFeatures.orEmpty()
        assertTrue(ReviewBuild.SHARED_DATA in disabled)
        assertTrue("fieldHealth" in disabled) // もとの分は残す
        assertFalse(ReviewBuild.canShare(review = true, mock = false, mockSharing = false))
        assertFalse(ReviewBuild.canShare(review = true, mock = true, mockSharing = true))
        assertFalse(PromptTips.Feature.SHARING in PromptTips.available(review = true))
        assertEquals( // 重ねない
            listOf("a", "sharedData", "fieldHealth", "cameraCapture"),
            ReviewBuild.disabledFeatures(listOf("a", "sharedData"), review = true),
        )
    }

    @Test
    fun reviewSetsMatchIos() {
        assertEquals(iosReviewExcluded, ReviewBuild.EXCLUDED_COMPONENTS)
        assertEquals(iosReviewWithheld, ReviewBuild.WITHHELD_FEATURES)
        assertEquals(listOf("fieldHealth", "cameraCapture", "sharedData"), ReviewBuild.DISABLED_FEATURES)
    }

    @Test
    fun reviewBuildLeavesOutWhatIosReviewLeavesOut() {
        val caps = deviceCapabilities(review = true)
        val allowed = caps.allowedComponents.orEmpty()
        val supported = caps.supportedFeatures.orEmpty()
        iosReviewExcluded.forEach { assertFalse("$it は審査用の版で申告しない", it in allowed) }
        iosReviewWithheld.forEach { assertFalse("$it は審査用の版で送らない", it in supported) }
        assertEquals(DeviceCapabilities.of().allowedComponents.filterNot { it in iosReviewExcluded }, allowed)
        assertEquals(DeviceCapabilities.of().supportedFeatures.filterNot { it in iosReviewWithheld }, supported)
        assertEquals(setOf("fieldHealth", "cameraCapture", "sharedData"), caps.disabledFeatures.orEmpty().toSet())
        // 審査用の版でも残す(端末内の処理だけ・iOS の審査用の版も送る)
        listOf("recordRetention", "singleEntity", "notification.near", "openUrlButton.app").forEach {
            assertTrue("$it は審査用の版も送る", it in supported)
        }
    }

    @Test
    fun reviewTipsMatchIosAvailability() {
        // iOS の Availability.review: 共有・撮影・近く・健康・位置の記録の例を出さない
        assertEquals(emptySet<PromptTips.Feature>(), PromptTips.available(review = true))
    }
}
