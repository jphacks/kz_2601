package jp.kz2601.metamon.desktop

import jp.kz2601.metamon.desktop.store.ThumbnailStore
import org.jetbrains.skia.Color
import org.jetbrains.skia.Image
import org.jetbrains.skia.Surface
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Test

/** 行のサムネイル(#3204): 画面の写しを縮めた PNG にして置き場に入れ、パレットのアプリの行が読み戻せる。 */
class AppThumbnails3204Test {
    private fun screen(w: Int, h: Int): Image {
        val surface = Surface.makeRasterN32Premul(w, h)
        surface.canvas.clear(Color.makeRGB(30, 120, 200))
        return surface.makeImageSnapshot()
    }

    @Test
    fun capturedScreenIsShrunkToPng() {
        val png = ThumbnailImage.png(screen(1440, 2460))
        assertNotNull(png)
        val back = Image.makeFromEncoded(png!!)
        assertEquals(ThumbnailImage.MAX_PX, maxOf(back.width, back.height))
        assertEquals("縦横比はそのまま", 1440.0 / 2460, back.width.toDouble() / back.height, 0.01)
    }

    @Test
    fun smallScreenKeepsItsSize() {
        val back = Image.makeFromEncoded(ThumbnailImage.png(screen(200, 300))!!)
        assertEquals(200, back.width)
        assertEquals(300, back.height)
    }

    @Test
    fun storedThumbnailDecodesForTheRow() {
        val store = ThumbnailStore(null)
        store.put("shopping", ThumbnailImage.png(screen(480, 820))!!)
        val bitmap = ThumbnailImage.decode(store.get("shopping")!!)
        assertNotNull(bitmap)
        assertNull("壊れた物は出さない(今の見た目のまま)", ThumbnailImage.decode(byteArrayOf(1, 2, 3)))
    }

    @Test
    fun onlyAppRowsHaveAnAppId() {
        assertEquals("shopping", PaletteCommand("app:shopping", "Shopping", PaletteCommand.Kind.APP) {}.appId)
        assertNull(PaletteCommand("screen:list", "List", PaletteCommand.Kind.SCREEN) {}.appId)
        assertNull(PaletteCommand("other", "x", PaletteCommand.Kind.APP) {}.appId)
    }
}
