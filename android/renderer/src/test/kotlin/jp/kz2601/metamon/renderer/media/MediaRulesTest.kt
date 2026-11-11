package jp.kz2601.metamon.renderer.media

import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.PhotoAnalyze
import jp.kz2601.metamon.dsl.PhotoAnalyzeKind
import jp.kz2601.metamon.dsl.Source
import jp.kz2601.metamon.dsl.decode
import jp.kz2601.metamon.dsl.records.FieldType
import jp.kz2601.metamon.dsl.records.JSONValue
import jp.kz2601.metamon.dsl.records.entityModels
import jp.kz2601.metamon.renderer.components.PhotoValue
import jp.kz2601.metamon.renderer.components.media.DrawingCodec
import jp.kz2601.metamon.renderer.components.media.DrawingState
import jp.kz2601.metamon.renderer.components.media.ExifRotation
import jp.kz2601.metamon.renderer.components.media.InkPoint
import jp.kz2601.metamon.renderer.components.media.InkStroke
import jp.kz2601.metamon.renderer.components.media.PhotoCapturePlan
import jp.kz2601.metamon.renderer.components.media.PhotoClues
import jp.kz2601.metamon.renderer.components.media.PhotoCodec
import jp.kz2601.metamon.renderer.components.media.PhotoIllustrate
import jp.kz2601.metamon.renderer.components.media.PhotoReader
import jp.kz2601.metamon.renderer.components.media.PhotoReading
import jp.kz2601.metamon.renderer.components.media.PhotoSourcePolicy
import jp.kz2601.metamon.renderer.form.FormDraft
import jp.kz2601.metamon.renderer.registry.ComponentRegistry
import kotlinx.coroutines.runBlocking
import org.junit.Assert.assertArrayEquals
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/** 写真・手書き(#862)の決まり。縮小・回転・値の形は iOS の PhotoCodec / DrawingCodec と同じ。 */
class MediaRulesTest {

    @Test
    fun registryHasPhotoAndDrawing() {
        assertTrue(ComponentRegistry.standard.registeredTypes.containsAll(setOf("photoField", "drawingField")))
    }

    // 縮小(長い辺 800px・切り捨て・拡大しない)

    @Test
    fun resizesLongSideTo800LikeIos() {
        assertEquals(800 to 600, PhotoCodec.targetSize(4032, 3024))
        assertEquals(600 to 800, PhotoCodec.targetSize(3024, 4032))
        assertEquals("切り捨て", 800 to 533, PhotoCodec.targetSize(3000, 2000))
        assertEquals("小さい写真は拡大しない", 640 to 480, PhotoCodec.targetSize(640, 480))
        assertEquals("細長くても 1px は残す", 800 to 1, PhotoCodec.targetSize(10000, 2))
    }

    @Test
    fun sampleSizeKeepsAtLeastTheTargetAndIgnoresOrientation() {
        assertEquals(4, PhotoCodec.sampleSize(4032, 3024))
        assertEquals(4, PhotoCodec.sampleSize(3024, 4032))
        assertEquals(1, PhotoCodec.sampleSize(1200, 900))
        assertEquals(2, PhotoCodec.sampleSize(1600, 1200))
        assertEquals(1, PhotoCodec.sampleSize(640, 480))
    }

    @Test
    fun qualityMatchesIos() {
        assertEquals("iOS の写真は q0.5", 50, PhotoCodec.QUALITY)
        assertEquals("iOS の手書きは q0.7", 70, DrawingCodec.QUALITY)
        assertEquals(800, DrawingCodec.MAX_SIDE)
    }

    // 回転(EXIF)

    @Test
    fun exifOrientationsBecomeRotationAndMirror() {
        assertEquals(ExifRotation.Transform(0, false), ExifRotation.of(1))
        assertEquals(ExifRotation.Transform(0, true), ExifRotation.of(2))
        assertEquals(ExifRotation.Transform(180, false), ExifRotation.of(3))
        assertEquals(ExifRotation.Transform(180, true), ExifRotation.of(4))
        assertEquals(ExifRotation.Transform(90, true), ExifRotation.of(5))
        assertEquals("縦持ちのカメラ", ExifRotation.Transform(90, false), ExifRotation.of(6))
        assertEquals(ExifRotation.Transform(270, true), ExifRotation.of(7))
        assertEquals(ExifRotation.Transform(270, false), ExifRotation.of(8))
        assertEquals("知らない値はそのまま", ExifRotation.Transform(0, false), ExifRotation.of(0))
        assertEquals(listOf(5, 6, 7, 8), (0..9).filter(ExifRotation::swapsSides))
    }

