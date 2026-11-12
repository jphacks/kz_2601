package jp.kz2601.metamon.desktop

import androidx.compose.runtime.mutableStateMapOf
import androidx.compose.ui.graphics.ImageBitmap
import androidx.compose.ui.graphics.toComposeImageBitmap
import jp.kz2601.metamon.desktop.io.DragOut
import jp.kz2601.metamon.desktop.io.ImageSource
import jp.kz2601.metamon.desktop.net.DesktopApiConfig
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

    /** 描く絵の長い辺の既定(maxPx を渡さない時も、元の大きさのまま持たない)。 */
    const val DISPLAY_PX = 1600

    /** 開く画像の画素数の上限(約 40 MP)。超える画像は中身を開かない。 */
    const val MAX_PIXELS = ImageSource.MAX_PIXELS

    /**
     * 記録の値の https の画像を、押さずに読んでよい場所(アプリの置き場)。
     * appconfig・`TAILORIT_API_BASE` のホストと、`TAILORIT_MEDIA_HOSTS`(カンマ区切り)。
     */
    val trustedHosts: Set<String> by lazy { hostsFrom(System.getenv()) }

    internal fun hostsFrom(env: Map<String, String>): Set<String> {
        val urls = listOfNotNull(DesktopApiConfig.fromEnvironment(env).appConfigUrl, env["TAILORIT_API_BASE"])
        val fromUrls = urls.mapNotNull { runCatching { java.net.URI(it.trim()).host }.getOrNull() }
        val extra = env["TAILORIT_MEDIA_HOSTS"].orEmpty().split(',').map { it.trim() }.filter { it.isNotEmpty() }
        return (fromUrls + extra).map { it.lowercase() }.toSet()
    }

    /** 「画像を読む」を押した記録の画像(画面を描き直すので state)。 */
    private val approved = mutableStateMapOf<String, Boolean>()

    /** 記録の値([fromRecord])の https の画像で、アプリの置き場の外の物は押すまで読まない。 */
    fun needsConsent(src: String?, fromRecord: Boolean, hosts: Set<String> = trustedHosts): Boolean {
        if (!fromRecord || src.isNullOrBlank() || !src.startsWith("https://")) return false
        val host = runCatching { java.net.URI(src).host?.lowercase() }.getOrNull() ?: return true
        return host !in hosts
    }

    /** まだ押されていない、読まない画像か(描く側はこの時「画像を読む」を出す)。 */
    fun blocked(src: String?, fromRecord: Boolean): Boolean = needsConsent(src, fromRecord) && approved[src] != true

    fun approve(src: String) {
        approved[src] = true
    }

    private data class Key(val src: String, val allowFile: Boolean, val maxPx: Int, val withPng: Boolean)

    private val cache = object : LinkedHashMap<Key, LoadedImage>(CACHE_SIZE, 0.75f, true) {
        override fun removeEldestEntry(eldest: MutableMap.MutableEntry<Key, LoadedImage>?): Boolean = size > CACHE_SIZE
    }

    /** 手元のファイル(photoField の値)は中身が変わるので覚えない。 */
    private fun cacheable(src: String): Boolean = !src.startsWith("file:")

    fun cached(src: String?, allowFile: Boolean, maxPx: Int = DISPLAY_PX, withPng: Boolean = true): LoadedImage? {
        if (src.isNullOrBlank() || !cacheable(src)) return null
        return synchronized(cache) { cache[Key(src, allowFile, maxPx, withPng)] }
    }

    /**
     * [allowFile] は記録・フォームの値の時(その時の https は [needsConsent] を通す)。
     * [useCache] が false なら覚えた物を使わず、覚えもしない(写真のプレビューのような大きい絵は一覧の分を押し出さない)。
     */
    fun load(src: String?, allowFile: Boolean, maxPx: Int = DISPLAY_PX, withPng: Boolean = true, useCache: Boolean = true): LoadedImage? {
        if (src.isNullOrBlank()) return null
        if (blocked(src, fromRecord = allowFile)) return null
        if (useCache) cached(src, allowFile, maxPx, withPng)?.let { return it }
        val bytes = ImageSource.bytes(src, allowFile) ?: return null
        val loaded = decode(bytes, maxPx, withPng) ?: return null
        if (useCache && cacheable(src)) synchronized(cache) { cache[Key(src, allowFile, maxPx, withPng)] = loaded }
        return loaded
    }

    /** バイト列 → 描く絵(長い辺を [maxPx] まで縮める)+ 引き出す PNG。読めない・画素が多すぎる時は null。 */
    fun decode(bytes: ByteArray, maxPx: Int = DISPLAY_PX, withPng: Boolean = true): LoadedImage? {
        if (!ImageSource.withinPixelLimit(bytes, MAX_PIXELS)) return null
        val image = runCatching { Image.makeFromEncoded(bytes) }.getOrNull() ?: return null
        val shown = runCatching { downscale(image, maxPx) }.getOrNull() ?: return null
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

    /** 覚えた絵と「読む」を押した印を捨てる(このパソコンのデータを消す時も)。 */
    fun clear() {
        synchronized(cache) { cache.clear() }
        runCatching { approved.clear() }
    }

    internal fun size(): Int = synchronized(cache) { cache.size }
}
