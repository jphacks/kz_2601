package jp.kz2601.metamon.desktop

import androidx.compose.foundation.Canvas
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.gestures.detectDragGestures
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.FilterChip
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Slider
import androidx.compose.material3.Switch
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.mutableStateListOf
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.StrokeCap
import androidx.compose.ui.input.pointer.pointerInput
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.text.input.VisualTransformation
import androidx.compose.ui.unit.dp
import com.google.zxing.BinaryBitmap
import com.google.zxing.MultiFormatReader
import com.google.zxing.RGBLuminanceSource
import com.google.zxing.common.HybridBinarizer
import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.ComponentType
import jp.kz2601.metamon.dsl.records.FieldType
import jp.kz2601.metamon.dsl.records.JSONValue
import jp.kz2601.metamon.renderer.form.FormDraft
import java.awt.FileDialog
import java.awt.Frame
import java.awt.GraphicsEnvironment
import java.awt.Toolkit
import java.awt.datatransfer.DataFlavor
import java.awt.image.BufferedImage
import java.io.File
import javax.imageio.ImageIO
import java.awt.BasicStroke as AwtStroke
import java.awt.Color as AwtColor
import java.awt.RenderingHints as AwtHints

/**
 * 入力の部品 20 種(iOS の Inputs・Android の components/inputs)。値は [FormDraft] の values に field をキーに書く。
 * カメラ・マイク・位置など、このパソコンで出来ない物は「このパソコンでは使えません」の枠 + 出来る範囲の代わり(ファイルを選ぶ等)。
 */
object InputLeaves {
    val types: Set<ComponentType> = setOf(
        ComponentType.TEXT_FIELD, ComponentType.NUMBER_FIELD, ComponentType.PICKER, ComponentType.TOGGLE, ComponentType.DATE_PICKER,
        ComponentType.SLIDER, ComponentType.STEPPER, ComponentType.TEXT_EDITOR, ComponentType.SECURE_FIELD, ComponentType.TIME_PICKER,
        ComponentType.RATING, ComponentType.COLOR_PICKER, ComponentType.PHOTO_FIELD, ComponentType.DRAWING_FIELD, ComponentType.VIDEO_FIELD,
        ComponentType.AUDIO_RECORDER, ComponentType.TRANSCRIBE_FIELD, ComponentType.LOCATION_PICKER, ComponentType.LOCATION_BUTTON,
        ComponentType.QR_SCANNER,
    )

    /** ラベルを `c.label ?: field.name` で補う入力。他は `c.label` だけ(iOS の InputRules と同じ)。 */
    private val labelFallsBackToFieldName =
        setOf(ComponentType.TOGGLE, ComponentType.DATE_PICKER, ComponentType.COLOR_PICKER, ComponentType.STEPPER, ComponentType.TIME_PICKER)

    fun label(c: Component, form: FormDraft): String? {
        val fid = c.field ?: return c.label
        if (c.type !in labelFallsBackToFieldName) return c.label
        return c.label ?: form.entity.field(fid)?.name ?: (if (c.type == ComponentType.TIME_PICKER) "" else fid)
    }

    /** 画面に出す見出し(ラベルが無い入力は field の名前で補う・見出しだけ)。 */
    fun caption(c: Component, form: FormDraft): String =
        label(c, form) ?: c.field?.let { form.entity.field(it)?.name } ?: c.field.orEmpty()

    /** 選べる項目(enum の options・今の値が外れていれば足す)。 */
    fun options(form: FormDraft, field: String): List<String> {
        val base = form.entity.field(field)?.options.orEmpty()
        val now = form.values[field]?.stringValue?.takeIf { it.isNotEmpty() }
        return if (now != null && now !in base) base + now else base
    }

    /** 文字の入力欄に出す文字(数は余計な .0 を付けない)。 */
    fun text(v: JSONValue?): String = when (v) {
        null, JSONValue.Null -> ""
        is JSONValue.Str -> v.value
        is JSONValue.Num -> JSONValue.numberText(v.value)
        else -> v.displayString
    }

    /** 画像から QR / バーコードを読む(zxing・純 Java)。読めなければ null。 */
    fun decodeCode(image: BufferedImage): String? = runCatching {
        val px = IntArray(image.width * image.height)
        image.getRGB(0, 0, image.width, image.height, px, 0, image.width)
        MultiFormatReader().decode(BinaryBitmap(HybridBinarizer(RGBLuminanceSource(image.width, image.height, px)))).text
    }.getOrNull()

