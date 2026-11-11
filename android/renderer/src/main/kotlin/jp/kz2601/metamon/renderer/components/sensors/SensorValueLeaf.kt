package jp.kz2601.metamon.renderer.components.sensors

import androidx.compose.foundation.Canvas
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.aspectRatio
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.LinearProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.graphics.drawscope.Stroke
import androidx.compose.ui.hapticfeedback.HapticFeedbackType
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalHapticFeedback
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.semantics.testTag
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.LifecycleEventObserver
import androidx.lifecycle.compose.LocalLifecycleOwner
import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.ComponentStyle
import jp.kz2601.metamon.dsl.expr.DSLClock
import jp.kz2601.metamon.renderer.ActionContext
import jp.kz2601.metamon.renderer.R
import jp.kz2601.metamon.renderer.components.UnavailableComponent
import jp.kz2601.metamon.renderer.components.actions.ActionButton
import jp.kz2601.metamon.renderer.components.actions.ActionButtonKind
import jp.kz2601.metamon.renderer.registry.RenderContext
import jp.kz2601.metamon.renderer.theme.RenderTheme
import jp.kz2601.metamon.renderer.theme.SymbolIcon
import jp.kz2601.metamon.renderer.theme.colors
import jp.kz2601.metamon.renderer.theme.renderCard
import kotlinx.coroutines.launch
import kotlin.math.min

/**
 * `sensorValue`(#871): 端末のセンサーの今の値(iOS の SensorValueLeaf)。画面が前面(STARTED 以上)の間だけ購読し、背面では止める。
 * style number(大きな数字)/ gauge(min〜max)/ level(水平器の泡・tilt)。
 * entity + field があれば「今の値を記録」、seconds で「記録を開始/停止」、threshold で警告 + 振動(任意で保存・action)。
 */
