package jp.kz2601.metamon.desktop.usage

import jp.kz2601.metamon.desktop.i18n.Strings
import jp.kz2601.metamon.services.MyUsage
import jp.kz2601.metamon.services.UsageMeter
import java.time.Instant
import java.time.ZoneId
import java.time.format.DateTimeFormatter

/**
 * 設定の「使っている量」の文(`GET /v1/me/usage`・#2812)。描かない純粋な関数だけ。
 * 文言は Android と同じキー(`voice_remaining_*`・`voice_rate_body`・`settings_usage_*`)。
 */
object UsageText {
    /** 作ったアプリの行(Android の `UsageSectionModel.Row`)。 */
    data class Meter(val title: String, val amount: String, val near: Boolean)

    /** 設定に出す分。[remaining] は「今日の残り 3」か「上限に達しました」。数が無い時は null */
    data class Summary(val pro: Boolean, val remaining: String?, val exhausted: Boolean, val apps: Meter?, val footer: String?)

    fun of(usage: MyUsage, observedAt: Instant, now: Instant = observedAt, zone: ZoneId = ZoneId.systemDefault()): Summary {
        val today = usage.remainingToday(observedAt)?.takeUnless { it.isStale(now) }
        val exhausted = today != null && today.remaining <= 0
        val remaining = when {
            today == null -> null
            exhausted -> Strings.format("voice_rate_body", resetText(today.resetAt, zone))
            else -> Strings.format("voice_remaining_today", today.remaining)
        }
        val apps = usage.apps?.let { meter(it) }
        val footer = apps?.let { Strings[if (usage.isPro) "settings_usage_footer_pro" else "settings_usage_footer_free"] }
        return Summary(usage.isPro, remaining, exhausted, apps, footer)
    }

    /** 「3 / 100」。数えない時は「3(上限なし)」 */
    fun meter(meter: UsageMeter): Meter = Meter(
        title = Strings["settings_usage_apps"],
        amount = meter.limit?.let { "${meter.used} / $it" } ?: Strings.format("settings_usage_no_limit", meter.used),
        near = meter.isNear,
    )

    /** 戻る時刻(Android の `resetText`)。分からない時は「明日」 */
    fun resetText(at: Instant?, zone: ZoneId = ZoneId.systemDefault()): String {
        at ?: return Strings["voice_tomorrow"]
        return DateTimeFormatter.ofPattern(Strings["desktop_usage_reset_pattern"], Strings.locale).withZone(zone).format(at)
    }
}