    const val UNAVAILABLE = "このパソコンでは使えません"
}

/** 入力の部品 1 つ。隠す・フォームの外は呼ぶ側([DesktopSession.tapState])で決めてある。 */
@Composable
fun InputLeaf(c: Component, form: FormDraft, onMessage: (String) -> Unit) {
    val field = c.field ?: return
    val caption = InputLeaves.caption(c, form)
    val value = form.values[field]
    val readOnly = form.isReadOnly
    val flagged = form.isFlagged(field)
    fun set(v: JSONValue) {
        if (!readOnly) form.values[field] = v
    }
    Column(Modifier.fillMaxWidth(), verticalArrangement = Arrangement.spacedBy(4.dp)) {
        when (c.type) {
            ComponentType.TEXT_FIELD, ComponentType.NUMBER_FIELD, ComponentType.TEXT_EDITOR, ComponentType.SECURE_FIELD, ComponentType.TRANSCRIBE_FIELD -> {
                val numeric = c.type == ComponentType.NUMBER_FIELD || form.entity.field(field)?.type?.isNumeric == true
                OutlinedTextField(
                    value = InputLeaves.text(value),
                    onValueChange = { set(JSONValue.Str(it)) },
                    label = { Text(caption + (c.unit?.let { " ($it)" } ?: "")) },
                    placeholder = c.placeholder?.let { p -> { Text(p) } },
                    singleLine = c.type != ComponentType.TEXT_EDITOR && c.type != ComponentType.TRANSCRIBE_FIELD,
                    minLines = if (c.type == ComponentType.TEXT_EDITOR || c.type == ComponentType.TRANSCRIBE_FIELD) 3 else 1,
                    readOnly = readOnly,
                    isError = flagged || (numeric && InputLeaves.text(value).let { it.isNotEmpty() && it.replace(",", "").toDoubleOrNull() == null }),
                    visualTransformation = if (c.type == ComponentType.SECURE_FIELD) PasswordVisualTransformation() else VisualTransformation.None,
                    modifier = Modifier.fillMaxWidth(),
                )
                if (c.type == ComponentType.TRANSCRIBE_FIELD) Note("話して入力は $UNAVAILABLE_SHORT。キーボードで入力してください")
            }
            ComponentType.PICKER -> {
                Caption(caption)
                FlowRow(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    InputLeaves.options(form, field).forEach { o ->
                        FilterChip(selected = value?.stringValue == o, onClick = { set(JSONValue.Str(o)) }, label = { Text(o) })
                    }
                }
            }
            ComponentType.TOGGLE -> Row(verticalAlignment = Alignment.CenterVertically) {
                Text(caption, Modifier.weight(1f))
                Switch(checked = value?.truthy == true, onCheckedChange = { set(JSONValue.Bool(it)) }, enabled = !readOnly)
            }
            ComponentType.DATE_PICKER -> DateInput(caption, form.entity.field(field)?.type == FieldType.DATETIME, value, flagged, ::set)
            ComponentType.TIME_PICKER -> TimeInput(caption, value, form.entity.field(field)?.type, ::set)
            ComponentType.SLIDER -> {
                val min = c.min ?: 0.0
                val max = (c.max ?: 100.0).coerceAtLeast(min + 0.0001)
                val step = c.step
                val n = (value?.doubleValue ?: min).coerceIn(min, max)
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Text(caption, Modifier.weight(1f))
                    Text(JSONValue.numberText(n) + (c.unit?.let { " $it" } ?: ""))
                }
                val steps = step?.takeIf { it > 0 }?.let { ((max - min) / it).toInt() - 1 }?.coerceIn(0, 200) ?: 0
                Slider(
                    value = n.toFloat(),
                    onValueChange = { v -> set(JSONValue.Num(snap(v.toDouble(), min, step))) },
                    valueRange = min.toFloat()..max.toFloat(),
                    steps = steps,
                    enabled = !readOnly,
                )
            }
            ComponentType.STEPPER -> {
                val step = c.step?.takeIf { it > 0 } ?: 1.0
                val n = value?.doubleValue
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Text(caption, Modifier.weight(1f))
                    TextButton(enabled = !readOnly, onClick = { set(JSONValue.Num(bound((n ?: c.min ?: 0.0) - step, c.min, c.max))) }) { Text("−") }
                    Text(n?.let { JSONValue.numberText(roundToStep(it, step)) } ?: "—")
                    TextButton(enabled = !readOnly, onClick = { set(JSONValue.Num(bound((n ?: c.min ?: 0.0) + if (n == null) 0.0 else step, c.min, c.max))) }) { Text("+") }
                }
            }
            ComponentType.RATING -> RatingLeaf(c, value?.doubleValue?.toInt() ?: 0, caption) { set(JSONValue.Num(it.toDouble())) }
            ComponentType.COLOR_PICKER -> ColorInput(caption, value?.stringValue, ::set)
            ComponentType.PHOTO_FIELD, ComponentType.VIDEO_FIELD, ComponentType.AUDIO_RECORDER -> {
                val kind = when (c.type) {
                    ComponentType.PHOTO_FIELD -> "撮影"
                    ComponentType.VIDEO_FIELD -> "動画の撮影"
                    else -> "録音"
                }
                Caption(caption)
                Note("$kind は $UNAVAILABLE_SHORT。ファイルから選べます")
                Row(horizontalArrangement = Arrangement.spacedBy(8.dp), verticalAlignment = Alignment.CenterVertically) {
                    OutlinedButton(enabled = !readOnly, onClick = { pickFile(caption)?.let { set(JSONValue.Str(it.toURI().toString())) } }) { Text("ファイルを選ぶ") }
                    value?.stringValue?.let { Text(File(java.net.URI(it).path ?: it).name, style = MaterialTheme.typography.bodySmall) }
                    if (value != null && !value.isNull) TextButton(enabled = !readOnly, onClick = { set(JSONValue.Null) }) { Text("消す") }
                }
                if (c.type == ComponentType.PHOTO_FIELD) value?.stringValue?.let { ImageLeaf(c, it) }
            }
            ComponentType.DRAWING_FIELD -> DrawingInput(caption, value?.stringValue, readOnly, ::set)
            ComponentType.QR_SCANNER -> {
                Caption(caption)
                Note("カメラでの読み取りは $UNAVAILABLE_SHORT。画像ファイルかクリップボードの画像から読めます")
                Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    OutlinedButton(enabled = !readOnly, onClick = {
                        val img = pickFile(caption)?.let { runCatching { ImageIO.read(it) }.getOrNull() }
                        readCode(img, onMessage)?.let { set(JSONValue.Str(it)) }
                    }) { Text("画像から読む") }
                    OutlinedButton(enabled = !readOnly, onClick = { readCode(clipboardImage(), onMessage)?.let { set(JSONValue.Str(it)) } }) { Text("クリップボードから") }
                }
                value?.stringValue?.let { Text(it, style = MaterialTheme.typography.bodyMedium) }
            }
            ComponentType.LOCATION_PICKER, ComponentType.LOCATION_BUTTON -> {
                Caption(caption)
                UnavailableFrame(if (c.type == ComponentType.LOCATION_BUTTON) "現在地の記録は$UNAVAILABLE_SHORT" else "地図で場所を選ぶのは$UNAVAILABLE_SHORT")
                value?.takeIf { !it.isNull }?.let { Text(it.displayString, style = MaterialTheme.typography.bodySmall) }
            }
            else -> UnavailableFrame("デスクトップ版ではまだ使えない部品: ${DesktopLeaves.typeName(c)}")
        }
        if (flagged) Text("$caption を入力してください", color = MaterialTheme.colorScheme.error, style = MaterialTheme.typography.bodySmall)
    }
}

