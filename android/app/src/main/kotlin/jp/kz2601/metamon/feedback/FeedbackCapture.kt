package jp.kz2601.metamon.feedback

import android.app.Activity
import android.content.Context
import android.content.ContextWrapper
import android.graphics.Bitmap
import android.graphics.Canvas
import android.graphics.Color
import android.graphics.Paint
import android.graphics.Rect
import android.os.Build
import android.os.Handler
import android.os.Looper
import android.view.PixelCopy
import android.view.View
import android.view.WindowManager
import android.view.inspector.WindowInspector
import jp.kz2601.metamon.AppContainer
import jp.kz2601.metamon.BuildConfig
import jp.kz2601.metamon.language.AppLocales
import jp.kz2601.metamon.services.feedback.FeedbackConsent
import jp.kz2601.metamon.services.feedback.FeedbackContext
import jp.kz2601.metamon.services.feedback.FeedbackSubmission
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.suspendCancellableCoroutine
import java.io.ByteArrayOutputStream
import kotlin.coroutines.resume
import kotlin.math.max
import kotlin.math.roundToInt

/** 開いているフィードバックの画面。スクショはメモリにだけ持ち、送らずに閉じれば捨てる。 */
class FeedbackOpen(val context: FeedbackContext, val screenshot: Bitmap?)

/**
 * フィードバックの画面の出し入れ(iOS の `FeedbackPresenter`)。設定の入口と端末を振った時の両方がここを通る。
 * 二重には開かない。
 */
class FeedbackComposerLauncher {
    private val _open = MutableStateFlow<FeedbackOpen?>(null)
    val open: StateFlow<FeedbackOpen?> = _open.asStateFlow()

    /** 撮っている途中(振った直後にもう 1 回振っても 2 回撮らない) */
    private var capturing = false

    val isOpen: Boolean get() = _open.value != null || capturing

    /** 今の画面を撮り([withOverlays] は振った時だけ true)、今のアプリと端末の情報を集めて開く。どれも送るかは本人が選ぶ(既定はすべて送らない)。 */
    suspend fun capture(activity: Activity, container: AppContainer, withOverlays: Boolean = false) {
        if (isOpen) return
        capturing = true
        try {
            val shot = FeedbackScreenshot.capture(activity, withOverlays)
            val doc = container.runtime.state.value.document
            val envelope = runCatching { container.documentCache.loadJson(doc.appId) }.getOrNull()
            val context = FeedbackContext(
                app = FeedbackContext.appSummary(doc, envelope),
                device = currentDevice(activity),
                hasScreenshot = shot != null,
            )
            _open.value = FeedbackOpen(context, shot)
        } finally {
            capturing = false
        }
    }

    fun close() {
        _open.value = null
    }

    companion object {
        /** アプリの版・OS・機種・言語。uid などの本人に結びつく値は入れない。 */
        fun currentDevice(context: Context): FeedbackSubmission.Device = FeedbackSubmission.Device(
            appVersion = BuildConfig.VERSION_NAME,
            build = BuildConfig.VERSION_CODE.toString(),
            os = "Android ${Build.VERSION.RELEASE}",
            model = listOf(Build.MANUFACTURER, Build.MODEL).filter { it.isNotBlank() }.joinToString(" "),
            locale = AppLocales.languageCode(context),
        ).clipped()
    }
}

/** スクショの撮り方と縮め方(iOS の `FeedbackScreenshot`)。撮るのはこのアプリの画面だけ。 */
object FeedbackScreenshot {
    /** 長い辺の上限(px)。文字が読める程度に縮めて送る量を減らす */
    const val MAX_PIXELS = 1280

    /**
     * Activity の窓を PixelCopy で撮る(他のアプリ・システムの画面は写らない)。
     * [withOverlays] が true なら、上に出ているこのアプリのシート・ダイアログ(別の窓)も重ねる(振った時・iOS の `captureWindow()`)。
     * false なら Activity の窓だけ(設定から開いた時・シートの下のアプリの画面)。
     */
    suspend fun capture(activity: Activity, withOverlays: Boolean = false): Bitmap? {
        val window = activity.window ?: return null
        val view = window.decorView
        if (view.width <= 0 || view.height <= 0) return null
        val full = Bitmap.createBitmap(view.width, view.height, Bitmap.Config.ARGB_8888)
        val ok = suspendCancellableCoroutine { cont ->
            runCatching {
                PixelCopy.request(window, full, { result -> cont.resume(result == PixelCopy.SUCCESS) }, Handler(Looper.getMainLooper()))
            }.onFailure { cont.resume(false) }
        }
        if (!ok) {
            full.recycle()
            return null
        }
        if (withOverlays) runCatching { drawOverlays(activity, view, full) }
        return shrink(full, MAX_PIXELS)
    }

