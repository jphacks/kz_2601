package jp.kz2601.metamon.desktop

import androidx.compose.ui.graphics.ImageBitmap
import androidx.compose.ui.graphics.toComposeImageBitmap
import jp.kz2601.metamon.desktop.io.DragOut
import jp.kz2601.metamon.desktop.io.ImageSource
import org.jetbrains.skia.Image
import org.jetbrains.skia.Rect
import org.jetbrains.skia.SamplingMode
import org.jetbrains.skia.Surface
import kotlin.math.roundToInt

/**
 * 読んだ画像。[bitmap] は描く絵([ImageLoader.load] の maxPx で縮めた物)、
 * [png] は外へ引き出す時の PNG(読んだ時に裏で変換しておく・#1764)。PNG にできない画像は null(引き出さない)。
 */
class LoadedImage(val bitmap: ImageBitmap, val png: ByteArray?)

/** 画像を読む(呼ぶのは裏のスレッドから)。https・data URL は URL ごとに少しだけ覚える(古い物から捨てる)。 */
object ImageLoader {
    /** 一覧の写真(44dp)を高解像度の画面でも粗くならない大きさ。 */
    const val THUMBNAIL_PX = 160
    const val CACHE_SIZE = 64

    private data class Key(val src: String, val allowFile: Boolean, val maxPx: Int?, val withPng: Boolean)

    private val cache = object : LinkedHashMap<Key, LoadedImage>(CACHE_SIZE, 0.75f, true) {
        override fun removeEldestEntry(eldest: MutableMap.MutableEntry<Key, LoadedImage>?): Boolean = size > CACHE_SIZE
    }

    /** 手元のファイル(photoField の値)は中身が変わるので覚えない。 */
    private fun cacheable(src: String): Boolean = !src.startsWith("file:")

    fun cached(src: String?, allowFile: Boolean, maxPx: Int?, withPng: Boolean = true): LoadedImage? {
        if (src.isNullOrBlank() || !cacheable(src)) return null
        return synchronized(cache) { cache[Key(src, allowFile, maxPx, withPng)] }
    }

    /** [useCache] が false なら覚えた物を使わず、覚えもしない(写真のプレビューのような大きい絵は一覧の分を押し出さない)。 */
    fun load(src: String?, allowFile: Boolean, maxPx: Int? = null, withPng: Boolean = true, useCache: Boolean = true): LoadedImage? {
        if (src.isNullOrBlank()) return null
        if (useCache) cached(src, allowFile, maxPx, withPng)?.let { return it }
        val bytes = ImageSource.bytes(src, allowFile) ?: return null
        val loaded = decode(bytes, maxPx, withPng) ?: return null
        if (useCache && cacheable(src)) synchronized(cache) { cache[Key(src, allowFile, maxPx, withPng)] = loaded }
        return loaded
    }

    /** バイト列 → 描く絵(長い辺を [maxPx] まで縮める)+ 引き出す PNG。読めなければ null。 */
    fun decode(bytes: ByteArray, maxPx: Int? = null, withPng: Boolean = true): LoadedImage? {
        val image = runCatching { Image.makeFromEncoded(bytes) }.getOrNull() ?: return null
        val shown = runCatching { if (maxPx != null) downscale(image, maxPx) else image }.getOrNull() ?: return null
        return LoadedImage(shown.toComposeImageBitmap(), if (withPng) DragOut.png(bytes) else null)
    }

    /** 長い辺が [maxPx] を超える時だけ縮める(縦横比はそのまま)。 */
    fun downscale(image: Image, maxPx: Int): Image {
        val long = maxOf(image.width, image.height)
        if (long <= maxPx) return image
        val scale = maxPx.toFloat() / long
        val w = (image.width * scale).roundToInt().coerceAtLeast(1)
        val h = (image.height * scale).roundToInt().coerceAtLeast(1)
        val surface = Surface.makeRasterN32Premul(w, h)
        surface.canvas.drawImageRect(
            image,
            Rect.makeWH(image.width.toFloat(), image.height.toFloat()),
            Rect.makeWH(w.toFloat(), h.toFloat()),
            SamplingMode.LINEAR,
            null,
            true,
        )
        return surface.makeImageSnapshot()
    }

    internal fun clear() = synchronized(cache) { cache.clear() }

    internal fun size(): Int = synchronized(cache) { cache.size }
}
