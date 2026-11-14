package jp.kz2601.metamon.renderer.components.health

import android.Manifest
import android.content.Context
import android.content.pm.PackageManager
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.Canvas
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.LinearProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.alpha
import androidx.compose.ui.geometry.CornerRadius
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.geometry.Size
import androidx.compose.ui.graphics.PathEffect
import androidx.compose.ui.graphics.StrokeCap
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalLocale
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.semantics.testTag
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.health.connect.client.PermissionController
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.LifecycleEventObserver
import androidx.lifecycle.compose.LifecycleEventEffect
import androidx.lifecycle.compose.LocalLifecycleOwner
import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.targetNumber
import jp.kz2601.metamon.renderer.R
import jp.kz2601.metamon.renderer.components.lists.skeletonPulse
import jp.kz2601.metamon.renderer.registry.RenderContext
import jp.kz2601.metamon.renderer.theme.RenderTheme
import jp.kz2601.metamon.renderer.theme.SymbolIcon
import jp.kz2601.metamon.renderer.theme.colors
import jp.kz2601.metamon.renderer.theme.renderCard
import java.time.LocalDate
import java.time.format.DateTimeFormatter
import java.time.format.FormatStyle
import java.time.format.TextStyle as DayTextStyle
import kotlin.math.max
import kotlin.math.min

/**
 * `healthStat`(#870): 端末のヘルスのデータのカード(iOS の `HealthStatLeaf`)。entity を持たず Health Connect から読む。
 * metric steps|weight × period today|week。today は数字(steps は target で達成率のバー)、week は 7 日のグラフ。
 * Health Connect が無い・権限が無い時は理由を出す。前面に戻るたびに読み直す。
 * `live: true`(steps・#3694)は、前面(RESUMED)にいる間だけ `TYPE_STEP_COUNTER` を購読し、一発読みの今日の値に増分を足して数字を増やす。
 * 背面に回ると listener を外す。既定(live 無し)は今までどおり一発読みだけ。
 */
@Composable
fun HealthStatLeaf(c: Component, ctx: RenderContext, modifier: Modifier = Modifier) {
    val context = LocalContext.current
    val source = LocalHealthSource.current ?: remember(context) { defaultHealthSource(context) }
    val weight = HealthStatRules.isWeight(c.metric)
    val week = HealthStatRules.isWeek(c.period)
    val live = c.live == true && !weight
    var phase by remember(c) { mutableStateOf<HealthPhase>(HealthPhase.Loading) }
    // live が増分を足す元(一発読みの値)。phase は増分込みなので別に持つ(二重に数えない)
    var baseline by remember(c) { mutableStateOf<List<HealthDailyValue>?>(null) }
    var reload by remember { mutableIntStateOf(0) }

    val needed = remember(source, weight) { source.readPermissions(weight) }
    val launcher = rememberLauncherForActivityResult(PermissionController.createRequestPermissionResultContract()) { reload++ }

    // 前面に戻るたびに読み直す(Health Connect の画面で許可してから戻った時も効く)
    LifecycleEventEffect(Lifecycle.Event.ON_RESUME) { reload++ }
    LaunchedEffect(source, weight, week, reload) {
        phase = try {
            val days = HealthStatRules.days(c.metric, c.period)
            val values = if (weight) source.weights(days) else source.steps(days)
            baseline = values
            HealthPhase.Loaded(values)
        } catch (e: HealthDataException) {
            baseline = null
            HealthPhase.Failed(e.failure)
        }
    }

    if (live) {
        LiveSteps(reload, onValues = { phase = HealthPhase.Loaded(it) }, baseline = { baseline })
    }

    Column(
        modifier.fillMaxWidth().renderCard().semantics { testTag = "healthStat" },
        verticalArrangement = Arrangement.spacedBy(RenderTheme.Spacing.sm),
    ) {
        Header(c)
        when (val p = phase) {
            HealthPhase.Loading -> Value(c, 0.0, 34, loading = true)
            is HealthPhase.Failed -> Failure(p.failure, onAllow = { launcher.launch(needed) })
            is HealthPhase.Loaded -> when {
                p.values.isEmpty() && weight -> Note(stringResource(R.string.renderer_health_no_weight))
                p.values.isEmpty() -> Value(c, 0.0, 44, loading = false)
                week -> Week(c, p.values)
                else -> Today(c, p.values)
            }
        }
    }
}

/**
 * live(#3694): 前面(RESUMED)にいる間だけ歩数計を購読し、[baseline] の今日に増分を足して [onValues] で返す。
 * [reload](前面に戻った・許可した)のたびに増分の基準を取り直す。`ACTIVITY_RECOGNITION` が無ければ 1 回だけ聞き、拒まれたら一発読みのまま。
 */
