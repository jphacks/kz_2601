package jp.kz2601.metamon.notifications

import jp.kz2601.metamon.dsl.Activity
import jp.kz2601.metamon.notifications.ActivityContent.Clock
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

class ActivityNotificationPlannerTest {
    private val now = 1_000_000L
    private val minute = 60_000L

    private fun content(spec: Activity, stat: ActivityStatReading? = null, appId: String = "a1") =
        ActivityNotificationPlanner.content(spec, appId, "読書記録", stat, now)

    @Test
    fun minutesがあれば残り時間_無ければ経過時間() {
        val countdown = content(Activity(title = "集中", minutes = 25))
        assertEquals(Clock.Countdown(now + 25 * minute), countdown.clock)
        assertEquals(now + 25 * minute, countdown.timeoutAtMillis)

        val elapsed = content(Activity(title = "読書中"))
        assertEquals(Clock.Elapsed(now), elapsed.clock)
        // iOS と同じく 8 時間で終える
        assertEquals(now + 480 * minute, elapsed.timeoutAtMillis)

        assertEquals(Clock.Elapsed(now), content(Activity(title = "t", minutes = 0)).clock)
    }

    @Test
    fun minutesは8時間で頭打ち() {
        assertEquals(Clock.Countdown(now + 480 * minute), content(Activity(title = "t", minutes = 9999)).clock)
    }

    @Test
    fun DSLの中身をiOSと同じ項目で持つ() {
        val c = content(
            Activity(title = "読書中", subtitle = "1 章", icon = "book", stat = "pages"),
            ActivityStatReading(label = "今日", value = "42 ページ", progress = 1.4),
        )
        assertEquals("a1", c.appId)
        assertEquals("読書記録", c.appName)
        assertEquals("読書中", c.title)
        assertEquals("1 章", c.subtitle)
        assertEquals("今日 42 ページ", c.statLine)
        assertEquals("1 章\n今日 42 ページ", c.body)
        // 超過は 100 で頭打ち
        assertEquals(100, c.progressPercent)
    }

    @Test
    fun 空のsubtitleとstat無しは本文を出さない() {
        val c = content(Activity(title = "t", subtitle = "  "))
        assertNull(c.subtitle)
        assertNull(c.statLine)
        assertNull(c.body)
        assertNull(c.progressPercent)
    }

    @Test
    fun 同じアプリの表示が出ていれば差し替える_無ければ新しく出す() {
        val c = content(Activity(title = "t"))
        val first = ActivityNotificationPlanner.start(c, active = emptySet())
        assertEquals("activity:a1", first.tag)
        assertFalse(first.update)

        val again = ActivityNotificationPlanner.start(c, active = setOf("activity:a1"))
        assertEquals("activity:a1", again.tag)
        assertTrue(again.update)

        // 別のアプリの表示は別
        assertFalse(ActivityNotificationPlanner.start(content(Activity(title = "t"), appId = "a2"), setOf("activity:a1")).update)
    }

    @Test
    fun endはそのアプリの表示だけ消す() {
        val active = setOf("activity:a1", "activity:a2")
        assertEquals(listOf("activity:a1"), ActivityNotificationPlanner.end("a1", active))
        assertEquals(emptyList<String>(), ActivityNotificationPlanner.end("a3", active))
    }

    @Test
    fun 消去は進行中の表示を全部_他の通知は見ない() {
        val active = setOf("activity:a2", "activity:a1", "timer.a1/home/t")
        assertEquals(listOf("activity:a1", "activity:a2"), ActivityNotificationPlanner.wipe(active))
        assertEquals(emptyList<String>(), ActivityNotificationPlanner.wipe(emptySet()))
    }
}