    // 値の形

    @Test
    fun valueIsAJpegDataUrlThatTheRestOfTheRendererReads() {
        val bytes = byteArrayOf(0xFF.toByte(), 0xD8.toByte(), 0xFF.toByte(), 0x01)
        val url = PhotoCodec.dataUrl(bytes)
        assertTrue(url.startsWith("data:image/jpeg;base64,"))
        assertEquals("data:image/jpeg;base64,/9j/AQ==", url)
        assertArrayEquals(bytes, PhotoValue.dataBytes(url))
        assertTrue(PhotoValue.hasPhoto(url))
    }

    @Test
    fun photoValueIsStoredAsIsInTheDraft() {
        val dsl = AppDSL.decode(
            """{"schemaVersion":1,"app":{"name":"t","voiceEntry":"none"},"initialScreen":"s","entities":[{"id":"e","name":"E","fields":[
               {"id":"p","name":"写真","type":"photo"},{"id":"memo","name":"メモ","type":"text"},
               {"id":"n","name":"人数","type":"integer"},{"id":"w","name":"重さ","type":"number"}]}],
               "screens":[{"id":"s","entity":"e","components":[{"type":"photoField","field":"p"}]}]}""",
        )
        val form = FormDraft(dsl.entityModels().first())
        val url = PhotoCodec.dataUrl(byteArrayOf(1, 2, 3))
        form.values["p"] = JSONValue.Str(url)
        assertEquals("iOS と同じく data URL の文字列のまま送る", JSONValue.Str(url), form.payload["p"])
        form.values["p"] = JSONValue.Null
        assertNull("消したら送らない", form.payload["p"])
    }

    // source と illustrate

    @Test
    fun sourcePlanFollowsIos() {
        assertEquals(PhotoCapturePlan.CAMERA, PhotoSourcePolicy.plan(Source.CAMERA, cameraAvailable = true))
        assertEquals(PhotoCapturePlan.LIBRARY, PhotoSourcePolicy.plan(Source.LIBRARY, cameraAvailable = true))
        assertEquals(PhotoCapturePlan.CHOOSE, PhotoSourcePolicy.plan(Source.BOTH, cameraAvailable = true))
        assertEquals("省略は both", PhotoCapturePlan.CHOOSE, PhotoSourcePolicy.plan(null, cameraAvailable = true))
        assertEquals("知らない値は both", PhotoCapturePlan.CHOOSE, PhotoSourcePolicy.plan(Source.UNKNOWN, cameraAvailable = true))
        for (s in listOf(Source.CAMERA, Source.BOTH, null)) {
            assertEquals("カメラが無ければ写真選択", PhotoCapturePlan.LIBRARY, PhotoSourcePolicy.plan(s, cameraAvailable = false))
        }
    }

    @Test
    fun illustrateIsHiddenUntilAndroidHasAnOnDeviceGenerator() {
        assertFalse(PhotoIllustrate.showsButton(true))
        assertTrue(PhotoIllustrate.showsButton(true, available = true))
        assertFalse(PhotoIllustrate.showsButton(null, available = true))
    }

    // analyze(フェイクの読み取り)

    private val entity = AppDSL.decode(
        """{"schemaVersion":1,"app":{"name":"t","voiceEntry":"none"},"initialScreen":"s","entities":[{"id":"e","name":"E","fields":[
           {"id":"p","name":"写真","type":"photo"},{"id":"memo","name":"メモ","type":"text"},
           {"id":"code","name":"コード","type":"string"},{"id":"n","name":"人数","type":"integer"},
           {"id":"total","name":"合計","type":"currency"}]}],
           "screens":[{"id":"s","entity":"e","components":[]}]}""",
    ).entityModels().first()

    private class FakeReader(val clues: PhotoClues) : PhotoReader {
        var calls = 0
        override suspend fun clues(jpeg: ByteArray, specs: List<PhotoAnalyze>): PhotoClues = clues.also { calls++ }
    }

