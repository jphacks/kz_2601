package jp.kz2601.metamon.desktop.io

import jp.kz2601.metamon.desktop.Examples
import jp.kz2601.metamon.desktop.MockStore
import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.ComponentType
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertTrue
import org.junit.Test
import java.awt.datatransfer.DataFlavor
import java.awt.datatransfer.Transferable
import java.awt.datatransfer.UnsupportedFlavorException
import java.awt.image.BufferedImage
import java.io.File

/** Ctrl+V・ドロップ: 文字もある時は画像を取らない・書けるフォームが無ければ写真・動画を入れない。 */
class PasteDropTest {
    private val io = DesktopIo(Examples.load("shopping"), { MockStore() }, {})
    private val photo = Component(type = ComponentType.PHOTO_FIELD, field = "photo")
    private val video = Component(type = ComponentType.VIDEO_FIELD, field = "clip")

    private class Clip(private val data: Map<DataFlavor, Any>) : Transferable {
        override fun getTransferDataFlavors(): Array<DataFlavor> = data.keys.toTypedArray()
        override fun isDataFlavorSupported(flavor: DataFlavor): Boolean = flavor in data
        override fun getTransferData(flavor: DataFlavor): Any = data[flavor] ?: throw UnsupportedFlavorException(flavor)
    }

    private val image = BufferedImage(4, 4, BufferedImage.TYPE_INT_ARGB)

    @Test
    fun spreadsheetCellsPasteAsTextNotImage() {
        val draft = mutableMapOf<String, String>()
        val cells = Clip(mapOf(DataFlavor.stringFlavor to "牛乳\t2", DataFlavor.imageFlavor to image))
        assertFalse("文字もある時は入力欄の貼り付けに任せる", io.paste(cells, listOf(photo), draft))
        assertTrue(draft.isEmpty())
    }

    @Test
    fun imageOnlyClipboardFillsThePhotoField() {
        val draft = mutableMapOf<String, String>()
        assertTrue(io.paste(Clip(mapOf(DataFlavor.imageFlavor to image)), listOf(photo), draft))
        assertNotNull(draft["photo"])
        draft["photo"]?.let { fileOf(it)?.delete() }
    }

    @Test
    fun noWritableFormMeansNoMedia() {
        assertFalse("フォームの無い画面に画像は入らない", io.drop(Clip(mapOf(DataFlavor.imageFlavor to image)), listOf(photo), null))
        val f = File.createTempFile("clip-", ".mp4").apply { deleteOnExit() }
        assertFalse("動画も入らない", io.dropFiles(listOf(f), listOf(video), null))
        assertNotNull("落とせない旨", io.notice)
        val draft = mutableMapOf<String, String>()
        assertTrue(io.dropFiles(listOf(f), listOf(video), draft))
        assertEquals(f.toURI().toString(), draft["clip"])
    }
}
