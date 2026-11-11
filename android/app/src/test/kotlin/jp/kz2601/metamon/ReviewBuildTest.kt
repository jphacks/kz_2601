package jp.kz2601.metamon

import jp.kz2601.metamon.voice.PromptTips
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

/** Play の審査用の版(#1275)。 */
class ReviewBuildTest {
    @Test
    fun normalBuildIsUnchanged() {
        assertFalse(BuildConfig.PLAY_REVIEW) // 既定はオフ
        assertFalse(ReviewBuild.SHARED_DATA in deviceCapabilities(review = false).disabledFeatures.orEmpty())
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
        assertEquals(listOf("a", "sharedData"), ReviewBuild.disabledFeatures(listOf("a", "sharedData"), review = true))
    }
}
