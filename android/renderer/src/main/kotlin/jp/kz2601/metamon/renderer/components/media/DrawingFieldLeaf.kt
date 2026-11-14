package jp.kz2601.metamon.renderer.components.media

import android.graphics.Bitmap
import android.graphics.BitmapFactory
import android.graphics.Canvas
import android.graphics.Paint
import android.graphics.Path
import android.graphics.RectF
import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.Image
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.gestures.detectDragGestures
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.safeDrawingPadding
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateListOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.ImageBitmap
import androidx.compose.ui.graphics.StrokeCap
import androidx.compose.ui.graphics.StrokeJoin
import androidx.compose.ui.graphics.asAndroidBitmap
import androidx.compose.ui.graphics.asImageBitmap
import androidx.compose.ui.graphics.drawscope.Stroke
import androidx.compose.ui.input.pointer.pointerInput
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.layout.onSizeChanged
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.semantics.testTag
import androidx.compose.ui.unit.IntSize
import androidx.compose.ui.unit.dp
import androidx.compose.ui.window.Dialog
import androidx.compose.ui.window.DialogProperties
import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.records.JSONValue
import jp.kz2601.metamon.renderer.R
import jp.kz2601.metamon.renderer.components.PhotoValue
import jp.kz2601.metamon.renderer.components.PhotoView
import jp.kz2601.metamon.renderer.components.inputs.FormInput
import jp.kz2601.metamon.renderer.form.FormDraft
import jp.kz2601.metamon.renderer.registry.RenderContext
import jp.kz2601.metamon.renderer.theme.RenderTheme
import jp.kz2601.metamon.renderer.theme.colors
import androidx.compose.ui.graphics.Path as ComposePath

/**
 * `drawingField`(iOS の `DrawingFieldLeaf`): 指で手書きして photo field に入れる。権限は要らない。
 * フォームには描いた絵(か「タップして描く」の枠)を出し、タップで全画面のキャンバスを開く(フォームのスクロールとぶつからない)。
 * 完了で白地の JPEG(長い辺 800px 以下・品質 70)の data URL にする。写真と同じ形なので一覧・同期・iOS でもそのまま見える。
 */
@Composable
fun DrawingFieldLeaf(c: Component, ctx: RenderContext, modifier: Modifier = Modifier) = FormInput(c, ctx, modifier) { form, field ->
    DrawingFieldPanel(c, form, field)
}

@Composable
private fun DrawingFieldPanel(c: Component, form: FormDraft, field: String) {
    val title = c.label?.takeIf { it.isNotEmpty() } ?: stringResource(R.string.media_drawing_title)
    val value = (form.values[field] as? JSONValue.Str)?.value
    val picture = remember(value) { decode(value) }
    val hasDrawing = PhotoValue.hasPhoto(value)
    var open by remember { mutableStateOf(false) }
    val shape = RoundedCornerShape(RenderTheme.Radius.inner)
    val a11y = if (hasDrawing) stringResource(R.string.media_drawing_a11y_has, title) else title

    Column(Modifier.fillMaxWidth(), verticalArrangement = Arrangement.spacedBy(RenderTheme.Spacing.sm)) {
        Text(title, style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.onSurfaceVariant, modifier = Modifier.padding(start = 2.dp))
        val preview = Modifier
            .fillMaxWidth()
            .clip(shape)
            .clickable { open = true }
            .semantics { contentDescription = a11y; testTag = "drawingField.$field" }
        when {
            picture != null -> Image(
                picture,
                null,
                preview.heightIn(min = 100.dp, max = 220.dp).background(Color.White).border(1.dp, RenderTheme.colors.hairline, shape),
                contentScale = ContentScale.Fit,
            )
            hasDrawing -> PhotoView(value, 160.dp, preview)
            else -> Box(
                preview.heightIn(min = 120.dp).background(RenderTheme.colors.brandSoft).border(1.5.dp, RenderTheme.colors.brandSoftStroke, shape),
                contentAlignment = Alignment.Center,
            ) {
                Text(stringResource(R.string.media_drawing_tap), color = RenderTheme.colors.brandText, style = MaterialTheme.typography.bodyLarge)
            }
        }
        if (hasDrawing) {
            Row(horizontalArrangement = Arrangement.spacedBy(RenderTheme.Spacing.sm)) {
                MediaButton(stringResource(R.string.media_drawing_more), "pencil", Modifier.weight(1f)) { open = true }
                MediaButton(
                    stringResource(R.string.media_delete),
                    "trash",
                    Modifier.weight(1f),
                    a11yLabel = stringResource(R.string.media_delete_named, title),
                    destructive = true,
                ) { form.values[field] = JSONValue.Null }
            }
        }
    }

    if (open) {
        DrawingSheet(title, background = picture, onCancel = { open = false }) { state, size ->
            form.values[field] = DrawingRender.fieldValue(state, picture, size)
            open = false
        }
    }
}

