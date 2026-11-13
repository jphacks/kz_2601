package jp.kz2601.metamon.desktop.share

import com.google.zxing.BarcodeFormat
import com.google.zxing.BinaryBitmap
import com.google.zxing.DecodeHintType
import com.google.zxing.MultiFormatReader
import com.google.zxing.RGBLuminanceSource
import com.google.zxing.common.HybridBinarizer
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.rules.TemporaryFolder
import java.awt.datatransfer.Clipboard
import java.awt.datatransfer.DataFlavor
import java.awt.image.BufferedImage
import java.io.ByteArrayInputStream
import javax.imageio.ImageIO

/** 共有のリンクの QR(#3205): zxing で読み戻すとリンクに戻る・PNG の保存は書き出しと同じ名前の決まり・クリップボードへ画像で。 */
class ShareQr3205Test {
    @get:Rule
    val tmp = TemporaryFolder()

    private val link = "https://tailorit.app/s/AbC123xyz?invite=9f8e7d"

    private fun read(image: BufferedImage): String {
        val px = IntArray(image.width * image.height)
        image.getRGB(0, 0, image.width, image.height, px, 0, image.width)
        val hints = mapOf(DecodeHintType.POSSIBLE_FORMATS to listOf(BarcodeFormat.QR_CODE), DecodeHintType.CHARACTER_SET to "UTF-8")
        return MultiFormatReader().decode(BinaryBitmap(HybridBinarizer(RGBLuminanceSource(image.width, image.height, px))), hints).text
    }

    @Test
    fun shareQrRoundTripsToTheLink() {
        val png = ShareQr.png(link)
        assertNotNull(png)
        val image = ImageIO.read(ByteArrayInputStream(png))
        assertEquals(link, read(image))
        assertEquals("周りに余白を持った正方形", image.width, image.height)
        assertEquals(0, image.width % ShareQr.MODULE_PX)
        assertEquals("左上は余白(白)", 0xFFFFFF, image.getRGB(0, 0) and 0xFFFFFF)
    }

    @Test
    fun nonAsciiLinkRoundTrips() {
        val url = "https://tailorit.app/s/家計簿?name=やまだ"
        assertEquals(url, read(ShareQr.image(url)!!))
    }

    @Test
    fun blankOrTooLongIsNotAQr() {
        assertNull(ShareQr.png("  "))
        assertNull(ShareQr.image("https://x/" + "a".repeat(5_000)))
    }

    @Test
    fun savedPngFollowsExportNaming() {
        val dir = tmp.newFolder("Downloads")
        val first = ShareQr.save(link, dir, "家計簿 の共有の QR")!!
        val second = ShareQr.save(link, dir, "家計簿 の共有の QR")!!
        assertEquals("家計簿 の共有の QR.png", first.name)
        assertEquals("既存は上書きしない", "家計簿 の共有の QR (2).png", second.name)
        assertEquals(link, read(ImageIO.read(second)))
        assertEquals("_CON.png", ShareQr.fileName("CON"))
        assertEquals("a_b_c.png", ShareQr.fileName("a/b:c"))
        assertEquals("share-qr.png", ShareQr.fileName(""))
        assertNull(ShareQr.save("", dir, "x"))
        assertFalse("QR にできない時はファイルを作らない", dir.resolve("x.png").exists())
    }

    @Test
    fun copyPutsAnImageOnTheClipboard() {
        val cb = Clipboard("test")
        assertTrue(ShareQr.copy(link, cb))
        val image = cb.getData(DataFlavor.imageFlavor) as BufferedImage
        assertEquals(link, read(image))
        assertFalse(ShareQr.copy(link, null))
        assertFalse(ShareQr.copy("", cb))
    }
}