@Composable
fun SensorValueLeaf(c: Component, ctx: RenderContext, modifier: Modifier = Modifier) {
    val kind = SensorKind.of(c.sensor)
    if (kind == null) {
        UnavailableComponent(c, modifier)
        return
    }
    val context = LocalContext.current
    val source = LocalSensorSource.current ?: remember(context) { AndroidSensorSource(context) }
    val availability = remember(source, kind) { source.availability(kind) }
    val canSave = c.entity != null && c.field != null
    val pipeline = remember(c.threshold, canSave) { SensorPipeline(c.threshold, canSave) }
    var value by remember { mutableStateOf<Double?>(null) }
    var motion by remember { mutableStateOf<MotionSample?>(null) }
    var alerting by remember { mutableStateOf(false) }
    var logging by remember { mutableStateOf(false) }
    var savedCount by remember { mutableIntStateOf(0) }
    var note by remember { mutableStateOf<String?>(null) }
    var tick by remember { mutableIntStateOf(0) } // 一定間隔の記録の件数を描き直す
    val haptics = LocalHapticFeedback.current
    val scope = rememberCoroutineScope()
    val saveFailed = stringResource(R.string.renderer_sensor_save_failed)
    val finishedText = stringResource(R.string.renderer_sensor_log_finished)

    suspend fun save(v: Double) {
        val entity = c.entity ?: return
        val data = SensorRules.record(kind, v, c, ctx.runtime.entityModel(entity), DSLClock.now()) ?: return
        if (ctx.runtime.addRecord(entity, data)) savedCount += 1 else note = saveFailed
    }

    // 前面にいる間だけ購読する(背面へ回ると収集をやめ、SensorManager の listener も外れる)
    val lifecycle = LocalLifecycleOwner.current.lifecycle
    var foreground by remember { mutableStateOf(lifecycle.currentState.isAtLeast(Lifecycle.State.STARTED)) }
    DisposableEffect(lifecycle) {
        val observer = LifecycleEventObserver { _, _ -> foreground = lifecycle.currentState.isAtLeast(Lifecycle.State.STARTED) }
        lifecycle.addObserver(observer)
        onDispose {
            lifecycle.removeObserver(observer)
            pipeline.stopLog() // 画面を離れたら一定間隔の記録も止める
        }
    }
    LaunchedEffect(source, kind, foreground, availability) {
        if (!foreground || availability != SensorAvailability.AVAILABLE) return@LaunchedEffect
        source.frames(kind).collect { f ->
            val events = pipeline.ingest(f.value, System.currentTimeMillis()) ?: return@collect
            value = f.value
            motion = f.motion
            alerting = pipeline.isAlerting
            for (e in events) {
                when (e) {
                    is SensorPipeline.Event.Fired -> {
                        haptics.performHapticFeedback(HapticFeedbackType.LongPress)
                        c.threshold?.action?.let { ctx.dispatch(it, ActionContext(ctx.route, ctx.form)) }
                    }
                    is SensorPipeline.Event.Save -> launch { save(e.value) }
                    is SensorPipeline.Event.LogFinished -> note = String.format(finishedText, e.count)
                }
            }
            logging = pipeline.isLogging
            tick++
        }
    }

    Column(
        modifier.fillMaxWidth().renderCard().semantics { testTag = "sensorValue" },
        verticalArrangement = Arrangement.spacedBy(RenderTheme.Spacing.sm),
    ) {
        Header(kind, c.label)
        when (availability) {
            SensorAvailability.UNAVAILABLE -> Text(
                stringResource(R.string.renderer_sensor_unavailable),
                Modifier.semantics { testTag = "sensorValue.unavailable" },
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            SensorAvailability.AVAILABLE -> {
                val v = value
                when {
                    v == null -> Number(kind, null, 34)
                    c.style == ComponentStyle.GAUGE -> Gauge(kind, v, c, alerting)
                    c.style == ComponentStyle.LEVEL && kind == SensorKind.TILT -> Level(v, motion, alerting)
                    else -> Number(kind, v, 44)
                }
            }
        }
        if (alerting) {
            Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(6.dp)) {
                SymbolIcon("exclamationmark.triangle", 16.dp, RenderTheme.colors.negative)
                Text(
                    c.threshold?.message ?: stringResource(R.string.renderer_sensor_alert),
                    Modifier.semantics { testTag = "sensorValue.alert" },
                    style = MaterialTheme.typography.bodyMedium.copy(fontWeight = FontWeight.SemiBold),
                    color = RenderTheme.colors.negative,
                )
            }
        }
        if (canSave && availability == SensorAvailability.AVAILABLE) {
            Row(horizontalArrangement = Arrangement.spacedBy(RenderTheme.Spacing.sm)) {
                ActionButton(
                    stringResource(R.string.renderer_sensor_save),
                    ActionButtonKind.SECONDARY,
                    onClick = { value?.let { v -> scope.launch { save(v) } } },
                    modifier = Modifier.weight(1f).semantics { testTag = "sensorValue.save" },
                    enabled = value != null,
                )
                val seconds = c.seconds
                if (seconds != null && seconds > 0) {
                    if (logging) {
                        ActionButton(
                            stringResource(R.string.renderer_sensor_stop),
                            ActionButtonKind.DESTRUCTIVE,
                            onClick = { pipeline.stopLog(); logging = false },
                            modifier = Modifier.weight(1f).semantics { testTag = "sensorValue.stop" },
                        )
                    } else {
                        ActionButton(
                            stringResource(R.string.renderer_sensor_start),
                            ActionButtonKind.PRIMARY,
                            onClick = {
                                note = null
                                pipeline.startLog(seconds, c.limit)
                                logging = true
                            },
                            modifier = Modifier.weight(1f).semantics { testTag = "sensorValue.start" },
                        )
                    }
                }
            }
            val log = pipeline.log
            val status = when {
                log != null -> {
                    tick.let { } // 件数が変わったら読み直す
                    stringResource(R.string.renderer_sensor_logging, log.saved, log.limit, log.intervalSeconds)
                }
                note != null -> note
                savedCount > 0 -> stringResource(R.string.renderer_sensor_saved, savedCount)
                else -> null
            }
            status?.let {
                Text(it, style = MaterialTheme.typography.labelSmall.copy(fontFeatureSettings = "tnum"), color = MaterialTheme.colorScheme.onSurfaceVariant)
            }
        }
    }
}