private const val UNAVAILABLE_SHORT = "このパソコンでは使えません"

/** 「このパソコンでは使えません」の枠。 */
@Composable
fun UnavailableFrame(text: String) {
    Box(
        Modifier.fillMaxWidth().border(1.dp, Color(0x33000000), RoundedCornerShape(8.dp)).padding(12.dp),
        contentAlignment = Alignment.CenterStart,
    ) { Text(text, color = Color.Gray, style = MaterialTheme.typography.bodySmall) }
}

@Composable
private fun Caption(text: String) {
    if (text.isNotEmpty()) Text(text, style = MaterialTheme.typography.labelLarge)
}

@Composable
private fun Note(text: String) = Text(text, color = Color.Gray, style = MaterialTheme.typography.bodySmall)

private fun snap(v: Double, min: Double, step: Double?): Double {
    if (step == null || step <= 0) return v
    return roundToStep(min + Math.round((v - min) / step) * step, step)
}

/** step の小数の桁に丸める(0.1 刻みで 70.30000000001 を出さない)。 */
internal fun roundToStep(v: Double, step: Double): Double {
    val scale = java.math.BigDecimal(step.toString()).stripTrailingZeros().scale().coerceAtLeast(0)
    return java.math.BigDecimal(v).setScale(scale, java.math.RoundingMode.HALF_UP).toDouble()
}

