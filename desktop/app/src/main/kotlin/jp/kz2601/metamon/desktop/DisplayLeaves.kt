package jp.kz2601.metamon.desktop

import androidx.compose.foundation.Canvas
import androidx.compose.foundation.Image
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.aspectRatio
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.LinearProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.geometry.Size
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.ImageBitmap
import androidx.compose.ui.graphics.Path
import androidx.compose.ui.graphics.StrokeCap
import androidx.compose.ui.graphics.drawscope.Stroke
import androidx.compose.ui.graphics.toComposeImageBitmap
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.text.AnnotatedString
import androidx.compose.ui.text.LinkAnnotation
import androidx.compose.ui.text.SpanStyle
import androidx.compose.ui.text.TextLinkStyles
import androidx.compose.ui.text.buildAnnotatedString
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontStyle
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextDecoration
import androidx.compose.ui.text.withStyle
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.ComponentKind
import jp.kz2601.metamon.dsl.ComponentType
import jp.kz2601.metamon.dsl.Size as DslSize
import jp.kz2601.metamon.dsl.records.Aggregator
import jp.kz2601.metamon.dsl.records.FieldCoding
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import java.net.URI
import java.time.DayOfWeek
import java.time.LocalDate
import java.time.YearMonth
import java.time.temporal.TemporalAdjusters
import java.util.Base64
import kotlin.math.max
import kotlin.math.min

// 表示の部品(#1742): image・avatar・badge・markdown・stat・chart・progressBar・gauge・rating・calendar。

/** 系列の色(bar / pie)。iOS・Android の既定の配色に近い 6 色。 */
private val SERIES = listOf(
    Color(0xFF4F7CFF), Color(0xFF34C759), Color(0xFFFF9F0A), Color(0xFFAF52DE), Color(0xFFFF375F), Color(0xFF5AC8FA),
)

fun dslColor(c: jp.kz2601.metamon.dsl.Color?): Color? = when (c) {
    jp.kz2601.metamon.dsl.Color.RED -> Color(0xFFE5484D)
    jp.kz2601.metamon.dsl.Color.GREEN -> Color(0xFF30A46C)
    jp.kz2601.metamon.dsl.Color.ORANGE -> Color(0xFFF76808)
    jp.kz2601.metamon.dsl.Color.BLUE -> Color(0xFF0091FF)
    jp.kz2601.metamon.dsl.Color.ACCENT, jp.kz2601.metamon.dsl.Color.PRIMARY -> Color(0xFF4F7CFF)
    jp.kz2601.metamon.dsl.Color.SECONDARY -> Color.Gray
    else -> null
}

@Composable
fun BadgeLeaf(c: Component) {
    val tint = dslColor(c.color) ?: MaterialTheme.colorScheme.primary
    Text(
        c.text ?: c.label.orEmpty(),
        style = MaterialTheme.typography.labelMedium,
        color = tint,
        modifier = Modifier.clip(RoundedCornerShape(50)).background(tint.copy(alpha = 0.14f)).padding(horizontal = 10.dp, vertical = 3.dp),
    )
}

// MARK: - markdown

/** 行単位(見出し・箇条書き・番号・引用・コード)+ 行内(**太字**・*斜体*・`コード`・[リンク](url))。 */
@Composable
fun MarkdownLeaf(text: String) {
    val link = TextLinkStyles(SpanStyle(color = MaterialTheme.colorScheme.primary, textDecoration = TextDecoration.Underline))
    val blocks = remember(text) { Markdown.blocks(text) }
    Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
        blocks.forEach { b ->
            when (b) {
                is Markdown.Block.Heading -> Text(
                    Markdown.inline(b.text, link),
                    style = when (b.level) {
                        1 -> MaterialTheme.typography.titleLarge
                        2 -> MaterialTheme.typography.titleMedium
                        else -> MaterialTheme.typography.titleSmall
                    },
                )
                is Markdown.Block.Item -> Row {
                    Text(b.marker, Modifier.width(22.dp))
                    Text(Markdown.inline(b.text, link))
                }
                is Markdown.Block.Quote -> Text(
                    Markdown.inline(b.text, link),
                    color = Color.Gray,
                    modifier = Modifier.padding(start = 8.dp),
                )
                is Markdown.Block.Code -> Text(
                    b.text,
                    fontFamily = FontFamily.Monospace,
                    fontSize = 13.sp,
                    modifier = Modifier.fillMaxWidth().clip(RoundedCornerShape(6.dp)).background(Color(0x14000000)).padding(8.dp),
                )
                is Markdown.Block.Paragraph -> Text(Markdown.inline(b.text, link))
            }
        }
    }
}

