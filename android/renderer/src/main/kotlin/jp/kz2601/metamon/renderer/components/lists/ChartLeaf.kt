package jp.kz2601.metamon.renderer.components.lists

import androidx.compose.foundation.Canvas
import androidx.compose.foundation.background
import androidx.compose.foundation.gestures.detectTapGestures
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
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.geometry.CornerRadius
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.geometry.Size
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.Path
import androidx.compose.ui.graphics.StrokeCap
import androidx.compose.ui.graphics.drawscope.Stroke
import androidx.compose.ui.input.pointer.pointerInput
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.ComponentKind
import jp.kz2601.metamon.dsl.records.Aggregator
import jp.kz2601.metamon.renderer.R
import jp.kz2601.metamon.renderer.registry.RenderContext
import jp.kz2601.metamon.renderer.theme.RenderTheme
import jp.kz2601.metamon.renderer.theme.colors
import jp.kz2601.metamon.renderer.theme.renderCard
import java.util.Locale
import kotlin.math.atan2
import kotlin.math.hypot
import kotlin.math.max
import kotlin.math.min

/**
 * `chart`: groupBy で束ねた集計を bar | line | pie で描く。iOS の `ChartLeaf`(Swift Charts)。
 * グラフのライブラリは足さず Compose の Canvas で描く。押した棒・点・扇の値を上に出し、読み上げには要約(最大・最小)を渡す(L4)。
 */
@Composable
fun ChartLeaf(c: Component, ctx: RenderContext, modifier: Modifier = Modifier) {
    val data = rememberLeafData(ctx.runtime)
    // 日付の見出し(「3月」「3/4週」)は端末の言語で
    val buckets = ChartBuckets.buckets(c, data, Locale.getDefault())
    var selected by remember(buckets) { mutableStateOf<Int?>(null) }
    Column(modifier.renderCard(), verticalArrangement = Arrangement.spacedBy(8.dp)) {
        c.title?.let { Text(it, style = RenderTheme.Fonts.cardTitle) }
        if (buckets.isEmpty()) {
            // 記録すると出ることを伝える(L4)
            EmptyPlaceholder("chart.bar", stringResource(R.string.lists_chart_empty))
            return@Column
        }
        // 押していない時も高さを取っておく(押すたびにグラフが上下しない)
        Text(
            selected?.let { buckets.getOrNull(it) }?.let { ChartReadout.line(it.label, it.value) } ?: " ",
            style = RenderTheme.Fonts.legend.copy(fontWeight = FontWeight.SemiBold, fontFeatureSettings = "tnum"),
            color = RenderTheme.colors.brandA,
            maxLines = 1,
        )
        val summary = listOfNotNull(c.title, ChartReadout.summary(buckets, data.labels)).joinToString("。")
        val chartModifier = Modifier.fillMaxWidth().semantics { contentDescription = summary }
        when (c.kind) {
            ComponentKind.PIE -> PieChart(buckets, selected, { selected = it }, chartModifier)
            ComponentKind.LINE -> BarOrLineChart(buckets, line = true, selected, { selected = it }, chartModifier)
            else -> BarOrLineChart(buckets, line = false, selected, { selected = it }, chartModifier)
        }
    }
}