private fun bound(v: Double, min: Double?, max: Double?): Double = v.coerceAtLeast(min ?: Double.NEGATIVE_INFINITY).coerceAtMost(max ?: Double.POSITIVE_INFINITY)

/** 日付(yyyy-MM-dd)・日時(yyyy-MM-ddTHH:mm)。「今日」「前の日」「次の日」で動かす。 */
@Composable
private fun DateInput(caption: String, withTime: Boolean, value: JSONValue?, flagged: Boolean, set: (JSONValue) -> Unit) {
    val zone = jp.kz2601.metamon.dsl.records.FieldCoding.zone
    val instant = value?.let { jp.kz2601.metamon.dsl.records.FieldCoding.date(it) }
    val shown = instant?.let {
        val t = java.time.LocalDateTime.ofInstant(it, zone)
        if (withTime) t.toString().take(16) else t.toLocalDate().toString()
    } ?: InputLeaves.text(value)
    fun write(text: String) {
        val v = if (withTime) {
            runCatching { java.time.LocalDateTime.parse(text).atZone(zone).toInstant() }.getOrNull()?.let { jp.kz2601.metamon.dsl.records.FieldCoding.formatDatetime(it) }
        } else {
            runCatching { java.time.LocalDate.parse(text).toString() }.getOrNull()
        }
        set(JSONValue.Str(v ?: text))
    }
    fun shift(days: Long) {
        val base = instant ?: jp.kz2601.metamon.dsl.expr.DSLClock.now()
        val t = java.time.LocalDateTime.ofInstant(base, zone).plusDays(days)
        write(if (withTime) t.toString().take(16) else t.toLocalDate().toString())
    }
    Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(4.dp)) {
        OutlinedTextField(
            value = shown,
            onValueChange = ::write,
            label = { Text(caption) },
            placeholder = { Text(if (withTime) "2026-11-12T09:00" else "2026-11-12") },
            singleLine = true,
            isError = flagged,
            modifier = Modifier.weight(1f),
        )
        TextButton(onClick = { shift(-1) }) { Text("‹") }
        TextButton(onClick = { shift(0) }) { Text("今日") }
        TextButton(onClick = { shift(1) }) { Text("›") }
    }
}

/** 時刻(HH:mm)。datetime の field なら今日のその時刻として書く。 */
@Composable
private fun TimeInput(caption: String, value: JSONValue?, type: FieldType?, set: (JSONValue) -> Unit) {
    val zone = jp.kz2601.metamon.dsl.records.FieldCoding.zone
    val shown = when {
        type == FieldType.DATETIME -> value?.let { jp.kz2601.metamon.dsl.records.FieldCoding.date(it) }
            ?.let { java.time.LocalTime.ofInstant(it, zone).toString().take(5) } ?: InputLeaves.text(value)
        else -> InputLeaves.text(value)
    }
    OutlinedTextField(
        value = shown,
        onValueChange = { text ->
            val t = runCatching { java.time.LocalTime.parse(text) }.getOrNull()
            val v = when {
                t == null -> text
                type == FieldType.DATETIME -> jp.kz2601.metamon.dsl.records.FieldCoding.formatDatetime(
                    java.time.LocalDate.ofInstant(jp.kz2601.metamon.dsl.expr.DSLClock.now(), zone).atTime(t).atZone(zone).toInstant(),
                )
                else -> t.toString().take(5)
            }
            set(JSONValue.Str(v))
        },
        label = { Text(caption) },
        placeholder = { Text("07:00") },
        singleLine = true,
        modifier = Modifier.fillMaxWidth(),
    )
}

private val PALETTE = listOf("#E53935", "#FB8C00", "#FDD835", "#43A047", "#00ACC1", "#1E88E5", "#5E35B1", "#D81B60", "#6D4C41", "#757575", "#000000", "#FFFFFF")

@Composable
private fun ColorInput(caption: String, hex: String?, set: (JSONValue) -> Unit) {
    Caption(caption)
    FlowRow(horizontalArrangement = Arrangement.spacedBy(6.dp), verticalArrangement = Arrangement.spacedBy(6.dp)) {
        PALETTE.forEach { h ->
            val selected = hex.equals(h, ignoreCase = true)
            Box(
                Modifier.size(28.dp).background(parseHex(h) ?: Color.Gray, CircleShape)
                    .border(if (selected) 3.dp else 1.dp, if (selected) MaterialTheme.colorScheme.primary else Color(0x33000000), CircleShape)
                    .clickable { set(JSONValue.Str(h)) },
            )
        }
    }
    OutlinedTextField(value = hex.orEmpty(), onValueChange = { set(JSONValue.Str(it)) }, singleLine = true, placeholder = { Text("#5E35B1") }, modifier = Modifier.fillMaxWidth())
}