object Markdown {
    sealed interface Block {
        data class Heading(val level: Int, val text: String) : Block
        data class Item(val marker: String, val text: String) : Block
        data class Quote(val text: String) : Block
        data class Code(val text: String) : Block
        data class Paragraph(val text: String) : Block
    }

    fun blocks(src: String): List<Block> {
        val out = mutableListOf<Block>()
        val para = mutableListOf<String>()
        fun flush() {
            if (para.isNotEmpty()) out += Block.Paragraph(para.joinToString("\n"))
            para.clear()
        }
        val lines = src.lines()
        var i = 0
        while (i < lines.size) {
            val line = lines[i]
            val t = line.trim()
            when {
                t.startsWith("```") -> {
                    flush()
                    val code = mutableListOf<String>()
                    i++
                    while (i < lines.size && !lines[i].trim().startsWith("```")) code += lines[i++]
                    out += Block.Code(code.joinToString("\n"))
                }
                t.isEmpty() -> flush()
                Regex("^#{1,6} ").containsMatchIn(t) -> {
                    flush()
                    val level = t.takeWhile { it == '#' }.length
                    out += Block.Heading(level, t.drop(level).trim())
                }
                Regex("^[-*+] ").containsMatchIn(t) -> {
                    flush()
                    out += Block.Item("•", t.drop(2))
                }
                Regex("^\\d+[.)] ").containsMatchIn(t) -> {
                    flush()
                    val n = t.takeWhile { it.isDigit() }
                    out += Block.Item("$n.", t.drop(n.length + 1).trim())
                }
                t.startsWith(">") -> {
                    flush()
                    out += Block.Quote(t.drop(1).trim())
                }
                else -> para += t
            }
            i++
        }
        flush()
        return out
    }

    private val INLINE = Regex("\\*\\*(.+?)\\*\\*|__(.+?)__|\\*(.+?)\\*|_(.+?)_|`(.+?)`|\\[(.+?)]\\((.+?)\\)")

    fun inline(s: String, link: TextLinkStyles? = null): AnnotatedString = buildAnnotatedString {
        var at = 0
        for (m in INLINE.findAll(s)) {
            append(s.substring(at, m.range.first))
            val g = m.groupValues
            when {
                g[1].isNotEmpty() || g[2].isNotEmpty() -> withStyle(SpanStyle(fontWeight = FontWeight.Bold)) { append(g[1].ifEmpty { g[2] }) }
                g[3].isNotEmpty() || g[4].isNotEmpty() -> withStyle(SpanStyle(fontStyle = FontStyle.Italic)) { append(g[3].ifEmpty { g[4] }) }
                g[5].isNotEmpty() -> withStyle(SpanStyle(fontFamily = FontFamily.Monospace, background = Color(0x14000000))) { append(g[5]) }
                else -> {
                    pushLink(LinkAnnotation.Url(g[7], link))
                    append(g[6])
                    pop()
                }
            }
            at = m.range.last + 1
        }
        append(s.substring(at))
    }
}

// MARK: - image / avatar

/** https・data URL の画像を読む(読めなければ null)。Coil は入れず JVM の HTTP + Skia で十分。 */
@Composable
private fun rememberRemoteImage(src: String?): ImageBitmap? {
    var bitmap by remember(src) { mutableStateOf<ImageBitmap?>(null) }
    LaunchedEffect(src) {
        if (src.isNullOrBlank()) return@LaunchedEffect
        bitmap = withContext(Dispatchers.IO) {
            runCatching {
                val bytes = if (src.startsWith("data:")) {
                    Base64.getDecoder().decode(src.substringAfter("base64,"))
                } else if (src.startsWith("https://")) {
                    URI(src).toURL().openStream().use { it.readBytes() }
                } else {
                    return@runCatching null
                }
                org.jetbrains.skia.Image.makeFromEncoded(bytes).toComposeImageBitmap()
            }.getOrNull()
        }
    }
    return bitmap
}

