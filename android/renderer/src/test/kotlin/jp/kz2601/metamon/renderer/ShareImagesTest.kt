package jp.kz2601.metamon.renderer

import jp.kz2601.metamon.dsl.logic.ShareTarget
import jp.kz2601.metamon.dsl.records.Entity
import jp.kz2601.metamon.dsl.records.Field
import jp.kz2601.metamon.dsl.records.FieldType
import jp.kz2601.metamon.dsl.records.JSONValue
import jp.kz2601.metamon.renderer.components.actions.QrMatrix
import jp.kz2601.metamon.renderer.components.actions.ShareAction
import jp.kz2601.metamon.renderer.components.actions.ShareImages
import jp.kz2601.metamon.renderer.form.FormDraft
import org.junit.Assert.assertArrayEquals
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.rules.TemporaryFolder
import java.io.File
import java.util.Base64

/** #1086: shareButton の写真・qrCode の保存と共有の決まり(何を送るか・一時ファイル・写真に足す時の値)。 */
class ShareImagesTest {
    @get:Rule val tmp = TemporaryFolder()

    private val entity = Entity(
        id = "shop",
        fields = listOf(
            Field("name", type = FieldType.STRING),
            Field("photo", type = FieldType.PHOTO),
        ),
    )
    private val jpeg = byteArrayOf(0xFF.toByte(), 0xD8.toByte(), 1, 2, 3)
    private val dataUrl = "data:image/jpeg;base64," + Base64.getEncoder().encodeToString(jpeg)

    private fun form(vararg values: Pair<String, String>) = FormDraft(entity).also { f ->
        values.forEach { (k, v) -> f.values[k] = JSONValue.Str(v) }
    }

    @Test
    fun photoFieldSharesPhotoWithText() {
        val f = form("name" to "麺屋", "photo" to dataUrl)
        assertEquals(ShareAction.Photo("麺屋", dataUrl), ShareAction.of(ShareTarget.ANY, "{{name}}", "photo", f))
        assertEquals("slack も共有シートなので写真を載せる", ShareAction.Photo("💬 麺屋", dataUrl), ShareAction.of(ShareTarget.SLACK, "{{name}}", "photo", f))
    }

    @Test
    fun directTargetsStayTextOnly() {
        val f = form("name" to "麺屋", "photo" to dataUrl)
        assertTrue("LINE は URL なので写真は渡せない", ShareAction.of(ShareTarget.LINE, "{{name}}", "photo", f) is ShareAction.Direct)
    }

    @Test
    fun noPhotoFallsBackToText() {
        assertEquals(ShareAction.Sheet("a"), ShareAction.of(ShareTarget.ANY, "a", "photo", form()))
        assertEquals("https は文面だけ(iOS と同じ)", ShareAction.Sheet("a"), ShareAction.of(ShareTarget.ANY, "a", "photo", form("photo" to "https://example.com/a.jpg")))
        assertEquals("写真でない field は文面に足す", ShareAction.Sheet("a\n麺屋"), ShareAction.of(ShareTarget.ANY, "a", "name", form("name" to "麺屋")))
        assertNull(ShareImages.photoDataUrl("photo", null))
        assertNull(ShareImages.photoDataUrl("missing", form("photo" to dataUrl)))
        assertNull("壊れた data URL は送らない", ShareImages.photoDataUrl("photo", form("photo" to "data:image/jpeg;base64,")))
    }

    @Test
    fun qrExportScaleTargets1024() {
        assertEquals(31, ShareImages.qrExportScale(33)) // 33 × 31 = 1023
        assertEquals(1, ShareImages.qrExportScale(2000))
        assertEquals(1024, ShareImages.qrExportScale(0))
        val m = QrMatrix.encode("https://example.com")!!
        assertTrue(m.size * ShareImages.qrExportScale(m.size) in 1000..1024)
    }

    @Test
    fun galleryEntryIsPngInPictures() {
        val e = ShareImages.qrGalleryEntry(1_700_000_000_000)
        assertEquals("qrcode-1700000000000.png", e.displayName)
        assertEquals("image/png", e.mimeType)
        assertEquals("Pictures", e.relativePath)
    }

    @Test
    fun writeFileReplacesPreviousShare() {
        val cache = tmp.newFolder("cache")
        val first = ShareImages.writeFile(cache, ShareImages.PHOTO_FILE, jpeg)
        assertArrayEquals(jpeg, first.readBytes())
        assertEquals(File(cache, ShareImages.DIR), first.parentFile!!.parentFile)
        val second = ShareImages.writeFile(cache, ShareImages.QR_FILE, byteArrayOf(9))
        assertFalse("前の共有のファイルは次で消す", first.exists())
        assertEquals("qrcode.png", second.name)
        assertTrue(second.exists())
    }

    @Test
    fun removeAllLeavesOtherCache() {
        val cache = tmp.newFolder("cache")
        val shared = ShareImages.writeFile(cache, ShareImages.QR_FILE, byteArrayOf(1))
        val other = File(cache, "metamon_export").apply { mkdirs() }
        ShareImages.removeAll(cache)
        assertFalse(shared.exists())
        assertFalse(File(cache, ShareImages.DIR).exists())
        assertTrue(other.exists())
    }
}