@Composable
private fun Header(kind: SensorKind, label: String?) {
    Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(6.dp)) {
        Box(
            Modifier.size(18.dp).background(RenderTheme.colors.brandSoft, RoundedCornerShape(6.dp)),
            contentAlignment = Alignment.Center,
        ) {
            SymbolIcon(SensorRules.icon(kind), 12.dp, RenderTheme.colors.brandA)
        }
        Text(
            label.orEmpty(),
            style = MaterialTheme.typography.labelMedium,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
            maxLines = 1,
            overflow = TextOverflow.Ellipsis,
        )
    }
}

/** 大きな数字 + 単位。値がまだ無い時は薄い「0」(iOS の redacted)。 */
@Composable
private fun Number(kind: SensorKind, v: Double?, size: Int) {
    Row(
        Modifier.semantics { testTag = "sensorValue.value" },
        verticalAlignment = Alignment.Bottom,
        horizontalArrangement = Arrangement.spacedBy(4.dp),
    ) {
        val faint = v == null
        Text(
            kind.format(v ?: 0.0),
            style = MaterialTheme.typography.displaySmall.copy(fontSize = size.sp, fontWeight = FontWeight.Bold, fontFeatureSettings = "tnum"),
            color = if (faint) MaterialTheme.colorScheme.onSurface.copy(alpha = 0.2f) else MaterialTheme.colorScheme.onSurface,
            maxLines = 1,
        )
        Text(
            kind.unit,
            Modifier.padding(bottom = (size / 8).dp),
            style = MaterialTheme.typography.titleMedium.copy(fontSize = maxOf(size * 0.4f, 13f).sp, fontWeight = FontWeight.SemiBold),
            color = MaterialTheme.colorScheme.onSurfaceVariant,
            maxLines = 1,
        )
    }
}

@Composable
private fun Gauge(kind: SensorKind, v: Double, c: Component, alerting: Boolean) {
    val range = SensorRules.range(kind, c.min, c.max)
    val span = range.endInclusive - range.start
    val fraction = ((v.coerceIn(range.start, range.endInclusive) - range.start) / span).toFloat()
    Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
        Number(kind, v, 34)
        LinearProgressIndicator(
            progress = { fraction },
            modifier = Modifier.fillMaxWidth(),
            color = if (alerting) RenderTheme.colors.negative else RenderTheme.colors.brandA,
            trackColor = RenderTheme.colors.brandSoft,
        )
        Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.SpaceBetween) {
            Text(kind.format(range.start), style = MaterialTheme.typography.labelSmall)
            Text(kind.format(range.endInclusive), style = MaterialTheme.typography.labelSmall)
        }
    }
}

/** 水平器: 円の中の泡(高い方へ動く)と傾きの角度。1° 未満は水平として緑。 */
@Composable
private fun Level(tilt: Double, motion: MotionSample?, alerting: Boolean) {
    val (bx, by) = SensorMath.bubble(motion?.gravity ?: Vector3(0.0, 0.0, -1.0))
    val flat = tilt < 1
    val colors = RenderTheme.colors
    val bubble = when {
        flat -> colors.positive
        alerting -> colors.negative
        else -> colors.brandA
    }
    Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(RenderTheme.Spacing.md)) {
        Canvas(Modifier.widthIn(min = 60.dp, max = 140.dp).weight(1f, fill = false).aspectRatio(1f)) {
            val d = min(size.width, size.height)
            val r = d / 2 - d * 0.11f
            val center = Offset(size.width / 2, size.height / 2)
            drawCircle(colors.hairline, radius = d / 2 - 1.dp.toPx(), center = center, style = Stroke(2.dp.toPx()))
            drawCircle(colors.hairline, radius = d * 0.15f, center = center, style = Stroke(1.dp.toPx()))
            drawCircle(bubble, radius = d * 0.11f, center = Offset(center.x + bx.toFloat() * r, center.y + by.toFloat() * r))
        }
        Column(verticalArrangement = Arrangement.spacedBy(2.dp)) {
            Number(SensorKind.TILT, tilt, 40)
            Text(
                stringResource(if (flat) R.string.renderer_sensor_level else R.string.renderer_sensor_tilted),
                style = MaterialTheme.typography.labelSmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
    }
}