@Composable
fun ImageLeaf(c: Component, src: String?) {
    val bitmap = rememberRemoteImage(src ?: c.url ?: c.image)
    val h = when (c.height?.wire) { "small" -> 120.dp; "large" -> 280.dp; else -> 200.dp }
    Box(
        Modifier.fillMaxWidth().heightIn(min = 80.dp).height(h).clip(RoundedCornerShape(10.dp)).background(Color(0x11000000)),
        contentAlignment = Alignment.Center,
    ) {
        if (bitmap != null) {
            Image(bitmap, c.label, Modifier.fillMaxSize(), contentScale = ContentScale.Crop)
        } else {
            Text("画像なし", color = Color.Gray, style = MaterialTheme.typography.bodySmall)
        }
    }
}

@Composable
fun AvatarLeaf(c: Component, src: String?) {
    val bitmap = rememberRemoteImage(src ?: c.image)
    val d = when (c.size) { DslSize.SMALL, DslSize.COMPACT -> 40.dp; DslSize.LARGE -> 88.dp; else -> 60.dp }
    Box(Modifier.fillMaxWidth(), contentAlignment = Alignment.Center) {
        Box(
            Modifier.size(d).clip(CircleShape).background(MaterialTheme.colorScheme.primaryContainer),
            contentAlignment = Alignment.Center,
        ) {
            if (bitmap != null) {
                Image(bitmap, c.name, Modifier.fillMaxSize(), contentScale = ContentScale.Crop)
            } else {
                Text(initials(c.name), style = MaterialTheme.typography.titleLarge, color = MaterialTheme.colorScheme.onPrimaryContainer)
            }
        }
    }
}

/** 名前の頭文字(英字は 2 語の頭・それ以外は 1 文字)。無ければ人の印。 */
fun initials(name: String?): String {
    val words = name?.trim()?.split(Regex("\\s+"))?.filter { it.isNotEmpty() }.orEmpty()
    if (words.isEmpty()) return "👤"
    return if (words.size >= 2 && words[0].first().isLetter() && words[0].first().code < 128) {
        "${words[0].first()}${words[1].first()}".uppercase()
    } else {
        words[0].take(1).uppercase()
    }
}

// MARK: - stat / progressBar / gauge / rating

@Composable
fun StatLeaf(c: Component, data: LeafData) {
    val (text, _) = StatValue.resolve(c, data)
    Column(
        Modifier.fillMaxWidth().clip(RoundedCornerShape(12.dp)).background(MaterialTheme.colorScheme.surfaceVariant).padding(12.dp),
        verticalArrangement = Arrangement.spacedBy(2.dp),
    ) {
        c.label?.let { Text(it, style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.onSurfaceVariant) }
        Text(text, style = MaterialTheme.typography.headlineSmall, fontWeight = FontWeight.SemiBold)
    }
}

@Composable
fun ProgressLeaf(c: Component, data: LeafData) {
    val raw = ProgressValue.ratio(c, data)
    val over = c.tone == null && raw >= 1 || c.tone?.wire == "negative" && raw > 1
    Column(Modifier.fillMaxWidth(), verticalArrangement = Arrangement.spacedBy(6.dp)) {
        Row(Modifier.fillMaxWidth()) {
            Text(c.label.orEmpty(), Modifier.weight(1f), style = MaterialTheme.typography.labelLarge)
            Text(ProgressValue.percent(c, data), style = MaterialTheme.typography.labelLarge)
        }
        LinearProgressIndicator(
            progress = { raw.toFloat().coerceIn(0f, 1f) },
            color = if (over) MaterialTheme.colorScheme.error else MaterialTheme.colorScheme.primary,
            modifier = Modifier.fillMaxWidth().height(8.dp),
            drawStopIndicator = {},
        )
    }
}

@Composable
fun GaugeLeaf(c: Component, data: LeafData) {
    val (value, unit) = GaugeValue.value(c, data)
    val lo = c.min ?: 0.0
    val hi = c.max ?: 1.0
    val frac = if (hi > lo) ((value - lo) / (hi - lo)).coerceIn(0.0, 1.0).toFloat() else 0f
    val track = MaterialTheme.colorScheme.surfaceVariant
    val fill = MaterialTheme.colorScheme.primary
    Column(Modifier.fillMaxWidth(), horizontalAlignment = Alignment.CenterHorizontally) {
        Box(Modifier.width(140.dp).aspectRatio(2f), contentAlignment = Alignment.BottomCenter) {
            Canvas(Modifier.fillMaxSize()) {
                val stroke = 14.dp.toPx()
                val r = min(size.width / 2, size.height) - stroke / 2
                val topLeft = Offset(size.width / 2 - r, size.height - r)
                val arc = Size(r * 2, r * 2)
                drawArc(track, 180f, 180f, false, topLeft, arc, style = Stroke(stroke, cap = StrokeCap.Round))
                drawArc(fill, 180f, 180f * frac, false, topLeft, arc, style = Stroke(stroke, cap = StrokeCap.Round))
            }
            Text(Aggregator.format(value, null, unit), style = MaterialTheme.typography.titleMedium)
        }
        c.label?.let { Text(it, style = MaterialTheme.typography.labelMedium, color = Color.Gray) }
    }
}

