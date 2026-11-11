package jp.kz2601.metamon.renderer.components.actions

import android.graphics.Bitmap
import androidx.compose.foundation.Image
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
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
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.FilterQuality
import androidx.compose.ui.graphics.ImageBitmap
import androidx.compose.ui.graphics.asImageBitmap
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.clearAndSetSemantics
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.LiveRegionMode
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.liveRegion
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
import jp.kz2601.metamon.dsl.expr.Expr
import jp.kz2601.metamon.dsl.expr.ExprEvaluator
import jp.kz2601.metamon.dsl.records.FieldType
import jp.kz2601.metamon.dsl.records.JSONValue
import jp.kz2601.metamon.dsl.records.Record
import jp.kz2601.metamon.dsl.records.RecordQuery
import jp.kz2601.metamon.renderer.DSLRuntime
import jp.kz2601.metamon.renderer.R
import jp.kz2601.metamon.renderer.form.FormDraft
import jp.kz2601.metamon.renderer.registry.RenderContext
import jp.kz2601.metamon.renderer.theme.RenderTheme
import jp.kz2601.metamon.renderer.theme.colors
import kotlinx.coroutines.launch

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

/**
 * QR・読み上げの中身(iOS の `QRCodeContent.resolve`): text(`{{id}}`)> field > value(式)のどれか 1 つ。
 * 保存値はそのまま(日時・数値を整形しない)、computed は式で評価(datetime は RFC3339)。行 = 表示中の下書き。
 * [evaluator] は式を読む時だけ呼ぶ。
 */
object QrContent {
    fun resolve(c: Component, form: FormDraft?, evaluator: () -> ExprEvaluator): String =
        resolve(c.text, c.field, RecordQuery.expr(c.value), form, evaluator)

    fun resolve(text: String?, field: String?, value: Expr?, form: FormDraft?, evaluator: () -> ExprEvaluator): String {
        val row = form?.let { ExprEvaluator.Row(it.entity, Record(it.recordId ?: "draft", it.values.toMap()), memoizable = false) }
        if (!text.isNullOrEmpty()) {
            if (form == null || !text.contains("{{")) return text
            var s: String = text
            for (f in form.entity.fields) {
                val token = "{{${f.id}}}"
                if (s.contains(token)) s = s.replace(token, fieldText(f.id, form, row, evaluator))
            }
            return s
        }
        if (!field.isNullOrEmpty()) return form?.let { fieldText(field, it, row, evaluator) }.orEmpty()
        if (value != null) return evaluator().evaluate(value, row)?.text.orEmpty()
        return ""
    }

    private fun fieldText(id: String, form: FormDraft, row: ExprEvaluator.Row?, evaluator: () -> ExprEvaluator): String {
        val f = form.entity.field(id)
        if (f != null && f.isComputed && row != null) {
            val v = evaluator().fieldValue(id, row)
            return if (f.type == FieldType.DATETIME) v?.json(f)?.displayString.orEmpty() else v?.text.orEmpty()
        }
        return (form.values[id] ?: JSONValue.Null).displayString
    }
}

/** 部品が式を読む評価器(iOS の `rt.evaluator()`)。記録が変わると描き直す。 */
@Composable
internal fun rememberEvaluatorFactory(runtime: DSLRuntime): () -> ExprEvaluator {
    val records = runtime.recordsFlow?.collectAsState()?.value.orEmpty()
    return { ExprEvaluator(runtime.entityModels, { records[it].orEmpty() }) }
}

/** 1 モジュール 1px の白地・黒の画像(描く時は補間なしで拡大する)。アプリの共有シート(#901)も使う。 */
fun QrMatrix.bitmap(): ImageBitmap {
    val pixels = IntArray(size * size) { i -> if (this[i % size, i / size]) 0xFF000000.toInt() else 0xFFFFFFFF.toInt() }
    return Bitmap.createBitmap(pixels, size, size, Bitmap.Config.ARGB_8888).asImageBitmap()
}

/**
 * `qrCode`: text / field / value から QR を作って見せる。ダークモードでも白地に黒(読み取りやすさ優先)。
 * 空・長すぎる時は案内を出す。読み上げは label(無ければ「QR コード」)+ 中身。
 * 下に「写真に保存」(MediaStore・追加だけ・権限なし)と「共有」(PNG・#1086・iOS と同じ)。
 */
@Composable
fun QrCodeLeaf(c: Component, ctx: RenderContext, modifier: Modifier = Modifier) {
    val content = QrContent.resolve(c, ctx.form, rememberEvaluatorFactory(ctx.runtime))
    val caption = c.label?.takeIf { it.isNotEmpty() }
    val matrix = remember(content) { if (content.isBlank()) null else QrMatrix.encode(content) }
    val image = remember(matrix) { matrix?.bitmap() }
    val context = LocalContext.current
    val scope = rememberCoroutineScope()
    var saveState by remember(content) { mutableStateOf(QrSaveState.IDLE) }
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
                val m = matrix!!
                Row(Modifier.widthIn(max = 320.dp).fillMaxWidth(), horizontalArrangement = Arrangement.spacedBy(RenderTheme.Spacing.sm)) {
                    ActionButton(
                        stringResource(saveState.labelRes),
                        ActionButtonKind.SECONDARY,
                        onClick = {
                            saveState = QrSaveState.SAVING
                            scope.launch {
                                val ok = runCatching {
                                    saveImageToGallery(context, ShareImages.qrGalleryEntry(System.currentTimeMillis()), m.exportPng())
                                }.getOrDefault(false)
                                saveState = if (ok) QrSaveState.SAVED else QrSaveState.FAILED
                            }
                        },
                        modifier = Modifier.weight(1f).semantics {
                            liveRegion = LiveRegionMode.Polite // 保存できた・できなかったを読み上げる
                            testTag = "qrCode.save"
                        },
                        icon = if (saveState == QrSaveState.SAVED) "checkmark" else null,
                        running = saveState == QrSaveState.SAVING,
                    )
                    val shareLabel = stringResource(R.string.renderer_qr_share)
                    ActionButton(
                        shareLabel,
                        ActionButtonKind.SECONDARY,
                        onClick = { shareImage(context, m.exportPng(), ShareImages.QR_FILE, ShareImages.PNG, null, description) },
                        modifier = Modifier.weight(1f).semantics { testTag = "qrCode.share" },
                        icon = "square.and.arrow.up",
                    )
                }
            }
        }
    }
}

/** 「写真に保存」の状態と、ボタンの文言(iOS の `QrCodeLeaf.saveLabel`)。中身が変わったら最初に戻す。 */
internal enum class QrSaveState(val labelRes: Int) {
    IDLE(R.string.renderer_qr_save),
    SAVING(R.string.renderer_qr_save),
    SAVED(R.string.renderer_qr_saved),
    FAILED(R.string.renderer_qr_save_failed),
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