    /** Activity の窓より後に足された、このアプリの窓(シート・ダイアログ・ポップアップ)を順に重ねる。 */
    private suspend fun drawOverlays(activity: Activity, base: View, into: Bitmap) {
        val roots = WindowInspector.getGlobalWindowViews()
        val index = roots.indexOf(base)
        if (index < 0) return
        val overlays = roots.drop(index + 1).filter { it.isAttachedToWindow && it.isShown && it.context.findActivity() === activity }
        if (overlays.isEmpty()) return
        val baseLoc = IntArray(2).also(base::getLocationOnScreen)
        val layers = overlays.map { root ->
            val loc = IntArray(2).also(root::getLocationOnScreen)
            val lp = root.layoutParams as? WindowManager.LayoutParams
            WindowLayer(
                left = loc[0],
                top = loc[1],
                width = root.width,
                height = root.height,
                secure = lp != null && lp.flags and WindowManager.LayoutParams.FLAG_SECURE != 0,
                dimBehind = if (lp != null && lp.flags and WindowManager.LayoutParams.FLAG_DIM_BEHIND != 0) lp.dimAmount else 0f,
            )
        }
        val canvas = Canvas(into)
        for (step in OverlayPlan.plan(baseLoc[0], baseLoc[1], into.width, into.height, layers)) {
            when (step) {
                is OverlayPlan.Step.Dim -> canvas.drawColor(Color.argb((step.alpha * 255).roundToInt(), 0, 0, 0))
                is OverlayPlan.Step.Draw -> {
                    val root = overlays[step.index]
                    val shot = copyWindow(root) ?: continue
                    val dst = Rect(step.dx, step.dy, step.dx + root.width, step.dy + root.height)
                    canvas.drawBitmap(shot, null, dst, Paint(Paint.FILTER_BITMAP_FLAG))
                    shot.recycle()
                }
            }
        }
    }

    /** 1 つの窓を撮る。34 以上は PixelCopy、それより前は View を描く(描けなければ写さない)。 */
    private suspend fun copyWindow(root: View): Bitmap? {
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.UPSIDE_DOWN_CAKE) {
            return suspendCancellableCoroutine { cont ->
                runCatching {
                    val request = PixelCopy.Request.Builder.ofWindow(root).build()
                    PixelCopy.request(request, root.context.mainExecutor) { result ->
                        cont.resume(if (result.status == PixelCopy.SUCCESS) result.bitmap else null)
                    }
                }.onFailure { cont.resume(null) }
            }
        }
        return runCatching {
            val bmp = Bitmap.createBitmap(root.width, root.height, Bitmap.Config.ARGB_8888)
            runCatching { root.draw(Canvas(bmp)) }.onFailure { bmp.recycle() }.getOrThrow()
            bmp
        }.getOrNull()
    }

    private fun shrink(bitmap: Bitmap, maxPixels: Int): Bitmap {
        val ratio = maxPixels.toFloat() / max(bitmap.width, bitmap.height)
        if (ratio >= 1f) return bitmap
        val out = Bitmap.createScaledBitmap(bitmap, (bitmap.width * ratio).roundToInt(), (bitmap.height * ratio).roundToInt(), true)
        bitmap.recycle()
        return out
    }

    /** 上限のバイト数以下の JPEG にする(画質 → 大きさの順に落とす)。無理なら null。 */
    fun jpeg(bitmap: Bitmap, maxBytes: Int = FeedbackConsent.MAX_SCREENSHOT_BYTES): ByteArray? = fit(maxBytes) { scale, quality ->
        val img = if (scale >= 1f) {
            bitmap
        } else {
            Bitmap.createScaledBitmap(bitmap, (bitmap.width * scale).roundToInt().coerceAtLeast(1), (bitmap.height * scale).roundToInt().coerceAtLeast(1), true)
        }
        ByteArrayOutputStream().use { out ->
            img.compress(Bitmap.CompressFormat.JPEG, quality, out)
            if (img !== bitmap) img.recycle()
            out.toByteArray()
        }
    }

    /** 画質 70 → 20 を試し、だめなら 0.7 倍に縮めてまた試す(4 段まで・iOS と同じ順)。 */
    fun fit(maxBytes: Int, encode: (scale: Float, quality: Int) -> ByteArray?): ByteArray? {
        var scale = 1f
        repeat(4) {
            for (q in QUALITIES) {
                val data = encode(scale, q)
                if (data != null && data.size <= maxBytes) return data
            }
            scale *= 0.7f
        }
        return null
    }

    private val QUALITIES = listOf(70, 50, 35, 20)
}

internal tailrec fun Context.findActivity(): Activity? = when (this) {
    is Activity -> this
    is ContextWrapper -> baseContext.findActivity()
    else -> null
}