/** rating: 星。field があればフォームの値を読み書きする。 */
@Composable
fun RatingLeaf(c: Component, value: Int, label: String?, onChange: ((Int) -> Unit)?) {
    val max = (c.max ?: 5.0).toInt().coerceIn(1, 10)
    Column {
        label?.let { Text(it, style = MaterialTheme.typography.labelLarge) }
        Row {
            (1..max).forEach { n ->
                Text(
                    if (n <= value) "★" else "☆",
                    fontSize = 26.sp,
                    color = if (n <= value) Color(0xFFFFB400) else Color.Gray,
                    modifier = Modifier.then(if (onChange != null) Modifier.clickable { onChange(if (n == value) 0 else n) } else Modifier).padding(2.dp),
                )
            }
        }
    }
}

// MARK: - chart

@Composable
fun ChartLeaf(c: Component, data: LeafData) {
    val buckets = ChartBuckets.buckets(c, data)
    Column(Modifier.fillMaxWidth(), verticalArrangement = Arrangement.spacedBy(8.dp)) {
        c.title?.let { Text(it, style = MaterialTheme.typography.titleSmall) }
        if (buckets.isEmpty()) {
            Box(Modifier.fillMaxWidth().height(120.dp), contentAlignment = Alignment.Center) {
                Text("まだデータがありません", color = Color.Gray)
            }
            return@Column
        }
        when (c.kind) {
            ComponentKind.PIE -> PieChart(buckets)
            ComponentKind.LINE -> LineChart(buckets)
            else -> BarChart(buckets)
        }
    }
}

private fun axisLabel(s: String): String = if (s.codePointCount(0, s.length) > 6) s.take(5) + "…" else s

@Composable
private fun BarChart(buckets: List<Aggregator.Bucket>) {
    val top = max(buckets.maxOf { it.value }, 0.0).takeIf { it > 0 } ?: 1.0
    Row(Modifier.fillMaxWidth().height(160.dp), horizontalArrangement = Arrangement.spacedBy(6.dp), verticalAlignment = Alignment.Bottom) {
        buckets.forEach { b ->
            Column(Modifier.weight(1f), horizontalAlignment = Alignment.CenterHorizontally) {
                Text(chartNumber(b.value), style = MaterialTheme.typography.labelSmall)
                Box(
                    Modifier.fillMaxWidth().height((110 * (max(b.value, 0.0) / top)).dp.coerceAtLeast(2.dp))
                        .clip(RoundedCornerShape(topStart = 4.dp, topEnd = 4.dp)).background(SERIES[0]),
                )
                Text(axisLabel(b.label), style = MaterialTheme.typography.labelSmall, maxLines = 1)
            }
        }
    }
}

@Composable
private fun LineChart(buckets: List<Aggregator.Bucket>) {
    val color = SERIES[0]
    val grid = Color(0x22000000)
    val lo = min(0.0, buckets.minOf { it.value })
    val hi = buckets.maxOf { it.value }.let { if (it > lo) it else lo + 1 }
    Column {
        Canvas(Modifier.fillMaxWidth().height(140.dp)) {
            val stepX = if (buckets.size > 1) size.width / (buckets.size - 1) else 0f
            fun pt(i: Int) = Offset(
                if (buckets.size > 1) stepX * i else size.width / 2,
                size.height - ((buckets[i].value - lo) / (hi - lo) * size.height).toFloat(),
            )
            drawLine(grid, Offset(0f, size.height), Offset(size.width, size.height))
            val path = Path().apply {
                moveTo(pt(0).x, pt(0).y)
                for (i in 1 until buckets.size) lineTo(pt(i).x, pt(i).y)
            }
            drawPath(path, color, style = Stroke(3.dp.toPx(), cap = StrokeCap.Round))
            buckets.indices.forEach { drawCircle(color, 4.dp.toPx(), pt(it)) }
        }
        Row(Modifier.fillMaxWidth()) {
            buckets.forEach { Text(axisLabel(it.label), Modifier.weight(1f), style = MaterialTheme.typography.labelSmall, maxLines = 1) }
        }
    }
}

