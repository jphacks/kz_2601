package jp.kz2601.metamon.renderer.components.lists

import androidx.compose.animation.core.animateFloatAsState
import androidx.compose.animation.core.snap
import androidx.compose.animation.core.tween
import androidx.compose.foundation.Canvas
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.geometry.Size
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.StrokeCap
import androidx.compose.ui.graphics.drawscope.Stroke
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.clearAndSetSemantics
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.stateDescription
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.records.Aggregator
import jp.kz2601.metamon.dsl.records.FieldCoding
import jp.kz2601.metamon.renderer.R
import jp.kz2601.metamon.renderer.registry.RenderContext
import jp.kz2601.metamon.renderer.theme.LocalReduceMotion
import jp.kz2601.metamon.renderer.theme.RenderTheme
import jp.kz2601.metamon.renderer.theme.colors
import jp.kz2601.metamon.renderer.theme.renderCard
import kotlin.math.abs

/**
 * `progressBar`: 目標に対する進捗バー。`value`(0..1)のリテラル、または entity の集計 ÷ `target`。iOS の `ProgressBarLeaf`。
 * 色は tone の段階(#140)。目標の式が空・0 以下の間は「—」(0% と区別)。読み上げは「ラベル, 42%」。
 */
@Composable
fun ProgressBarLeaf(c: Component, ctx: RenderContext, modifier: Modifier = Modifier) {
    val data = rememberLeafData(ctx.runtime)
    val raw = ProgressValue.ratio(c, data)
    val tone = c.tone?.wire
    val percent = if (ProgressValue.missingTarget(c, data)) FieldCoding.UNAVAILABLE else ProgressValue.percentText(raw, tone)
    val colors = RenderTheme.colors
    val level = ProgressValue.level(raw, tone)
    val textColor = when (level) {
        ProgressValue.Level.NORMAL -> colors.brandText
        ProgressValue.Level.SUCCESS -> colors.positive
        ProgressValue.Level.OVER -> colors.negative
    }
    val fill = when (level) {
        ProgressValue.Level.NORMAL -> Brush.horizontalGradient(listOf(colors.brandFillStart, colors.brandFillEnd))
        else -> Brush.horizontalGradient(listOf(textColor, textColor))
    }
    val ratio by animateFloatAsState(
        raw.coerceIn(0.0, 1.0).toFloat(),
        if (LocalReduceMotion.current) snap() else tween(300),
        label = "progress",
    )
    val progressLabel = c.label ?: stringResource(R.string.lists_progress)
    Column(
        modifier.renderCard().clearAndSetSemantics {
            contentDescription = progressLabel
            stateDescription = percent
        },
        verticalArrangement = Arrangement.spacedBy(6.dp),
    ) {
        c.label?.let {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Text(it, Modifier.weight(1f), style = RenderTheme.Fonts.statLabel, color = MaterialTheme.colorScheme.onSurfaceVariant, maxLines = 1)
                Text(percent, style = MaterialTheme.typography.bodyMedium.copy(fontWeight = FontWeight.SemiBold, fontFeatureSettings = "tnum"), color = textColor)
            }
        }
        Box(Modifier.fillMaxWidth().height(10.dp).background(colors.surfaceElevated, CircleShape)) {
            Box(Modifier.fillMaxHeight().fillMaxWidth(ratio).background(fill, CircleShape))
        }
    }
}

/** `gauge`: [min, max] の中の値を円弧のゲージで。`value` のリテラル、または entity の集計。iOS の `GaugeLeaf`。 */
@Composable
fun GaugeLeaf(c: Component, ctx: RenderContext, modifier: Modifier = Modifier) {
    val data = rememberLeafData(ctx.runtime)
    val lo = c.min ?: 0.0
    val hi = (c.max ?: 1.0).let { if (it > lo) it else lo + 1 }
    val (value, unit) = GaugeValue.value(c, data)
    val clamped = value.coerceIn(lo, hi)
    val valueText = Aggregator.format(value, null, null)
    val title = c.label?.let { l -> unit?.let { "$l ($it)" } ?: l }
    val gaugeLabel = title ?: stringResource(R.string.lists_gauge)
    val gaugeState = stringResource(R.string.lists_gauge_range, valueText, compact(lo), compact(hi))
    val colors = RenderTheme.colors
    Column(
        modifier.renderCard().clearAndSetSemantics {
            // 「体重 (kg), 62.5(範囲 40〜120)」と 1 要素で読む
            contentDescription = gaugeLabel
            stateDescription = gaugeState
        },
        horizontalAlignment = Alignment.CenterHorizontally,
        verticalArrangement = Arrangement.spacedBy(8.dp),
    ) {
        Box(Modifier.size(112.dp), contentAlignment = Alignment.Center) {
            Canvas(Modifier.size(104.dp)) {
                val stroke = 10.dp.toPx()
                val arc = Size(size.width - stroke, size.height - stroke)
                val tl = Offset(stroke / 2, stroke / 2)
                // 下を開けた 270° の円弧(iOS の accessoryCircular に近い形)
                drawArc(colors.surfaceElevated, 135f, 270f, false, tl, arc, style = Stroke(stroke, cap = StrokeCap.Round))
                val sweep = ((clamped - lo) / (hi - lo) * 270).toFloat()
                drawArc(
                    Brush.sweepGradient(listOf(colors.brandA, colors.brandB, colors.brandA)),
                    135f, sweep, false, tl, arc, style = Stroke(stroke, cap = StrokeCap.Round),
                )
            }
            Column(horizontalAlignment = Alignment.CenterHorizontally) {
                Text(valueText, style = MaterialTheme.typography.titleMedium.copy(fontWeight = FontWeight.SemiBold, fontFeatureSettings = "tnum"))
                Row(horizontalArrangement = Arrangement.spacedBy(18.dp)) {
                    Text(compact(lo), style = MaterialTheme.typography.labelSmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
                    Text(compact(hi), style = MaterialTheme.typography.labelSmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
                }
            }
        }
        title?.let { Text(it, style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.onSurfaceVariant, maxLines = 1) }
    }
}

/** 端の目盛り: 大きい値は 30K のように短く。 */
internal fun compact(v: Double): String {
    if (abs(v) < 10_000) return Aggregator.format(v, null, null)
    val (n, suffix) = when {
        abs(v) >= 1e9 -> v / 1e9 to "B"
        abs(v) >= 1e6 -> v / 1e6 to "M"
        else -> v / 1e3 to "K"
    }
    return ChartReadout.format(n, java.util.Locale.ROOT).replace(",", "") + suffix
}
