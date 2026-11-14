package jp.kz2601.metamon.renderer.components.health

import jp.kz2601.metamon.dsl.Metric
import jp.kz2601.metamon.dsl.Period
import java.time.LocalDate
import java.util.Locale
import kotlin.math.roundToInt

/**
 * `healthStat`(#870)の計算と書式。iOS の `HealthStatLeaf` の数の出し方の写し(Compose に依らない・JVM テストする)。
 * 歩数は桁区切りの整数、体重は小数 1 桁。week は 7 日、today は歩数 1 日・体重 30 日まで遡って最新を出す。
 */
object HealthStatRules {
    /** week の日数(iOS と同じ)。 */
    const val WEEK_DAYS = 7

    /** 体重の today が遡る日数(最新の記録を探す)。 */
    const val WEIGHT_LATEST_DAYS = 30

    fun isWeight(metric: Metric?): Boolean = metric == Metric.WEIGHT

    fun isWeek(period: Period?): Boolean = period == Period.WEEK

    /** 読む日数。week は 7 日、today は歩数 1 日・体重 30 日。 */
    fun days(metric: Metric?, period: Period?): Int = when {
        isWeek(period) -> WEEK_DAYS
        isWeight(metric) -> WEIGHT_LATEST_DAYS
        else -> 1
    }

    /** 見出しの印(docs/dsl/icon-map.json にある名前)。 */
    fun icon(metric: Metric?): String = if (isWeight(metric)) "scalemass" else "figure.walk"

    /** 今日を最後とする直近 [days] 日(古い順)。iOS の `healthDayStarts`。 */
    fun dayStarts(days: Int, today: LocalDate): List<LocalDate> =
        (maxOf(days, 1) - 1 downTo 0).map { today.minusDays(it.toLong()) }

    /** 歩数は桁区切りの整数(iOS の `stepsText`)。 */
    fun stepsText(v: Double, locale: Locale = Locale.getDefault()): String =
        String.format(locale, "%,d", v.roundToInt().coerceAtLeast(0))

    /** 体重は小数 1 桁。 */
    fun weightText(v: Double, locale: Locale = Locale.getDefault()): String = String.format(locale, "%.1f", v)

    fun valueText(metric: Metric?, v: Double, locale: Locale = Locale.getDefault()): String =
        if (isWeight(metric)) weightText(v, locale) else stepsText(v, locale)

    /** 目標に対する達成率(%)。0 以下の目標は null。iOS と同じく 0〜100 に丸めない値も出す。 */
    fun achievedPercent(latest: Double, target: Double?): Int? {
        if (target == null || target <= 0) return null
        return ((latest / target) * 100).roundToInt()
    }

    /** 目標のバーの割合(0〜1)。 */
    fun progress(latest: Double, target: Double?): Float {
        if (target == null || target <= 0) return 0f
        return (latest / target).coerceIn(0.0, 1.0).toFloat()
    }

    /** week の歩数の 1 日平均。 */
    fun averageSteps(values: List<HealthDailyValue>): Double =
        if (values.isEmpty()) 0.0 else values.sumOf { it.value } / values.size

    /** week の体重の増減(最新 - いちばん古い)。記録が 1 件しか無ければ null。 */
    fun weightDelta(values: List<HealthDailyValue>): Double? {
        if (values.size < 2) return null
        return values.last().value - values.first().value
    }

    /** 増減の符号つきの文字列(+0.3 / -0.3)。 */
    fun signedWeightText(delta: Double, locale: Locale = Locale.getDefault()): String = String.format(locale, "%+.1f", delta)
}