@Composable
private fun BarOrLineChart(buckets: List<Aggregator.Bucket>, line: Boolean, selected: Int?, onSelect: (Int?) -> Unit, modifier: Modifier) {
    val colors = RenderTheme.colors
    val hi = max(buckets.maxOf { it.value }, 0.0)
    val lo = min(buckets.minOf { it.value }, 0.0)
    val span = (hi - lo).takeIf { it > 0 } ?: 1.0
    Column(modifier) {
        Canvas(
            Modifier.fillMaxWidth().height(160.dp)
                .pointerInput(buckets) {
                    detectTapGestures { p ->
                        val i = (p.x / (size.width / buckets.size.toFloat())).toInt().coerceIn(0, buckets.size - 1)
                        onSelect(if (selected == i) null else i)
                    }
                },
        ) {
            val slot = size.width / buckets.size
            fun y(v: Double) = (size.height * (1 - (v - lo) / span)).toFloat()
            // 横の補助線(0 と最大)
            listOf(lo, hi).distinct().forEach { v -> drawLine(colors.hairline, Offset(0f, y(v)), Offset(size.width, y(v)), 1.dp.toPx()) }
            if (line) {
                val pts = buckets.mapIndexed { i, b -> Offset(slot * i + slot / 2, y(b.value)) }
                val area = Path().apply {
                    moveTo(pts.first().x, y(0.0))
                    pts.forEach { lineTo(it.x, it.y) }
                    lineTo(pts.last().x, y(0.0))
                    close()
                }
                drawPath(area, Brush.verticalGradient(listOf(colors.brandA.copy(alpha = 0.25f), colors.brandA.copy(alpha = 0.02f))))
                val path = Path().apply { pts.forEachIndexed { i, p -> if (i == 0) moveTo(p.x, p.y) else lineTo(p.x, p.y) } }
                drawPath(path, colors.brandA, style = Stroke(width = 2.5.dp.toPx(), cap = StrokeCap.Round))
                pts.forEachIndexed { i, p -> drawCircle(colors.brandA, radius = (if (selected == i) 5.dp else 3.5.dp).toPx(), center = p) }
            } else {
                val barW = slot * 0.6f
                buckets.forEachIndexed { i, b ->
                    val top = y(max(b.value, 0.0))
                    val bottom = y(min(b.value, 0.0))
                    drawRoundRect(
                        Brush.verticalGradient(listOf(colors.brandFillStart, colors.brandFillEnd), startY = top, endY = bottom),
                        topLeft = Offset(slot * i + (slot - barW) / 2, top),
                        size = Size(barW, max(bottom - top, 1f)),
                        cornerRadius = CornerRadius(6.dp.toPx()),
                        alpha = if (selected == null || selected == i) 1f else 0.4f,
                    )
                }
            }
        }
        // x 軸ラベル(#131: 長い名前は 5 文字 + …)
        Row(Modifier.fillMaxWidth().padding(top = 4.dp)) {
            buckets.forEach { b ->
                Text(
                    ChartBuckets.axisLabel(b.label),
                    Modifier.weight(1f),
                    style = RenderTheme.Fonts.legend,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    textAlign = TextAlign.Center,
                    maxLines = 1,
                    overflow = TextOverflow.Clip,
                )
            }
        }
    }
}

@Composable
private fun PieChart(buckets: List<Aggregator.Bucket>, selected: Int?, onSelect: (Int?) -> Unit, modifier: Modifier) {
    val palette = RenderTheme.colors.chartPalette
    fun color(i: Int): Color = palette[i % palette.size]
    val values = buckets.map { max(it.value, 0.0) }
    val total = values.sum().takeIf { it > 0 } ?: 1.0
    Column(modifier, verticalArrangement = Arrangement.spacedBy(8.dp)) {
        // 凡例は上(iOS と同じ)
        FlowRow(horizontalArrangement = Arrangement.spacedBy(10.dp), verticalArrangement = Arrangement.spacedBy(4.dp)) {
            buckets.forEachIndexed { i, b ->
                Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(4.dp)) {
                    Box(Modifier.size(8.dp).background(color(i), CircleShape))
                    Text(b.label, style = RenderTheme.Fonts.legend, maxLines = 1)
                }
            }
        }
        Canvas(
            Modifier.fillMaxWidth().height(160.dp)
                .pointerInput(buckets) {
                    detectTapGestures { p ->
                        val c = Offset(size.width / 2f, size.height / 2f)
                        val r = min(size.width, size.height) / 2f
                        val d = hypot(p.x - c.x, p.y - c.y)
                        if (d > r || d < r * 0.55f) return@detectTapGestures onSelect(null)
                        // 12 時から時計回りの角度 → 累積値
                        var deg = Math.toDegrees(atan2((p.y - c.y).toDouble(), (p.x - c.x).toDouble())) + 90
                        if (deg < 0) deg += 360
                        val i = ChartReadout.index(deg / 360 * total, values)
                        onSelect(if (i == selected) null else i)
                    }
                },
        ) {
            val r = min(size.width, size.height) / 2f
            val stroke = r * 0.45f
            val topLeft = Offset(size.width / 2f - r + stroke / 2, size.height / 2f - r + stroke / 2)
            val arc = Size((r - stroke / 2) * 2, (r - stroke / 2) * 2)
            var start = -90f
            values.forEachIndexed { i, v ->
                val sweep = (v / total * 360).toFloat()
                drawArc(
                    color(i),
                    startAngle = start + 0.75f,
                    sweepAngle = max(sweep - 1.5f, 0.5f),
                    useCenter = false,
                    topLeft = topLeft,
                    size = arc,
                    style = Stroke(width = stroke),
                    alpha = if (selected == null || selected == i) 1f else 0.4f,
                )
                start += sweep
            }
        }
    }
}
