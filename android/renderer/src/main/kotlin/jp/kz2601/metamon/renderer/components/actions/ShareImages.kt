package jp.kz2601.metamon.renderer.components.actions

import android.content.ActivityNotFoundException
import android.content.ClipData
import android.content.ContentValues
import android.content.Context
import android.content.Intent
import android.graphics.Bitmap
import android.provider.MediaStore
import androidx.core.content.FileProvider
import jp.kz2601.metamon.dsl.records.FieldType
import jp.kz2601.metamon.dsl.records.JSONValue
import jp.kz2601.metamon.renderer.components.PhotoValue
import jp.kz2601.metamon.renderer.form.FormDraft
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import java.io.ByteArrayOutputStream
import java.io.File
import java.util.UUID

/**
 * shareButton の写真・qrCode の画像を共有シートと写真に渡す決まり(#1086)。Android の部品に触らない部分は JVM で試す。
 * 共有の一時ファイルは cache/metamon_share(FileProvider の metamon_share)。次の共有とデータの削除で消す(export と同じ)。
 */
object ShareImages {
    const val DIR = "metamon_share"
    const val JPEG = "image/jpeg"
    const val PNG = "image/png"
    const val PHOTO_FILE = "photo.jpg"
    const val QR_FILE = "qrcode.png"

    /** 保存する QR の 1 辺の目安(iOS の `QRCodeImage.exportSide`)。 */
    const val QR_EXPORT_SIDE = 1024

    /** 写真に保存する置き場(Environment.DIRECTORY_PICTURES)。 */
    const val GALLERY_DIR = "Pictures"

    /** `field` が photo で、値が data URL の写真なら、その値(iOS の `ShareButtonLeaf.photo`。https は文面だけ)。 */
    fun photoDataUrl(field: String?, form: FormDraft?): String? {
        val f = field?.let { form?.entity?.field(it) } ?: return null
        if (f.type != FieldType.PHOTO) return null
        val raw = (form!!.values[f.id] as? JSONValue.Str)?.value ?: return null
        return raw.takeIf { PhotoValue.dataBytes(it) != null }
    }

    /** 1 モジュールの px(余白込みの辺が [QR_EXPORT_SIDE] 前後になる倍率・最低 1)。 */
    fun qrExportScale(side: Int): Int = maxOf(1, QR_EXPORT_SIDE / maxOf(1, side))

    /** 写真に追加する時の名前・種類・置き場(MediaStore の DISPLAY_NAME・MIME_TYPE・RELATIVE_PATH)。 */
    data class GalleryEntry(val displayName: String, val mimeType: String, val relativePath: String)

    fun qrGalleryEntry(nowMillis: Long): GalleryEntry = GalleryEntry("qrcode-$nowMillis.png", PNG, GALLERY_DIR)

    /** [cacheDir]/metamon_share/<uuid>/<名前> に書く。前回の共有のファイルは先に消す(共有シートが読み終える時が分からないので次で消す)。 */
    fun writeFile(cacheDir: File, fileName: String, bytes: ByteArray): File {
        val root = File(cacheDir, DIR)
        root.deleteRecursively()
        val dir = File(root, UUID.randomUUID().toString()).apply { mkdirs() }
        return File(dir, fileName).apply { writeBytes(bytes) }
    }

    /** 共有の一時ファイルを全部消す(アカウントの削除・サインアウト)。 */
    fun removeAll(cacheDir: File) {
        File(cacheDir, DIR).deleteRecursively()
    }
}

/** 保存・共有に使う QR の PNG(白地に黒・補間なしで拡大)。 */
fun QrMatrix.exportPng(): ByteArray {
    val scale = ShareImages.qrExportScale(size)
    val side = size * scale
    val pixels = IntArray(side * side) { i -> if (this[(i % side) / scale, (i / side) / scale]) 0xFF000000.toInt() else 0xFFFFFFFF.toInt() }
    val bitmap = Bitmap.createBitmap(pixels, side, side, Bitmap.Config.ARGB_8888)
    return ByteArrayOutputStream().use { out ->
        bitmap.compress(Bitmap.CompressFormat.PNG, 100, out)
        bitmap.recycle()
        out.toByteArray()
    }
}

/** 画像を共有シートで(FileProvider で読ませる)。[text] があれば文面も載せる。書けなければ文面だけ。 */
fun shareImage(context: Context, bytes: ByteArray, fileName: String, mimeType: String, text: String?, title: String?) {
    val file = runCatching { ShareImages.writeFile(context.cacheDir, fileName, bytes) }.getOrNull()
    if (file == null) {
        text?.takeIf { it.isNotEmpty() }?.let { shareText(context, it, title) }
        return
    }
    val uri = FileProvider.getUriForFile(context, "${context.packageName}.metamon.media", file)
    val send = Intent(Intent.ACTION_SEND).setType(mimeType).putExtra(Intent.EXTRA_STREAM, uri)
        .addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION)
    text?.takeIf { it.isNotEmpty() }?.let { send.putExtra(Intent.EXTRA_TEXT, it) }
    send.clipData = ClipData.newRawUri(title, uri) // 共有シートのプレビューと、受け取るアプリへの読み取りの許可
    val chooser = Intent.createChooser(send, title)
        .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK or Intent.FLAG_GRANT_READ_URI_PERMISSION)
    try {
        context.startActivity(chooser)
    } catch (_: ActivityNotFoundException) {
        // 共有先が 1 つも無い端末では何もしない
    }
}

/** 端末の写真(Pictures)に追加する。Android 10 以降は自分で足すだけなら権限が要らない。@return 保存できたか */
suspend fun saveImageToGallery(context: Context, entry: ShareImages.GalleryEntry, bytes: ByteArray): Boolean = withContext(Dispatchers.IO) {
    val resolver = context.contentResolver
    val values = ContentValues().apply {
        put(MediaStore.Images.Media.DISPLAY_NAME, entry.displayName)
        put(MediaStore.Images.Media.MIME_TYPE, entry.mimeType)
        put(MediaStore.Images.Media.RELATIVE_PATH, entry.relativePath)
        put(MediaStore.Images.Media.IS_PENDING, 1)
    }
    val collection = MediaStore.Images.Media.getContentUri(MediaStore.VOLUME_EXTERNAL_PRIMARY)
    val uri = runCatching { resolver.insert(collection, values) }.getOrNull() ?: return@withContext false
    val ok = runCatching { resolver.openOutputStream(uri)?.use { it.write(bytes) } != null }.getOrDefault(false)
    if (ok) {
        runCatching { resolver.update(uri, ContentValues().apply { put(MediaStore.Images.Media.IS_PENDING, 0) }, null, null) }
    } else {
        runCatching { resolver.delete(uri, null, null) }
    }
    ok
}
