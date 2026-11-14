package jp.kz2601.metamon.notifications

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test
import java.time.Instant

/** #1124: 設定の「次の通知」。iOS の UpcomingNotificationListTests と同じ条件。 */
class UpcomingNotificationListTest {
    private fun n(i: Int) = UpcomingNotification("t$i", "", Instant.ofEpochSecond(1_000L + i))

    @Test
    fun showsNearestFiveInOrder() {
        val list = UpcomingNotificationList.of((7 downTo 1).map(::n))
        assertEquals(listOf("t1", "t2", "t3", "t4", "t5"), list.rows.map { it.title })
        assertFalse(list.isEmpty)
    }

    @Test
    fun emptyOnlyWithoutNotificationsPlacesOrLocationNotice() {
        assertTrue(UpcomingNotificationList.of(emptyList()).isEmpty)
        assertFalse("場所を見張っている", UpcomingNotificationList.of(emptyList(), places = 2).isEmpty)
        assertFalse("位置の許可待ちの案内を出す", UpcomingNotificationList.of(emptyList(), locationOff = true).isEmpty)
    }
}