@Composable
private fun LiveSteps(reload: Int, onValues: (List<HealthDailyValue>) -> Unit, baseline: () -> List<HealthDailyValue>?) {
    val context = LocalContext.current
    val counter = LocalStepCounterSource.current ?: remember(context) { defaultStepCounterSource(context) }
    var allowed by remember { mutableStateOf(context.hasActivityRecognition()) }
    val ask = rememberLauncherForActivityResult(ActivityResultContracts.RequestPermission()) { allowed = it }
    LaunchedEffect(Unit) { if (!allowed) ask.launch(Manifest.permission.ACTIVITY_RECOGNITION) }

    val lifecycle = LocalLifecycleOwner.current.lifecycle
    var resumed by remember { mutableStateOf(lifecycle.currentState.isAtLeast(Lifecycle.State.RESUMED)) }
    DisposableEffect(lifecycle) {
        val observer = LifecycleEventObserver { _, _ -> resumed = lifecycle.currentState.isAtLeast(Lifecycle.State.RESUMED) }
        lifecycle.addObserver(observer)
        onDispose { lifecycle.removeObserver(observer) }
    }

    LaunchedEffect(counter, allowed, resumed, reload) {
        if (!allowed || !resumed) return@LaunchedEffect
        val delta = LiveStepDelta()
        counter.counts().collect { raw ->
            val base = baseline() ?: return@collect
            onValues(LiveStepDelta.apply(base, delta.delta(raw)))
        }
    }
}

private fun Context.hasActivityRecognition(): Boolean =
    checkSelfPermission(Manifest.permission.ACTIVITY_RECOGNITION) == PackageManager.PERMISSION_GRANTED

private sealed interface HealthPhase {
    data object Loading : HealthPhase
    data class Failed(val failure: HealthFailure) : HealthPhase
    data class Loaded(val values: List<HealthDailyValue>) : HealthPhase
}

@Composable
private fun Header(c: Component) {
    Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(6.dp)) {
        Box(
            Modifier.size(18.dp).background(RenderTheme.colors.brandSoft, RoundedCornerShape(6.dp)),
            contentAlignment = Alignment.Center,
        ) {
            SymbolIcon(HealthStatRules.icon(c.metric), 12.dp, RenderTheme.colors.brandA)
        }
        Text(
            c.label.orEmpty(),
            style = RenderTheme.Fonts.statLabel,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
            maxLines = 1,
            overflow = TextOverflow.Ellipsis,
        )
    }
}