@Composable
private fun PieChart(buckets: List<Aggregator.Bucket>) {
    val shown = buckets.filter { it.value > 0 }
    val total = shown.sumOf { it.value }
    Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(16.dp)) {
        Canvas(Modifier.size(140.dp)) {
            var start = -90f
            shown.forEachIndexed { i, b ->
                val sweep = (b.value / total * 360).toFloat()
                drawArc(SERIES[i % SERIES.size], start, sweep, true)
                start += sweep
            }
            if (shown.isEmpty()) drawCircle(Color(0x22000000))
        }
        Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
            shown.forEachIndexed { i, b ->
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Box(Modifier.size(10.dp).clip(CircleShape).background(SERIES[i % SERIES.size]))
                    Text("  ${b.label}  ${chartNumber(b.value)}", style = MaterialTheme.typography.bodySmall)
                }
            }
        }
    }
}

private fun chartNumber(v: Double): String = Aggregator.format(v, null)

// MARK: - calendar

/** 読み取りの月の表(今月・日曜始まり)。記録のある日に点を打ち、件数を出す。 */
@Composable
fun CalendarLeaf(c: Component, data: LeafData, onDateTap: ((LocalDate) -> Unit)?) {
    val today = LocalDate.now(FieldCoding.zone)
    val month = YearMonth.from(today)
    val counts = remember(c, data) {
        val field = c.dateField
        data.filtered(c).mapNotNull { r -> field?.let { FieldCoding.date(r[it]) }?.let { LocalDate.ofInstant(it, FieldCoding.zone) } }
            .groupingBy { it }.eachCount()
    }
    val first = month.atDay(1)
    val start = first.with(TemporalAdjusters.previousOrSame(DayOfWeek.SUNDAY))
    val weeks = ((first.dayOfWeek.value % 7) + month.lengthOfMonth() + 6) / 7
    Column(Modifier.fillMaxWidth(), verticalArrangement = Arrangement.spacedBy(4.dp)) {
        Text(c.title ?: "${month.year}年${month.monthValue}月", style = MaterialTheme.typography.titleSmall)
        Row(Modifier.fillMaxWidth()) {
            listOf("日", "月", "火", "水", "木", "金", "土").forEach {
                Text(it, Modifier.weight(1f), style = MaterialTheme.typography.labelSmall, color = Color.Gray)
            }
        }
        repeat(weeks) { w ->
            Row(Modifier.fillMaxWidth()) {
                repeat(7) { d ->
                    val day = start.plusDays((w * 7 + d).toLong())
                    val inMonth = day.monthValue == month.monthValue
                    val n = counts[day] ?: 0
                    Column(
                        Modifier.weight(1f).clip(RoundedCornerShape(6.dp))
                            .background(if (day == today) MaterialTheme.colorScheme.primaryContainer else Color.Transparent)
                            .then(if (onDateTap != null && inMonth) Modifier.clickable { onDateTap(day) } else Modifier)
                            .padding(vertical = 4.dp),
                        horizontalAlignment = Alignment.CenterHorizontally,
                    ) {
                        Text(
                            if (inMonth) day.dayOfMonth.toString() else "",
                            style = MaterialTheme.typography.bodySmall,
                            color = if (d == 0) Color(0xFFE5484D) else if (d == 6) Color(0xFF0091FF) else Color.Unspecified,
                        )
                        Box(Modifier.size(5.dp).clip(CircleShape).background(if (inMonth && n > 0) SERIES[0] else Color.Transparent))
                    }
                }
            }
        }
    }
}

/** 表示の部品(DesktopApp の Node が呼ぶ)。 */
val DISPLAY_LEAVES: Set<ComponentType> = setOf(
    ComponentType.IMAGE, ComponentType.AVATAR, ComponentType.STAT, ComponentType.CHART, ComponentType.PROGRESS_BAR,
    ComponentType.GAUGE, ComponentType.RATING, ComponentType.CALENDAR,
)
