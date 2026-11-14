package jp.kz2601.metamon.renderer.components.media

import android.content.Context
import android.graphics.Bitmap
import android.graphics.BitmapFactory
import android.graphics.Matrix
import android.media.ExifInterface
import android.net.Uri
import java.io.ByteArrayOutputStream
import java.util.Base64
import kotlin.math.floor
import kotlin.math.max
import kotlin.math.min

/**
 * photo field の値("data:image/jpeg;base64,…")を作る(iOS の `PhotoCodec`)。
 * 長い辺 800px 以下・JPEG 品質 50(iOS の q0.5)。EXIF の向きは画素に焼き込んで正立させる。
 * 計算だけの部分は android.* に触れない(JVM テスト)。
 */
object PhotoCodec {
    const val MAX_SIDE = 800
    const val QUALITY = 50
    const val PREFIX = "data:image/jpeg;base64,"

    /** 縮めた後の大きさ(iOS と同じく切り捨て・拡大はしない)。 */
    fun targetSize(width: Int, height: Int, maxSide: Int = MAX_SIDE): Pair<Int, Int> {
        val ratio = min(1.0, maxSide.toDouble() / max(width, height))
        return max(1, floor(width * ratio).toInt()) to max(1, floor(height * ratio).toInt())
    }

    /** 読み込み時の間引き(2 の冪)。間引いた後も長い辺が [maxSide] 以上残る最大の値。向きに依らない。 */
    fun sampleSize(width: Int, height: Int, maxSide: Int = MAX_SIDE): Int {
        val long = max(width, height)
        var n = 1
        while (long / (n * 2) >= maxSide) n *= 2
        return n
    }

    fun dataUrl(jpeg: ByteArray): String = PREFIX + Base64.getEncoder().encodeToString(jpeg)

    /** 端末の画像(Photo Picker / カメラの保存先)→ 値。読めなければ null。重いので IO のスレッドで呼ぶ。 */
    fun dataUrl(context: Context, uri: Uri, maxSide: Int = MAX_SIDE, quality: Int = QUALITY): String? = runCatching {
        val resolver = context.contentResolver
        val orientation = resolver.openInputStream(uri)?.use {
            ExifInterface(it).getAttributeInt(ExifInterface.TAG_ORIENTATION, ExifInterface.ORIENTATION_NORMAL)
        } ?: ExifInterface.ORIENTATION_NORMAL
        val bounds = BitmapFactory.Options().apply { inJustDecodeBounds = true }
        resolver.openInputStream(uri)?.use { BitmapFactory.decodeStream(it, null, bounds) }
        if (bounds.outWidth <= 0 || bounds.outHeight <= 0) return@runCatching null
        val opts = BitmapFactory.Options().apply { inSampleSize = sampleSize(bounds.outWidth, bounds.outHeight, maxSide) }
        val bitmap = resolver.openInputStream(uri)?.use { BitmapFactory.decodeStream(it, null, opts) } ?: return@runCatching null
        dataUrl(bitmap, orientation, maxSide, quality)
    }.getOrNull()

    /** 絵 → 値。[orientation] は EXIF の値(1〜8)。 */
    fun dataUrl(bitmap: Bitmap, orientation: Int = ExifRotation.NORMAL, maxSide: Int = MAX_SIDE, quality: Int = QUALITY): String? {
        val (w, h) = targetSize(bitmap.width, bitmap.height, maxSide)
        val t = ExifRotation.of(orientation)
        val matrix = Matrix().apply {
            postScale(w.toFloat() / bitmap.width, h.toFloat() / bitmap.height)
            if (t.degrees != 0) postRotate(t.degrees.toFloat())
            if (t.mirror) postScale(-1f, 1f)
        }
        val out = Bitmap.createBitmap(bitmap, 0, 0, bitmap.width, bitmap.height, matrix, true)
        val bytes = ByteArrayOutputStream().use { s ->
            if (!out.compress(Bitmap.CompressFormat.JPEG, quality, s)) return null
            s.toByteArray()
        }
        return dataUrl(bytes)
    }
}

/** EXIF の向き(1〜8)→ 回す角度(時計回り)と、回した後に左右を反転するか。 */
object ExifRotation {
    const val NORMAL = 1

    data class Transform(val degrees: Int, val mirror: Boolean)

    fun of(orientation: Int): Transform = when (orientation) {
        2 -> Transform(0, true)
        3 -> Transform(180, false)
        4 -> Transform(180, true)
        5 -> Transform(90, true)
        6 -> Transform(90, false)
        7 -> Transform(270, true)
        8 -> Transform(270, false)
        else -> Transform(0, false)
    }

    /** 正立させると縦横が入れ替わるか(5〜8)。 */
    fun swapsSides(orientation: Int): Boolean = of(orientation).degrees % 180 != 0
}