/** 使えない・許可されていない理由。許可はヘルスコネクトの画面で聞く。 */
@Composable
private fun Failure(failure: HealthFailure, onAllow: () -> Unit) {
    val text = stringResource(
        when (failure) {
            HealthFailure.UNAVAILABLE -> R.string.renderer_health_unavailable
            HealthFailure.UPDATE_REQUIRED -> R.string.renderer_health_update_required
            HealthFailure.DENIED -> R.string.renderer_health_denied
            HealthFailure.FAILED -> R.string.renderer_health_failed
        },
    )
    Column(verticalArrangement = Arrangement.spacedBy(RenderTheme.Spacing.xs)) {
        Row(horizontalArrangement = Arrangement.spacedBy(6.dp)) {
            SymbolIcon("heart.text.square", 16.dp, MaterialTheme.colorScheme.onSurfaceVariant)
            Text(
                text,
                Modifier.semantics { testTag = "healthStat.unavailable" },
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
        if (failure == HealthFailure.DENIED) {
            TextButton(onClick = onAllow, modifier = Modifier.semantics { testTag = "healthStat.allow" }) {
                Text(stringResource(R.string.renderer_health_allow))
            }
        }
    }
}

@Composable
private fun Note(text: String) {
    Text(
        text,
        Modifier.semantics { testTag = "healthStat.note" },
        style = MaterialTheme.typography.bodySmall,
        color = MaterialTheme.colorScheme.onSurfaceVariant,
    )
}

/** today: 大きな数字(+ steps は目標のバー・体重は今日でなければ日付)。 */
@Composable
private fun Today(c: Component, values: List<HealthDailyValue>) {
    val locale = LocalLocale.current.platformLocale
    val latest = values.last()
    Value(c, latest.value, 44, loading = false)
    if (HealthStatRules.isWeight(c.metric) && latest.day != LocalDate.now()) {
        Note(
            stringResource(
                R.string.renderer_health_recorded_on,
                latest.day.format(DateTimeFormatter.ofLocalizedDate(FormatStyle.MEDIUM).withLocale(locale)),
            ),
        )
    }
    val target = c.targetNumber
    if (!HealthStatRules.isWeight(c.metric) && target != null && target > 0) {
        val rate = HealthStatRules.progress(latest.value, target)
        LinearProgressIndicator(
            progress = { rate },
            modifier = Modifier.fillMaxWidth(),
            color = if (rate >= 1f) RenderTheme.colors.positive else RenderTheme.colors.brandA,
            trackColor = RenderTheme.colors.brandSoft,
        )
        Note(
            stringResource(
                R.string.renderer_health_target,
                HealthStatRules.stepsText(target, locale),
                HealthStatRules.achievedPercent(latest.value, target) ?: 0,
            ),
        )
    }
}

/** week: 7 日の棒(歩数)/ 折れ線(体重)と 1 行の要約。 */
@Composable
private fun Week(c: Component, values: List<HealthDailyValue>) {
    val locale = LocalLocale.current.platformLocale
    val weight = HealthStatRules.isWeight(c.metric)
    val colors = RenderTheme.colors
    val target = c.targetNumber
    Canvas(Modifier.fillMaxWidth().height(120.dp).semantics { testTag = "healthStat.week" }) {
        val hi = max(values.maxOf { it.value }, target?.takeIf { !weight } ?: 0.0)
        val lo = if (weight) min(values.minOf { it.value }, hi) else 0.0
        val span = (hi - lo).takeIf { it > 0 } ?: 1.0
        fun y(v: Double) = (size.height * (1 - (v - lo) / span)).toFloat()
        val slot = size.width / values.size
        if (weight) {
            val pts = values.mapIndexed { i, v -> Offset(slot * i + slot / 2, y(v.value)) }
            pts.zipWithNext { a, b -> drawLine(colors.brandA, a, b, 2.5.dp.toPx(), cap = StrokeCap.Round) }
            pts.forEach { drawCircle(colors.brandA, radius = 3.dp.toPx(), center = it) }
        } else {
            values.forEachIndexed { i, v ->
                val top = y(v.value)
                drawRoundRect(
                    colors.brandA,
                    topLeft = Offset(slot * i + slot * 0.2f, top),
                    size = Size(slot * 0.6f, size.height - top),
                    cornerRadius = CornerRadius(4.dp.toPx(), 4.dp.toPx()),
                )
            }
            if (target != null && target > 0) {
                drawLine(
                    colors.positive,
                    Offset(0f, y(target)),
                    Offset(size.width, y(target)),
                    1.5.dp.toPx(),
                    pathEffect = PathEffect.dashPathEffect(floatArrayOf(4.dp.toPx(), 3.dp.toPx())),
                )
            }
        }
    }
    Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.SpaceBetween) {
        values.forEach {
            Text(
                it.day.dayOfWeek.getDisplayName(DayTextStyle.NARROW, locale),
                style = RenderTheme.Fonts.legend,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
    }
    val summary = if (weight) {
        val delta = HealthStatRules.weightDelta(values) ?: 0.0
        stringResource(
            R.string.renderer_health_weight_summary,
            HealthStatRules.weightText(values.last().value, locale),
            HealthStatRules.signedWeightText(delta, locale),
        )
    } else {
        stringResource(R.string.renderer_health_average, HealthStatRules.stepsText(HealthStatRules.averageSteps(values), locale))
    }
    Note(summary)
}

/** 大きな数字と単位。[loading] の間は薄く明滅させる(iOS の redacted)。 */
@Composable
private fun Value(c: Component, v: Double, size: Int, loading: Boolean) {
    val locale = LocalLocale.current.platformLocale
    Row(
        Modifier.semantics { testTag = "healthStat.value" },
        verticalAlignment = Alignment.Bottom,
        horizontalArrangement = Arrangement.spacedBy(4.dp),
    ) {
        Text(
            HealthStatRules.valueText(c.metric, v, locale),
            if (loading) Modifier.alpha(skeletonPulse()) else Modifier,
            style = MaterialTheme.typography.displaySmall.copy(fontSize = size.sp, fontWeight = FontWeight.Bold, fontFeatureSettings = "tnum"),
            color = if (loading) RenderTheme.colors.tertiaryLabel else MaterialTheme.colorScheme.onSurface,
            maxLines = 1,
        )
        Text(
            stringResource(if (HealthStatRules.isWeight(c.metric)) R.string.renderer_health_weight_unit else R.string.renderer_health_steps_unit),
            Modifier.padding(bottom = (size / 8).dp),
            style = MaterialTheme.typography.titleMedium.copy(fontSize = max(size * 0.4f, 13f).sp, fontWeight = FontWeight.SemiBold),
            color = MaterialTheme.colorScheme.onSurfaceVariant,
            maxLines = 1,
        )
    }
}
