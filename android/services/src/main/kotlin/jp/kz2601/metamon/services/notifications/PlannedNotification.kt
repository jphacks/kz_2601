package jp.kz2601.metamon.services.notifications

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable
import java.time.DayOfWeek
import java.time.Instant
import java.time.LocalDate
import java.time.LocalTime
import java.time.YearMonth
import java.time.ZoneId
import java.time.ZonedDateTime

/**
 * 予約する通知 1 件(iOS の `PlannedNotification`)。アプリごとに永続化し、全アプリ分から近い順に上限内で予約し直す。
 * 曜日は iOS と同じ 1 = 日 〜 7 = 土。
 */
@Serializable
data class PlannedNotification(
    val identifier: String,
    val title: String,
    val body: String,
    val trigger: Trigger,
    /** 止めるまで鳴るアラームで届ける。null = 普通の通知。 */
    val alarm: Boolean? = null,
    /** 場所の通知: レコードを保存した時刻(epoch 秒。現在地が分からない時、新しい順に選ぶ)。 */
    val savedAt: Long? = null,
) {
    @Serializable
    sealed class Trigger {
        @Serializable @SerialName("at")
        data class At(val epochSecond: Long) : Trigger() {
            val instant: Instant get() = Instant.ofEpochSecond(epochSecond)
        }

        @Serializable @SerialName("daily")
        data class Daily(val hour: Int, val minute: Int) : Trigger()

        @Serializable @SerialName("weekly")
        data class Weekly(val weekday: Int, val hour: Int, val minute: Int) : Trigger()

        /** 毎月その日(日が毎月同じ時だけ)。 */
        @Serializable @SerialName("monthly")
        data class Monthly(val day: Int, val hour: Int, val minute: Int) : Trigger()

        /** 毎年その月日(2/29 などは使わない)。 */
        @Serializable @SerialName("yearly")
        data class Yearly(val month: Int, val day: Int, val hour: Int, val minute: Int) : Trigger()

        /** 場所に着いた時。repeats = 着くたび、false = 最初の 1 回だけ。 */
        @Serializable @SerialName("region")
        data class Region(val latitude: Double, val longitude: Double, val radius: Double, val repeats: Boolean) : Trigger()

        companion object {
            fun at(instant: Instant): At = At(instant.epochSecond)
        }
    }

    val isAlarm: Boolean get() = alarm == true
    val isRegion: Boolean get() = trigger is Trigger.Region

    /** [now] より後の次の発火時刻。過ぎた 1 回きり・場所の通知は null。 */
    fun nextFire(now: Instant, zone: ZoneId): Instant? = when (val t = trigger) {
        is Trigger.At -> t.instant.takeIf { it > now }
        is Trigger.Daily -> nextDay(now, zone, t.hour, t.minute, days = 3) { true }
        is Trigger.Weekly -> nextDay(now, zone, t.hour, t.minute, days = 9) { iosWeekday(it.dayOfWeek) == t.weekday }
        is Trigger.Monthly -> {
            val start = YearMonth.from(ZonedDateTime.ofInstant(now, zone))
            (0L..60L).asSequence().map { start.plusMonths(it) }
                .filter { t.day in 1..it.lengthOfMonth() }
                .map { at(it.atDay(t.day), t.hour, t.minute, zone) }
                .firstOrNull { it > now }
        }
        is Trigger.Yearly -> {
            val start = ZonedDateTime.ofInstant(now, zone).year
            (0..9).asSequence().map { start + it }
                .filter { t.month in 1..12 && t.day in 1..YearMonth.of(it, t.month).lengthOfMonth() }
                .map { at(LocalDate.of(it, t.month, t.day), t.hour, t.minute, zone) }
                .firstOrNull { it > now }
        }
        is Trigger.Region -> null
    }

    companion object {
        /** 1 = 日 〜 7 = 土(iOS の Calendar の weekday)。 */
        fun iosWeekday(d: DayOfWeek): Int = d.value % 7 + 1

        private fun at(day: LocalDate, hour: Int, minute: Int, zone: ZoneId): Instant =
            ZonedDateTime.of(day, LocalTime.of(hour, minute), zone).toInstant()

        private inline fun nextDay(now: Instant, zone: ZoneId, hour: Int, minute: Int, days: Long, match: (LocalDate) -> Boolean): Instant? {
            if (hour !in 0..23 || minute !in 0..59) return null
            val today = ZonedDateTime.ofInstant(now, zone).toLocalDate()
            for (i in 0..days) {
                val d = today.plusDays(i)
                if (!match(d)) continue
                val fire = at(d, hour, minute, zone)
                if (fire > now) return fire
            }
            return null
        }
    }
}
