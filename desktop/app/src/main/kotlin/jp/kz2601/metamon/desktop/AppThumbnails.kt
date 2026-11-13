package jp.kz2601.metamon.desktop

import androidx.compose.foundation.Image
import androidx.compose.foundation.border
import androidx.compose.foundation.layout.size
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.snapshotFlow
import androidx.compose.runtime.staticCompositionLocalOf
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.draw.drawWithContent
import androidx.compose.ui.graphics.ImageBitmap
import androidx.compose.ui.graphics.asSkiaBitmap
import androidx.compose.ui.graphics.layer.drawLayer
import androidx.compose.ui.graphics.rememberGraphicsLayer
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.platform.LocalWindowInfo
import androidx.compose.ui.semantics.clearAndSetSemantics
import androidx.compose.ui.unit.dp
import androidx.compose.material3.MaterialTheme
import jp.kz2601.metamon.desktop.store.ThumbnailStore
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.FlowPreview
import kotlinx.coroutines.flow.debounce
import kotlinx.coroutines.flow.drop
import kotlinx.coroutines.flow.filter
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import org.jetbrains.skia.EncodedImageFormat
import org.jetbrains.skia.Image as SkiaImage

/** アプリの写しの置き場(#3204)。Main.kt のウィンドウが渡す。null(テスト・開発の 1 枚起動)なら撮らない・出さない。 */
val LocalAppThumbnails = staticCompositionLocalOf<ThumbnailStore?> { null }

/** 画面の写しを縮めて PNG にする(iOS の 1/3 の大きさの代わりに長い辺で止める)。 */
object ThumbnailImage {
    /** 長い辺の画素。行の 40×28dp を 2 倍の画素でも滲まない大きさより少し大きめ */
    const val MAX_PX = 360

    /** 縮めた PNG。描けていない(0 画素)・書けない時は null。 */
    fun png(image: SkiaImage, maxPx: Int = MAX_PX): ByteArray? {
        if (image.width <= 0 || image.height <= 0) return null
        val small = runCatching { ImageLoader.downscale(image, maxPx) }.getOrNull() ?: return null
        return runCatching { small.encodeToData(EncodedImageFormat.PNG)?.bytes }.getOrNull()
    }

    fun png(bitmap: ImageBitmap, maxPx: Int = MAX_PX): ByteArray? =
        runCatching { SkiaImage.makeFromBitmap(bitmap.asSkiaBitmap()) }.getOrNull()?.let { png(it, maxPx) }

    /** 行に出す絵。読めない時は null。 */
    fun decode(png: ByteArray): ImageBitmap? = ImageLoader.decode(png, maxPx = MAX_PX, withPng = false)?.bitmap
}

/**
 * ウィンドウの中身を写せるようにする Modifier(Compose の GraphicsLayer に描いてから画面へ)。
 * 撮るのは: 別のアプリ(ウィンドウ)に切り替えた時(フォーカスが外れた)・画面が変わって落ち着いた時(閉じる前の最後の画面を残す)。
 * [appId] が null・置き場が無い時は何もしない。
 */
@OptIn(FlowPreview::class)
@Composable
fun Modifier.captureAppThumbnail(appId: String?, changes: () -> Any?): Modifier {
    val store = LocalAppThumbnails.current
    if (appId == null || store == null) return this
    val layer = rememberGraphicsLayer()
    val scope = rememberCoroutineScope()
    val window = LocalWindowInfo.current
    val capture: () -> Unit = {
        scope.launch {
            val bitmap = runCatching { layer.toImageBitmap() }.getOrNull() ?: return@launch
            val png = withContext(Dispatchers.Default) { ThumbnailImage.png(bitmap) } ?: return@launch
            withContext(Dispatchers.IO) { store.put(appId, png) }
        }
    }
    LaunchedEffect(appId) {
        snapshotFlow { window.isWindowFocused }.drop(1).filter { !it }.collect { capture() }
    }
    LaunchedEffect(appId) {
        snapshotFlow(changes).debounce(SETTLE_MILLIS).collect { if (window.isWindowFocused) capture() }
    }
    return drawWithContent {
        layer.record { this@drawWithContent.drawContent() }
        drawLayer(layer)
    }
}

private const val SETTLE_MILLIS = 1_500L

/** [appId] の写し(置いたら読み直す)。無い・置き場が無い時は null。 */
@Composable
fun rememberAppThumbnail(appId: String, store: ThumbnailStore?): ImageBitmap? {
    if (store == null) return null
    val revision by store.revision.collectAsState()
    return remember(appId, revision) { store.get(appId)?.let(ThumbnailImage::decode) }
}

/** 行の小さな写し。読み上げでは飾り(説明を付けない)。 */
@Composable
fun AppRowThumbnail(image: ImageBitmap, modifier: Modifier = Modifier) {
    val shape = MaterialTheme.shapes.extraSmall
    Image(
        image,
        contentDescription = null,
        contentScale = ContentScale.Crop,
        // 上(タイトル)が見えるよう上揃えで切り出す(Mac と同じ)
        alignment = Alignment.TopCenter,
        modifier = modifier.size(ROW_WIDTH.dp, ROW_HEIGHT.dp).clip(shape)
            .border(1.dp, MaterialTheme.colorScheme.outlineVariant, shape)
            .clearAndSetSemantics {},
    )
}

private const val ROW_WIDTH = 40
private const val ROW_HEIGHT = 28
