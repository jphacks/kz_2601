package jp.kz2601.metamon.switcher

import android.app.Activity
import android.content.Context
import android.content.ContextWrapper
import android.graphics.Bitmap
import android.graphics.BitmapFactory
import android.graphics.Rect
import android.os.Handler
import android.os.Looper
import android.util.Log
import android.view.PixelCopy
import androidx.compose.ui.graphics.ImageBitmap
import androidx.compose.ui.graphics.asImageBitmap
import androidx.core.view.ViewCompat
import androidx.core.view.WindowInsetsCompat
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.launch
import kotlinx.coroutines.suspendCancellableCoroutine
import java.io.ByteArrayOutputStream
import java.io.File
import kotlin.coroutines.resume

/**
 * 切り替え一覧のカードのサムネ(iOS の `AppThumbnailStore`・#1199)。`files/metamon-thumbs/<appId>.jpg` に 1/3 の大きさで保存する。
 * 撮るのは変身(apply・編集)の後(MainActivity が version の変化の 1.2 秒後に呼ぶ)。アプリごとに 1 枚で、撮り直すと上書きする。
 * [directory] が null なら端末に残さない(テスト)。
 */
class AppThumbnailStore(private val directory: File?) {
    private val memory = mutableMapOf<String, ImageBitmap>()
    private val _revision = MutableStateFlow(0)

    /** 撮るたびに進む。カードはこれを読んで描き直す。 */
    val revision: StateFlow<Int> = _revision.asStateFlow()

    /** 書き込みは 1 本ずつ(消した後に書きかけが残らないよう、消すのも同じ列に並べる)。 */
    private val io = CoroutineScope(SupervisorJob() + Dispatchers.IO.limitedParallelism(1))

    /** 覚えているサムネ。無ければ端末のファイルから読む。どちらも無ければ null(カードは今の絵のまま)。 */
    fun image(appId: String): ImageBitmap? {
        synchronized(memory) { memory[appId] }?.let { return it }
        val bytes = file(appId)?.takeIf { it.isFile }?.let { runCatching { it.readBytes() }.getOrNull() } ?: return null
        val image = BitmapFactory.decodeByteArray(bytes, 0, bytes.size)?.asImageBitmap() ?: return null
        synchronized(memory) { memory[appId] = image }
        return image
    }

    /** 撮った絵を覚えて、端末に JPEG で書く(書けなくても覚えた分は出す)。 */
    fun put(appId: String, bitmap: Bitmap) {
        if (appId.isEmpty()) return
        synchronized(memory) { memory[appId] = bitmap.asImageBitmap() }
        _revision.value += 1
        val target = file(appId) ?: return
        io.launch {
            try {
                val out = ByteArrayOutputStream()
                bitmap.compress(Bitmap.CompressFormat.JPEG, JPEG_QUALITY, out)
                write(target, out.toByteArray())
            } catch (e: Exception) {
                Log.w(TAG, "thumbnail store failed", e)
            }
        }
    }

    fun remove(appId: String) {
        synchronized(memory) { memory.remove(appId) }
        file(appId)?.let { f ->
            f.delete()
            io.launch { f.delete() }
        }
        _revision.value += 1
    }

    /** サインアウト・アカウント削除の後片付け(#841)。 */
    fun removeAll() {
        synchronized(memory) { memory.clear() }
        directory?.deleteRecursively()
        _revision.value += 1
        directory?.let { dir -> io.launch { dir.deleteRecursively() } } // 消す前に撮った分の書き込みが後から来ても残さない
    }

    /** 置き場のファイル(英数字・-・_ 以外は _ に)。 */
    internal fun file(appId: String): File? = directory?.resolve(fileName(appId))

    companion object {
        private const val TAG = "Metamon"
        const val SCALE = 1f / 3f
        const val JPEG_QUALITY = 75

        /** 変身の後、撮るまで待つ長さ(iOS の 1.2 秒・変わった部品の印が落ち着いてから)。 */
        const val SETTLE_MILLIS = 1_200L

        /** 組み上げ(reveal)が終わってから本物の画面に替わるまでの余裕(iOS の 0.7 秒)。 */
        const val AFTER_REVEAL_MILLIS = 700L

        fun fileName(appId: String): String =
            appId.map { if (it.isLetterOrDigit() || it == '-' || it == '_') it else '_' }.joinToString("") + ".jpg"

        /** 撮ってよいか。音声だけの初期画面は撮らない(カードに出す意味が無い)・シートやお知らせが出ている間も撮らない。 */
        fun shouldCapture(appId: String, isVoiceOnly: Boolean, blocked: Boolean): Boolean = appId.isNotEmpty() && !isVoiceOnly && !blocked

        /** 一時ファイルに書いてから置き換える(書きかけを読ませない)。 */
        internal fun write(target: File, bytes: ByteArray) {
            target.parentFile?.mkdirs()
            val tmp = File(target.parentFile, target.name + ".tmp")
            tmp.writeBytes(bytes)
            if (!tmp.renameTo(target)) {
                tmp.copyTo(target, overwrite = true)
                tmp.delete()
            }
        }
    }
}

/**
 * 今の画面([activity] の窓)を 1/3 の大きさで撮る。上のステータスバー(時計・電池)は切り落とす。
 * ダイアログ・下から出るシートは別の窓なので写らない。撮れなければ null。
 */
suspend fun captureWindow(activity: Activity): Bitmap? {
    val window = activity.window ?: return null
    val decor = window.decorView
    if (decor.width <= 0 || decor.height <= 0) return null
    val top = ViewCompat.getRootWindowInsets(decor)?.getInsets(WindowInsetsCompat.Type.statusBars())?.top ?: 0
    val src = Rect(0, top, decor.width, decor.height)
    val w = (src.width() * AppThumbnailStore.SCALE).toInt().coerceAtLeast(1)
    val h = (src.height() * AppThumbnailStore.SCALE).toInt().coerceAtLeast(1)
    val bitmap = Bitmap.createBitmap(w, h, Bitmap.Config.ARGB_8888)
    return suspendCancellableCoroutine { cont ->
        try {
            PixelCopy.request(window, src, bitmap, { result ->
                cont.resume(if (result == PixelCopy.SUCCESS) bitmap else null)
            }, Handler(Looper.getMainLooper()))
        } catch (e: IllegalArgumentException) {
            // 窓の面がまだ無い(閉じかけ等)
            cont.resume(null)
        }
    }
}

/** Compose の LocalContext から Activity を探す(言語の差し替えなどで包まれていても)。 */
tailrec fun Context.findActivity(): Activity? = when (this) {
    is Activity -> this
    is ContextWrapper -> baseContext.findActivity()
    else -> null
}
