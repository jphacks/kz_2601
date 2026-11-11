package jp.kz2601.metamon.renderer.media

import jp.kz2601.metamon.dsl.ComponentType
import jp.kz2601.metamon.dsl.Source
import jp.kz2601.metamon.dsl.records.JSONValue
import jp.kz2601.metamon.dsl.records.LocalVideoStore
import jp.kz2601.metamon.renderer.components.inputs.InputRules
import jp.kz2601.metamon.renderer.components.media.PhotoCapturePlan
import jp.kz2601.metamon.renderer.components.media.PhotoSourcePolicy
import jp.kz2601.metamon.renderer.components.media.VideoCodec
import jp.kz2601.metamon.renderer.components.media.VideoDisplay
import jp.kz2601.metamon.renderer.registry.ComponentRegistry
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test
import java.nio.file.Files

class VideoRulesTest {
    @Test
    fun registered() {
        assertTrue(ComponentRegistry.standard.registeredTypes.containsAll(setOf("videoField", "videoPlayer")))
        assertTrue(ComponentType.VIDEO_FIELD in InputRules.inputTypes)
    }

    @Test
    fun durationLimit() {
        assertEquals(60, VideoCodec.limitSeconds(null))
        assertEquals(30, VideoCodec.limitSeconds(30))
        assertEquals(1, VideoCodec.limitSeconds(0))
        assertEquals(1, VideoCodec.limitSeconds(-5))
        assertEquals(30_000_000L, VideoCodec.limitMicros(30))
    }

    @Test
    fun trimsAfterLimit() {
        val limit = VideoCodec.limitMicros(30)
        assertTrue(VideoCodec.keepsSample(0, limit))
        assertTrue(VideoCodec.keepsSample(limit, limit))
        assertFalse(VideoCodec.keepsSample(limit + 1, limit))
        assertFalse(VideoCodec.keepsSample(-1, limit))
    }

    @Test
    fun copiesAsIsOnlyWithinLimit() {
        assertTrue(VideoCodec.canCopyAsIs(30_000, 30))
        assertFalse(VideoCodec.canCopyAsIs(30_001, 30))
        assertFalse(VideoCodec.canCopyAsIs(null, 30))
        assertFalse(VideoCodec.canCopyAsIs(0, 30))
        assertTrue(VideoCodec.canCopyAsIs(59_000, null))
    }

    @Test
    fun sourceDecidesHowToOpen() {
        assertEquals(PhotoCapturePlan.CAMERA, PhotoSourcePolicy.plan(Source.CAMERA, cameraAvailable = true))
        assertEquals(PhotoCapturePlan.LIBRARY, PhotoSourcePolicy.plan(Source.LIBRARY, cameraAvailable = true))
        assertEquals(PhotoCapturePlan.CHOOSE, PhotoSourcePolicy.plan(null, cameraAvailable = true))
        assertEquals(PhotoCapturePlan.LIBRARY, PhotoSourcePolicy.plan(Source.CAMERA, cameraAvailable = false))
    }

    @Test
    fun displayDependsOnWhereTheFileIs() {
        val dir = Files.createTempDirectory("videos").toFile()
        try {
            val store = LocalVideoStore(dir)
            val id = store.importFile(dir.resolve("src.mp4").apply { writeText("x") }, "here")
            assertEquals(VideoDisplay.Empty, VideoDisplay.of(null, store))
            assertEquals(VideoDisplay.Empty, VideoDisplay.of(JSONValue.Str("not a video"), store))
            assertEquals(VideoDisplay.Playable("file://" + store.file(id)!!.absolutePath), VideoDisplay.of(JSONValue.Str("video:here"), store))
            assertEquals(VideoDisplay.Playable("https://e.com/a.mp4"), VideoDisplay.of(JSONValue.Str("https://e.com/a.mp4"), store))
            val thumb = "data:image/jpeg;base64,AAAA"
            assertEquals(VideoDisplay.Elsewhere(thumb), VideoDisplay.of(JSONValue.Str("video:other|$thumb"), store))
        } finally {
            dir.deleteRecursively()
        }
    }
}
