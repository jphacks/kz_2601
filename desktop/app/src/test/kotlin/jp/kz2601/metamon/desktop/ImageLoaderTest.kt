package jp.kz2601.metamon.desktop

import org.jetbrains.skia.Image
import org.jetbrains.skia.Surface
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertSame
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test
import java.awt.Color
import java.awt.image.BufferedImage
import java.io.ByteArrayOutputStream
import java.util.Base64
import javax.imageio.ImageIO

/** 画像の読み込みのキャッシュの規則と縮小(#2614)。 */
class ImageLoaderTest {
    @Before
    fun setUp() = ImageLoader.clear()

    @After
    fun tearDown() = ImageLoader.clear()

    private fun png(w: Int, h: Int): ByteArray {
        val img = BufferedImage(w, h, BufferedImage.TYPE_INT_RGB)
        img.createGraphics().apply { color = Color.RED; fillRect(0, 0, w, h) }.dispose()
        return ByteArrayOutputStream().also { ImageIO.write(img, "png", it) }.toByteArray()
    }

    private fun dataUrl(w: Int = 2, h: Int = 2) = "data:image/png;base64," + Base64.getEncoder().encodeToString(png(w, h))

    private fun image(w: Int, h: Int): Image = Surface.makeRasterN32Premul(w, h).makeImageSnapshot()

    // MARK: cacheable

    @Test
    fun cacheableSkipsLocalFiles() {
        assertTrue(ImageLoader.cacheable("data:image/png;base64,AAAA"))
        assertTrue(ImageLoader.cacheable("https://example.com/a.png"))
        // 手元のファイルは中身が変わるので置かない
        assertFalse(ImageLoader.cacheable("file:/tmp/a.png"))
        assertFalse(ImageLoader.cacheable("file:///tmp/a.png"))
        assertNull(ImageLoader.cached("file:/tmp/a.png", allowFile = true))
        assertNull(ImageLoader.cached(null, allowFile = false))
        assertNull(ImageLoader.cached(" ", allowFile = false))
    }

    @Test
    fun dataUrlIsCachedPerKey() {
        val src = dataUrl()
        val first = ImageLoader.load(src, allowFile = false, withPng = false)!!
        assertEquals(1, ImageLoader.size())
        assertSame(first, ImageLoader.cached(src, allowFile = false, withPng = false))
        assertSame("2 回目は覚えた物", first, ImageLoader.load(src, allowFile = false, withPng = false))
        // 鍵(allowFile・maxPx・withPng)が違えば別に読む
        assertNull(ImageLoader.cached(src, allowFile = false, withPng = true))
        assertNull(ImageLoader.cached(src, allowFile = false, maxPx = ImageLoader.THUMBNAIL_PX, withPng = false))
        assertNull(ImageLoader.cached(src, allowFile = true, withPng = false))
        // useCache = false は覚えた物を使わず、覚えもしない
        assertNotNull(ImageLoader.load(dataUrl(3, 3), allowFile = false, withPng = false, useCache = false))
        assertEquals(1, ImageLoader.size())
    }

    @Test
    fun unreadableFileIsNotCached() {
        assertNull(ImageLoader.load("file:/nonexistent/a.png", allowFile = true))
        assertEquals(0, ImageLoader.size())
    }

    // MARK: 古い物から捨てる

    @Test
    fun evictsLeastRecentlyUsedOverLimit() {
        val src = dataUrl()
        // maxPx を変えると鍵が別になるので、同じ data URL で上限ちょうどまで入れる
        repeat(ImageLoader.CACHE_SIZE) { ImageLoader.load(src, allowFile = false, maxPx = it + 1, withPng = false) }
        assertEquals(ImageLoader.CACHE_SIZE, ImageLoader.size())
        // 一番古い物(maxPx = 1)を使うと新しい側へ回る
        assertNotNull(ImageLoader.cached(src, allowFile = false, maxPx = 1, withPng = false))
        ImageLoader.load(src, allowFile = false, maxPx = 1000, withPng = false)
        assertEquals("上限を超えない", ImageLoader.CACHE_SIZE, ImageLoader.size())
        // 捨てられるのは使っていない一番古い物(maxPx = 2)
        assertNull(ImageLoader.cached(src, allowFile = false, maxPx = 2, withPng = false))
        assertNotNull(ImageLoader.cached(src, allowFile = false, maxPx = 1, withPng = false))
        assertNotNull(ImageLoader.cached(src, allowFile = false, maxPx = 1000, withPng = false))
        ImageLoader.clear()
        assertEquals(0, ImageLoader.size())
    }

    // MARK: downscale

    @Test
    fun downscaleFitsLongSideToMaxPx() {
        val wide = ImageLoader.downscale(image(3200, 1600), 1600)
        assertEquals(1600, wide.width)
        assertEquals(800, wide.height)
        val tall = ImageLoader.downscale(image(300, 900), 300)
        assertEquals(100, tall.width)
        assertEquals(300, tall.height)
        // 細い辺は 0 にせず 1 は残す
        val thin = ImageLoader.downscale(image(4000, 2), 100)
        assertEquals(100, thin.width)
        assertEquals(1, thin.height)
    }

    @Test
    fun downscaleKeepsSmallImages() {
        val small = image(100, 50)
        assertSame(small, ImageLoader.downscale(small, 1600))
        val exact = image(160, 90)
        assertSame("ちょうど maxPx も縮めない", exact, ImageLoader.downscale(exact, 160))
        val loaded = ImageLoader.decode(png(40, 20), maxPx = ImageLoader.THUMBNAIL_PX, withPng = false)!!
        assertEquals(40, loaded.bitmap.width)
        assertEquals(20, loaded.bitmap.height)
        assertNull(loaded.png)
    }
}
