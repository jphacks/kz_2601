package jp.kz2601.metamon.desktop.settings

import java.time.Instant
import java.time.ZoneId
import java.time.format.DateTimeFormatter

/** 予約中の通知 1 件(Android の `UpcomingNotification` に、取り消す id とアプリ名を足した物)。 */
data class UpcomingNotification(
    val id: String,
    val title: String,
    val body: String,
    /** どのアプリが予約したか。分からなければ null。 */
    val appName: String?,
    val at: Instant,
)

/**
 * 設定・トレイの「次の通知」。並びと件数を Android の `UpcomingNotificationList` に揃える
 * (近い順・先頭から [LIMIT] 件・同じ時刻は id 順)。
 */
data class UpcomingNotifications(
    val rows: List<UpcomingNotification>,
    /** [LIMIT] を超えて出さなかった件数。 */
    val hidden: Int,
    val isEmpty: Boolean,
) {
    companion object {
        const val LIMIT = 5

        val EMPTY = of(emptyList())

        fun of(upcoming: List<UpcomingNotification>, limit: Int = LIMIT): UpcomingNotifications {
            val sorted = upcoming.sortedWith(compareBy({ it.at }, { it.id }))
            return UpcomingNotifications(sorted.take(limit), (sorted.size - limit).coerceAtLeast(0), sorted.isEmpty())
        }

        /** 「11/13 14:05 タイトル(アプリ名)」。アプリ名が無ければ付けない。 */
        fun label(n: UpcomingNotification, zone: ZoneId = ZoneId.systemDefault()): String {
            val time = DateTimeFormatter.ofPattern("M/d HH:mm").format(n.at.atZone(zone))
            val app = n.appName?.takeIf { it.isNotBlank() }?.let { " ($it)" }.orEmpty()
            return "$time ${n.title}$app"
        }
    }
}