private fun parseHex(h: String): Color? = h.removePrefix("#").takeIf { it.length == 6 }?.toLongOrNull(16)?.let { Color(0xFF000000 or it) }

/** マウス・ペンで描く。「保存」で PNG に書き、その file URI を値にする。 */
@Composable
private fun DrawingInput(caption: String, saved: String?, readOnly: Boolean, set: (JSONValue) -> Unit) {
    val strokes = remember { mutableStateListOf<List<Offset>>() }
    val current = remember { mutableStateListOf<Offset>() }
    Caption(caption)
    Canvas(
        Modifier.fillMaxWidth().height(200.dp).background(Color.White, RoundedCornerShape(8.dp)).border(1.dp, Color(0x33000000), RoundedCornerShape(8.dp))
            .pointerInput(readOnly) {
                if (readOnly) return@pointerInput
                detectDragGestures(
                    onDragStart = { current.clear(); current.add(it) },
                    onDrag = { change, _ -> current.add(change.position) },
                    onDragEnd = { strokes.add(current.toList()); current.clear() },
                )
            },
    ) {
        (strokes + listOf(current.toList())).forEach { s ->
            s.zipWithNext().forEach { (a, b) -> drawLine(Color.Black, a, b, strokeWidth = 4f, cap = StrokeCap.Round) }
        }
    }
    Row(horizontalArrangement = Arrangement.spacedBy(8.dp), verticalAlignment = Alignment.CenterVertically) {
        TextButton(enabled = !readOnly && strokes.isNotEmpty(), onClick = {
            val file = Drawing.writePng(strokes.toList(), 600, 300)
            set(JSONValue.Str(file.toURI().toString()))
        }) { Text("この絵を使う") }
        TextButton(enabled = !readOnly, onClick = { strokes.clear(); set(JSONValue.Null) }) { Text("消す") }
        if (saved != null) Text("保存済み", style = MaterialTheme.typography.bodySmall, color = Color.Gray)
    }
}

/** 描いた線 → PNG(一時ファイル)。 */
object Drawing {
    fun writePng(strokes: List<List<Offset>>, w: Int, h: Int): File {
        val img = BufferedImage(w, h, BufferedImage.TYPE_INT_ARGB)
        val g = img.createGraphics()
        g.setRenderingHint(AwtHints.KEY_ANTIALIASING, AwtHints.VALUE_ANTIALIAS_ON)
        g.color = AwtColor.WHITE
        g.fillRect(0, 0, w, h)
        g.color = AwtColor.BLACK
        g.stroke = AwtStroke(4f, AwtStroke.CAP_ROUND, AwtStroke.JOIN_ROUND)
        strokes.forEach { s -> s.zipWithNext().forEach { (a, b) -> g.drawLine(a.x.toInt(), a.y.toInt(), b.x.toInt(), b.y.toInt()) } }
        g.dispose()
        val file = File.createTempFile("drawing-", ".png")
        ImageIO.write(img, "png", file)
        return file
    }
}

private fun pickFile(title: String): File? {
    if (GraphicsEnvironment.isHeadless()) return null
    val d = FileDialog(null as Frame?, title.ifEmpty { "ファイルを選ぶ" }, FileDialog.LOAD)
    d.isVisible = true
    val name = d.file ?: return null
    return File(d.directory, name)
}

private fun clipboardImage(): BufferedImage? = runCatching {
    val clip = Toolkit.getDefaultToolkit().systemClipboard
    if (!clip.isDataFlavorAvailable(DataFlavor.imageFlavor)) return null
    val img = clip.getData(DataFlavor.imageFlavor) as java.awt.Image
    BufferedImage(img.getWidth(null), img.getHeight(null), BufferedImage.TYPE_INT_RGB).also { it.createGraphics().drawImage(img, 0, 0, null) }
}.getOrNull()

private fun readCode(img: BufferedImage?, onMessage: (String) -> Unit): String? {
    if (img == null) {
        onMessage("画像がありません")
        return null
    }
    return InputLeaves.decodeCode(img) ?: run {
        onMessage("コードを読み取れませんでした")
        null
    }
}