private enum class Tool { PEN, ERASER }

/** 全画面のキャンバス。上に キャンセル / 完了、下に ペン / 消しゴム・元に戻す・やり直す・全部消す。 */
@Composable
private fun DrawingSheet(title: String, background: ImageBitmap?, onCancel: () -> Unit, onDone: (DrawingState, IntSize) -> Unit) {
    var state by remember { mutableStateOf(DrawingState(hasBackground = background != null)) }
    var tool by remember { mutableStateOf(Tool.PEN) }
    var size by remember { mutableStateOf(IntSize.Zero) }
    var confirmClear by remember { mutableStateOf(false) }
    var confirmDiscard by remember { mutableStateOf(false) }
    val current = remember { mutableStateListOf<InkPoint>() }
    val density = LocalDensity.current
    val penWidth = with(density) { 3.dp.toPx() }
    val eraserRadius = with(density) { 12.dp.toPx() }
    val cancel = { if (state.isChanged) confirmDiscard = true else onCancel() }

    Dialog(onDismissRequest = cancel, properties = DialogProperties(usePlatformDefaultWidth = false, dismissOnClickOutside = false)) {
        Surface(Modifier.fillMaxSize()) {
            Column(Modifier.fillMaxSize().safeDrawingPadding()) {
                Row(Modifier.fillMaxWidth().padding(horizontal = RenderTheme.Spacing.sm), verticalAlignment = Alignment.CenterVertically) {
                    TextButton(onClick = cancel) { Text(stringResource(R.string.media_cancel)) }
                    Text(title, Modifier.weight(1f), style = MaterialTheme.typography.titleMedium, maxLines = 1)
                    TextButton(onClick = { onDone(state, size) }, enabled = size.width > 0) { Text(stringResource(R.string.media_drawing_done)) }
                }
                val canvasLabel = stringResource(R.string.media_drawing_canvas)
                Box(
                    Modifier
                        .weight(1f)
                        .fillMaxWidth()
                        .background(Color.White)
                        .onSizeChanged { size = it }
                        .semantics { contentDescription = canvasLabel }
                        .pointerInput(tool) {
                            detectDragGestures(
                                onDragStart = { o ->
                                    when (tool) {
                                        Tool.PEN -> { current.clear(); current += InkPoint(o.x, o.y) }
                                        Tool.ERASER -> state = state.erasing(InkPoint(o.x, o.y), eraserRadius)
                                    }
                                },
                                onDrag = { change, _ ->
                                    val p = change.position
                                    when (tool) {
                                        Tool.PEN -> current += InkPoint(p.x, p.y)
                                        Tool.ERASER -> state = state.erasing(InkPoint(p.x, p.y), eraserRadius)
                                    }
                                },
                                onDragEnd = {
                                    if (tool == Tool.PEN) state = state.adding(InkStroke(current.toList(), penWidth))
                                    current.clear()
                                },
                                onDragCancel = { current.clear() },
                            )
                        },
                ) {
                    if (background != null && state.hasBackground) {
                        Image(background, null, Modifier.fillMaxSize(), contentScale = ContentScale.Fit)
                    }
                    androidx.compose.foundation.Canvas(Modifier.fillMaxSize()) {
                        state.strokes.forEach { drawInk(it.points, it.width) }
                        if (current.isNotEmpty()) drawInk(current, penWidth)
                    }
                }
                Row(
                    Modifier.fillMaxWidth().padding(RenderTheme.Spacing.sm),
                    horizontalArrangement = Arrangement.spacedBy(RenderTheme.Spacing.xs),
                    verticalAlignment = Alignment.CenterVertically,
                ) {
                    ToolButton(stringResource(R.string.media_drawing_pen), tool == Tool.PEN) { tool = Tool.PEN }
                    ToolButton(stringResource(R.string.media_drawing_eraser), tool == Tool.ERASER) { tool = Tool.ERASER }
                    Box(Modifier.weight(1f))
                    TextButton(onClick = { state = state.undone() }, enabled = state.canUndo) { Text(stringResource(R.string.media_drawing_undo)) }
                    TextButton(onClick = { state = state.redone() }, enabled = state.canRedo) { Text(stringResource(R.string.media_drawing_redo)) }
                    TextButton(onClick = { confirmClear = true }, enabled = !state.isEmpty) {
                        Text(stringResource(R.string.media_drawing_clear), color = MaterialTheme.colorScheme.error)
                    }
                }
            }
        }
        if (confirmClear) {
            AlertDialog(
                onDismissRequest = { confirmClear = false },
                title = { Text(stringResource(R.string.media_drawing_clear_title)) },
                text = { Text(stringResource(R.string.media_drawing_clear_body)) },
                confirmButton = {
                    TextButton(onClick = { confirmClear = false; state = state.cleared() }) {
                        Text(stringResource(R.string.media_drawing_clear), color = MaterialTheme.colorScheme.error)
                    }
                },
                dismissButton = { TextButton(onClick = { confirmClear = false }) { Text(stringResource(R.string.media_cancel)) } },
            )
        }
        if (confirmDiscard) {
            AlertDialog(
                onDismissRequest = { confirmDiscard = false },
                title = { Text(stringResource(R.string.media_drawing_discard_title)) },
                confirmButton = {
                    TextButton(onClick = { confirmDiscard = false; onCancel() }) {
                        Text(stringResource(R.string.media_drawing_discard), color = MaterialTheme.colorScheme.error)
                    }
                },
                dismissButton = { TextButton(onClick = { confirmDiscard = false }) { Text(stringResource(R.string.media_cancel)) } },
            )
        }
    }
}

