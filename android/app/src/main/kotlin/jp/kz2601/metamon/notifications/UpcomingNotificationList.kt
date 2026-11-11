package jp.kz2601.metamon.notifications

import java.time.Instant

/** 予約中の通知 1 件(iOS の `UpcomingNotification`)。 */
data class UpcomingNotification(val title: String, val body: String, val at: Instant)

/**
 * 設定の「次の通知」(#1124・iOS の `UpcomingNotificationList`)。出す件数と「予約中の通知はありません」を出す条件を iOS と揃える。
 * [places] は見張っている場所の数、[locationOff] は場所の通知が位置の許可待ちで届かない時。
 */
data class UpcomingNotificationList(
    val rows: List<UpcomingNotification>,
    val places: Int,
    val locationOff: Boolean,
    /** 予約も見張っている場所も無い(位置の許可待ちの時はその案内を出すので出さない) */
    val isEmpty: Boolean,
) {
    companion object {
        /** 先頭から何件まで出すか(iOS と同じ) */
        const val LIMIT = 5

        val EMPTY = of(emptyList())

        fun of(upcoming: List<UpcomingNotification>, places: Int = 0, locationOff: Boolean = false) = UpcomingNotificationList(
            rows = upcoming.sortedBy { it.at }.take(LIMIT),
            places = places,
            locationOff = locationOff,
            isEmpty = upcoming.isEmpty() && places == 0 && !locationOff,
        )
    }
}