    @Test
    fun analyzeFillsReadableFieldsAndReportsTheRest() = runBlocking {
        val reader = FakeReader(PhotoClues(lines = listOf("ありがとう", "またどうぞ"), barcodes = listOf(" 4901234567894 "), peopleCount = 3))
        val specs = listOf(
            PhotoAnalyze(PhotoAnalyzeKind.TEXT, "memo"),
            PhotoAnalyze(PhotoAnalyzeKind.BARCODE, "code"),
            PhotoAnalyze(PhotoAnalyzeKind.PEOPLE_COUNT, "n"),
            PhotoAnalyze(PhotoAnalyzeKind.AMOUNT, "total"),
            PhotoAnalyze(PhotoAnalyzeKind.TEXT, "nope"),
        )
        val out = PhotoReading.apply(reader.clues(byteArrayOf(), specs), specs, entity)
        assertEquals(1, reader.calls)
        assertEquals(JSONValue.Str("ありがとう\nまたどうぞ"), out.values["memo"])
        assertEquals("前後の空白は落とす", JSONValue.Str("4901234567894"), out.values["code"])
        assertEquals(JSONValue.Num(3.0), out.values["n"])
        assertEquals("まだ読めない kind と無い field は読めなかった扱い", listOf("total", "nope"), out.missed)
    }

    @Test
    fun readingsGoIntoTheFieldTypeLikeIos() {
        assertEquals(JSONValue.Num(1235.0), PhotoReading.jsonValue(1234.6, FieldType.CURRENCY))
        assertEquals(JSONValue.Num(1.3), PhotoReading.jsonValue(1.26, FieldType.NUMBER))
        assertEquals(JSONValue.Str("赤"), PhotoReading.jsonValue(" 赤 ", FieldType.ENUM))
        assertNull("空の文字は入れない", PhotoReading.jsonValue("  ", FieldType.TEXT))
        assertNull("型に入らない", PhotoReading.jsonValue("abc", FieldType.INTEGER))
        assertNull(PhotoReading.jsonValue(3.0, FieldType.PHOTO))
    }

    // 手書き

    private fun stroke(vararg xy: Float) = InkStroke(xy.toList().chunked(2).map { InkPoint(it[0], it[1]) }, 4f)

    @Test
    fun drawingUndoRedoAndClear() {
        var s = DrawingState()
        assertTrue(s.isEmpty)
        assertFalse(s.isChanged)
        s = s.adding(stroke(0f, 0f, 10f, 10f)).adding(stroke(100f, 100f))
        assertEquals(2, s.strokes.size)
        s = s.undone()
        assertEquals(1, s.strokes.size)
        assertTrue(s.canRedo)
        s = s.redone()
        assertEquals(2, s.strokes.size)
        s = s.adding(stroke(5f, 5f)).undone()
        assertEquals(2, s.strokes.size)
        s = s.adding(stroke(7f, 7f))
        assertFalse("新しく描いたらやり直しは消える", s.canRedo)
        assertEquals("空の線は足さない", s, s.adding(InkStroke(emptyList(), 4f)))
    }

    @Test
    fun clearAlsoRemovesTheEarlierDrawing() {
        var s = DrawingState(hasBackground = true)
        assertFalse("下絵だけでも空ではない", s.isEmpty)
        s = s.cleared()
        assertTrue(s.isEmpty)
        assertTrue(s.isChanged)
        s = s.undone()
        assertTrue("元に戻すで下絵も戻る", s.hasBackground)
        assertEquals("空なら全部消すは何もしない", DrawingState(), DrawingState().cleared())
    }

    @Test
    fun eraserRemovesWholeStrokesItTouches() {
        var s = DrawingState().adding(stroke(0f, 0f, 50f, 0f)).adding(stroke(200f, 200f, 220f, 220f))
        val before = s
        s = s.erasing(InkPoint(500f, 500f), 10f)
        assertEquals("何も触れなければ変えない", before, s)
        s = s.erasing(InkPoint(50f, 11f), 10f)
        assertEquals("線の太さの半分まで当たる", 1, s.strokes.size)
        assertEquals(200f, s.strokes[0].points[0].x)
        assertEquals(2, s.undone().strokes.size)
    }

    @Test
    fun drawingRenderScaleAndFitMatchIos() {
        assertEquals(2f, DrawingCodec.renderScale(390f, 600f))
        assertEquals("3 倍まで", 3f, DrawingCodec.renderScale(200f, 250f))
        assertEquals(1f, DrawingCodec.renderScale(1080f, 1600f))
        assertEquals(2f, DrawingCodec.renderScale(700f, 500f))
        val fit = DrawingCodec.aspectFit(400f, 200f, 300f, 300f)
        assertArrayEquals(floatArrayOf(0f, 75f, 300f, 150f), fit, 0.001f)
    }
}