@Composable
private fun ToolButton(label: String, selected: Boolean, onClick: () -> Unit) {
    val colors = MaterialTheme.colorScheme
    Surface(
        onClick = onClick,
        selected = selected,
        shape = RoundedCornerShape(RenderTheme.Radius.control),
        color = if (selected) colors.primaryContainer else Color.Transparent,
        border = if (selected) null else BorderStroke(1.dp, RenderTheme.colors.hairline),
    ) {
        Text(label, Modifier.padding(horizontal = 12.dp, vertical = 8.dp), color = if (selected) colors.onPrimaryContainer else colors.onSurface)
    }
}

private fun androidx.compose.ui.graphics.drawscope.DrawScope.drawInk(points: List<InkPoint>, width: Float) {
    if (points.size == 1) {
        drawCircle(Color.Black, width / 2, Offset(points[0].x, points[0].y))
        return
    }
    val path = ComposePath().apply {
        moveTo(points[0].x, points[0].y)
        for (i in 1 until points.size) lineTo(points[i].x, points[i].y)
    }
    drawPath(path, Color.Black, style = Stroke(width, cap = StrokeCap.Round, join = StrokeJoin.Round))
}

private fun decode(value: String?): ImageBitmap? =
    PhotoValue.dataBytes(value)?.let { BitmapFactory.decodeByteArray(it, 0, it.size) }?.asImageBitmap()

/** 描いた絵 → field の値(Android の Bitmap で描く部分)。何も無ければ Null(field を空にする)。 */
internal object DrawingRender {
    fun fieldValue(state: DrawingState, background: ImageBitmap?, canvas: IntSize): JSONValue {
        if (state.isEmpty || canvas.width < 1 || canvas.height < 1) return JSONValue.Null
        val w = canvas.width.toFloat()
        val h = canvas.height.toFloat()
        val scale = DrawingCodec.renderScale(w, h)
        val bitmap = Bitmap.createBitmap((w * scale).toInt(), (h * scale).toInt(), Bitmap.Config.ARGB_8888)
        Canvas(bitmap).apply {
            scale(scale, scale)
            drawColor(android.graphics.Color.WHITE)
            if (background != null && state.hasBackground) {
                val bg = background.asAndroidBitmap()
                val (l, t, bw, bh) = DrawingCodec.aspectFit(bg.width.toFloat(), bg.height.toFloat(), w, h).toList()
                drawBitmap(bg, null, RectF(l, t, l + bw, t + bh), Paint(Paint.FILTER_BITMAP_FLAG))
            }
            val paint = Paint(Paint.ANTI_ALIAS_FLAG).apply {
                color = android.graphics.Color.BLACK
                style = Paint.Style.STROKE
                strokeCap = Paint.Cap.ROUND
                strokeJoin = Paint.Join.ROUND
            }
            for (s in state.strokes) {
                paint.strokeWidth = s.width
                val path = Path().apply {
                    moveTo(s.points[0].x, s.points[0].y)
                    for (i in 1 until s.points.size) lineTo(s.points[i].x, s.points[i].y)
                    if (s.points.size == 1) lineTo(s.points[0].x + 0.01f, s.points[0].y)
                }
                drawPath(path, paint)
            }
        }
        return PhotoCodec.dataUrl(bitmap, maxSide = DrawingCodec.MAX_SIDE, quality = DrawingCodec.QUALITY)
            ?.let { JSONValue.Str(it) } ?: JSONValue.Null
    }
}
