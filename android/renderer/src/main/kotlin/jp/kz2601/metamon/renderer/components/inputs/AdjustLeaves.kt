package jp.kz2601.metamon.renderer.components.inputs

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Slider
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.expr.DSLClock
import jp.kz2601.metamon.dsl.records.FieldCoding
import jp.kz2601.metamon.dsl.records.FieldType
import jp.kz2601.metamon.dsl.records.JSONValue
import jp.kz2601.metamon.renderer.form.ColorHex
import jp.kz2601.metamon.renderer.form.NumericInput
import jp.kz2601.metamon.renderer.form.color
import jp.kz2601.metamon.renderer.form.double
import jp.kz2601.metamon.renderer.form.setColor
import jp.kz2601.metamon.renderer.form.setDouble
import jp.kz2601.metamon.renderer.registry.RenderContext
import jp.kz2601.metamon.renderer.theme.RenderTheme
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.ui.graphics.Color

// 数の調整(slider / stepper)と色・星(colorPicker / rating)。min・max・step は NumericInput で丸める。

/** `slider`: min..max(既定 0..100)。integer 系は step 既定 1、number は連続。 */
@Composable
fun SliderLeaf(c: Component, ctx: RenderContext, modifier: Modifier = Modifier) = FormInput(c, ctx, modifier) { form, field ->
    val bounds = NumericInput.range(c.min, c.max, 0.0, 100.0)
    val def = form.entity.field(field)
    val step = NumericInput.step(c.step) ?: (if (def?.type == FieldType.NUMBER) null else 1.0)
    val value = form.double(field, bounds.start)
    LabeledInput(c.label) {
        Row(Modifier.fillMaxWidth().inputChrome(), verticalAlignment = Alignment.CenterVertically) {
            Slider(
                value = value.coerceIn(bounds.start, bounds.endInclusive).toFloat(),
                onValueChange = { form.setDouble(field, NumericInput.snap(it.toDouble(), bounds, step)) },
                valueRange = bounds.start.toFloat()..bounds.endInclusive.toFloat(),
                steps = NumericInput.sliderSteps(bounds, step),
                modifier = Modifier.weight(1f),
            )
            Text(
                numberText(value, def),
                fontWeight = FontWeight.SemiBold,
                color = MaterialTheme.colorScheme.primary,
                textAlign = TextAlign.End,
                modifier = Modifier.widthIn(min = 40.dp),
            )
        }
    }
}

/** `stepper`: -/+ で増減。min / max 省略時は実質無制限、step 既定 1。 */
@Composable
fun StepperLeaf(c: Component, ctx: RenderContext, modifier: Modifier = Modifier) = FormInput(c, ctx, modifier) { form, field ->
    val bounds = NumericInput.range(c.min, c.max, -1_000_000_000.0, 1_000_000_000.0)
    val def = form.entity.field(field)
    val value = form.double(field, maxOf(0.0, bounds.start))
    val step = NumericInput.step(c.step) ?: 1.0
    InputRow(InputRules.label(c, form).orEmpty()) {
        OutlinedButton(onClick = { form.setDouble(field, (value - step).coerceIn(bounds.start, bounds.endInclusive)) }, enabled = value > bounds.start) {
            Text("−")
        }
        Text(numberText(value, def), fontWeight = FontWeight.SemiBold, color = MaterialTheme.colorScheme.primary)
        OutlinedButton(onClick = { form.setDouble(field, (value + step).coerceIn(bounds.start, bounds.endInclusive)) }, enabled = value < bounds.endInclusive) {
            Text("+")
        }
    }
}

/** `rating`: 星 1〜max(既定 5)。同じ星をもう一度押すと 0(未評価)。 */
@Composable
fun RatingLeaf(c: Component, ctx: RenderContext, modifier: Modifier = Modifier) = FormInput(c, ctx, modifier) { form, field ->
    val count = NumericInput.ratingCount(c.max)
    val current = NumericInput.ratingValue(form.double(field, 0.0), count)
    LabeledInput(c.label) {
        Row(Modifier.fillMaxWidth().inputChrome(), verticalAlignment = Alignment.CenterVertically) {
            for (i in 1..count) {
                TextButton(
                    onClick = { form.setDouble(field, if (i == current) 0.0 else i.toDouble()) },
                    modifier = Modifier.semantics { contentDescription = "$i" },
                ) {
                    Text(
                        if (i <= current) "★" else "☆",
                        style = MaterialTheme.typography.titleLarge,
                        color = if (i <= current) MaterialTheme.colorScheme.primary else MaterialTheme.colorScheme.outline,
                    )
                }
            }
        }
    }
}

/** `colorPicker`: 自前の色の一覧から選ぶ。string / text に "#RRGGBB" で保存。 */
@OptIn(ExperimentalLayoutApi::class)
@Composable
fun ColorPickerLeaf(c: Component, ctx: RenderContext, modifier: Modifier = Modifier) = FormInput(c, ctx, modifier) { form, field ->
    val current = form.color(field)
    LabeledInput(InputRules.label(c, form)) {
        FlowRow(
            Modifier.fillMaxWidth().inputChrome(),
            horizontalArrangement = Arrangement.spacedBy(RenderTheme.Spacing.sm),
            verticalArrangement = Arrangement.spacedBy(RenderTheme.Spacing.sm),
        ) {
            ColorHex.palette.forEach { rgb ->
                val selected = rgb == current
                Box(
                    Modifier
                        .size(32.dp)
                        .background(Color(0xFF000000 or rgb.toLong()), CircleShape)
                        .border(if (selected) 3.dp else 0.dp, MaterialTheme.colorScheme.onSurface, CircleShape)
                        .clickable(onClickLabel = ColorHex.format(rgb)) { form.setColor(field, rgb) },
                )
            }
        }
    }
}

private fun numberText(value: Double, field: jp.kz2601.metamon.dsl.records.Field?): String =
    FieldCoding.display(JSONValue.Num(value), field, now = DSLClock.now())
