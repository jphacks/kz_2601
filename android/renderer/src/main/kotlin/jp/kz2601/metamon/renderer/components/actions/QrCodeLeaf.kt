package jp.kz2601.metamon.renderer.components.actions

import android.graphics.Bitmap
import androidx.compose.foundation.Image
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.aspectRatio
import androidx.compose.foundation.layout.defaultMinSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Info
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.FilterQuality
import androidx.compose.ui.graphics.ImageBitmap
import androidx.compose.ui.graphics.asImageBitmap
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.clearAndSetSemantics
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.semantics.stateDescription
import androidx.compose.ui.semantics.testTag
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import com.google.zxing.EncodeHintType
import com.google.zxing.WriterException
import com.google.zxing.qrcode.decoder.ErrorCorrectionLevel
import com.google.zxing.qrcode.encoder.Encoder
import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.renderer.R
import jp.kz2601.metamon.renderer.form.FormDraft
import jp.kz2601.metamon.renderer.registry.RenderContext
import jp.kz2601.metamon.renderer.theme.RenderTheme
import jp.kz2601.metamon.renderer.theme.colors

/**
 * QR のモジュール(iOS の `QRCodeImage`: CoreImage の誤り訂正 M・周りに 4 モジュールの余白)。ZXing の core で作る。
 * [size] は余白込みの 1 辺のモジュール数。
 */
class QrMatrix private constructor(val size: Int, private val dark: BooleanArray) {
    operator fun get(x: Int, y: Int): Boolean = dark[y * size + x]

    companion object {
        const val QUIET_ZONE = 4

        /** 空・容量超え(バージョン 40 に入らない)は null。ASCII だけなら ECI を付けない(CoreImage と同じ形)。 */
        fun encode(text: String): QrMatrix? {
            if (text.isEmpty()) return null
            val hints = if (text.all { it.code < 0x80 }) emptyMap() else mapOf(EncodeHintType.CHARACTER_SET to "UTF-8")
            val code = try {
                Encoder.encode(text, ErrorCorrectionLevel.M, hints)
            } catch (_: WriterException) {
                return null
            }
            val m = code.matrix
            val size = m.width + QUIET_ZONE * 2
            val dark = BooleanArray(size * size)
            for (y in 0 until m.height) for (x in 0 until m.width) {
                if (m.get(x, y).toInt() == 1) dark[(y + QUIET_ZONE) * size + x + QUIET_ZONE] = true
            }
            return QrMatrix(size, dark)
        }
    }
}

/** QR の中身(iOS の `QRCodeContent.resolve`): text(`{{id}}` は下書きの保存値のまま)> field の値。 */
object QrContent {
    fun resolve(text: String?, field: String?, form: FormDraft?): String {
        if (!text.isNullOrEmpty()) return FormTemplate.fill(text, form, raw = true)
        if (!field.isNullOrEmpty()) return form?.values?.get(field)?.displayString.orEmpty()
        return ""
    }
}

/** 1 モジュール 1px の白地・黒の画像(描く時は補間なしで拡大する)。アプリの共有シート(#901)も使う。 */
fun QrMatrix.bitmap(): ImageBitmap {
    val pixels = IntArray(size * size) { i -> if (this[i % size, i / size]) 0xFF000000.toInt() else 0xFFFFFFFF.toInt() }
    return Bitmap.createBitmap(pixels, size, size, Bitmap.Config.ARGB_8888).asImageBitmap()
}

/**
 * `qrCode`: text / field から QR を作って見せる。ダークモードでも白地に黒(読み取りやすさ優先)。
 * 空・長すぎる時は案内を出す。読み上げは label(無ければ「QR コード」)+ 中身。
 */
@Composable
fun QrCodeLeaf(c: Component, ctx: RenderContext, modifier: Modifier = Modifier) {
    val content = QrContent.resolve(c.text, c.field, ctx.form)
    val caption = c.label?.takeIf { it.isNotEmpty() }
    val matrix = remember(content) { if (content.isBlank()) null else QrMatrix.encode(content) }
    val image = remember(matrix) { matrix?.bitmap() }
    val shape = RoundedCornerShape(RenderTheme.Radius.inner)
    Column(
        modifier.fillMaxWidth(),
        horizontalAlignment = Alignment.CenterHorizontally,
        verticalArrangement = Arrangement.spacedBy(RenderTheme.Spacing.sm),
    ) {
        c.title?.takeIf { it.isNotEmpty() }?.let {
            Text(
                it,
                Modifier.fillMaxWidth().semantics { heading() },
                style = RenderTheme.Fonts.sectionTitle,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
        when {
            content.isBlank() -> QrPlaceholder(stringResource(R.string.renderer_qr_empty))
            image == null -> QrPlaceholder(stringResource(R.string.renderer_qr_too_long))
            else -> {
                val description = caption ?: stringResource(R.string.renderer_qr_title)
                Image(
                    image,
                    contentDescription = null,
                    modifier = Modifier
                        .widthIn(max = 320.dp)
                        .fillMaxWidth()
                        .aspectRatio(1f)
                        .clip(shape)
                        .border(1.dp, RenderTheme.colors.hairline, shape)
                        .semantics {
                            contentDescription = description
                            stateDescription = content
                            testTag = "qrCode.image"
                        },
                    contentScale = ContentScale.Fit,
                    filterQuality = FilterQuality.None,
                )
                caption?.let {
                    Text(
                        it,
                        Modifier.clearAndSetSemantics { },
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                        textAlign = TextAlign.Center,
                        maxLines = 2,
                    )
                }
            }
        }
    }
}

@Composable
private fun QrPlaceholder(message: String) {
    Column(
        Modifier
            .fillMaxWidth()
            .defaultMinSize(minHeight = 120.dp)
            .background(RenderTheme.colors.surface, RoundedCornerShape(RenderTheme.Radius.inner))
            .semantics(mergeDescendants = true) { testTag = "qrCode.placeholder" },
        horizontalAlignment = Alignment.CenterHorizontally,
        verticalArrangement = Arrangement.spacedBy(RenderTheme.Spacing.sm, Alignment.CenterVertically),
    ) {
        Icon(Icons.Filled.Info, contentDescription = null, Modifier.size(34.dp), tint = MaterialTheme.colorScheme.outline)
        Text(message, style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant, textAlign = TextAlign.Center)
    }
}
