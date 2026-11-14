package jp.kz2601.metamon.notifications

import jp.kz2601.metamon.renderer.components.play.TimerRun
import jp.kz2601.metamon.services.consent.InMemoryKeyValueStore
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

class TimerRunStoreTest {
    private val kv = InMemoryKeyValueStore()
    private val store = TimerRunStore(kv)

    @Test
    fun 走行状態を保存して読み直せる_作り直しても残る() {
        store.save("a1/home/t", TimerRun(base = 12, startedAtMillis = 1_000L))
        store.save("a1/home/s", TimerRun(base = 30))
        val again = TimerRunStore(kv) // プロセスが終わった後
        assertEquals(TimerRun(12, 1_000L), again.load("a1/home/t"))
        assertEquals(TimerRun(30, null), again.load("a1/home/s"))
        assertNull(again.load("a1/home/none"))
    }

    @Test
    fun 初期状態はリセット扱いで消す() {
        store.save("a1/home/t", TimerRun(base = 5))
        store.save("a1/home/t", TimerRun())
        assertNull(store.load("a1/home/t"))
        assertNull(kv.getString(TimerRunStore.KEY_RUNS))
    }

    @Test
    fun 予約を置いて取り消せる() {
        store.setAlarm("a1/home/t", TimerAlarm(5_000L, "app", "t done"))
        assertEquals(mapOf("a1/home/t" to TimerAlarm(5_000L, "app", "t done")), TimerRunStore(kv).alarms())
        store.setAlarm("a1/home/t", null)
        assertTrue(store.alarms().isEmpty())
    }

    @Test
    fun アプリを消すとそのアプリの分だけ消え_取り消す予約を返す() {
        store.save("a1/home/t", TimerRun(1, 10L))
        store.save("a10/home/t", TimerRun(2, 20L))
        store.setAlarm("a1/home/t", TimerAlarm(1L, "", ""))
        store.setAlarm("a10/home/t", TimerAlarm(2L, "", ""))
        assertEquals(setOf("a1/home/t"), store.clearApp("a1"))
        assertNull(store.load("a1/home/t"))
        assertEquals(TimerRun(2, 20L), store.load("a10/home/t")) // 前方一致で別のアプリを消さない
        assertEquals(setOf("a10/home/t"), store.alarms().keys)
    }

    @Test
    fun 全部消す_LocalDataWipe() {
        store.save("a1/home/t", TimerRun(1, 10L))
        store.setAlarm("a2/home/t", TimerAlarm(1L, "", ""))
        assertEquals(setOf("a2/home/t"), store.clearAll())
        assertNull(store.load("a1/home/t"))
        assertTrue(store.alarms().isEmpty())
        assertNull(kv.getString(TimerRunStore.KEY_RUNS))
        assertNull(kv.getString(TimerRunStore.KEY_ALARMS))
    }

    @Test
    fun 壊れた保存は空として読む() {
        kv.putString(TimerRunStore.KEY_RUNS, "{broken")
        assertNull(store.load("a1/home/t"))
    }

    @Test
    fun 正確なアラームは許可がある時だけ() {
        assertEquals(TimerAlarmPolicy.Mode.EXACT, TimerAlarmPolicy.mode(34, canScheduleExact = true))
        assertEquals(TimerAlarmPolicy.Mode.INEXACT, TimerAlarmPolicy.mode(34, canScheduleExact = false))
        assertEquals(TimerAlarmPolicy.Mode.EXACT, TimerAlarmPolicy.mode(30, canScheduleExact = false))
    }

    @Test
    fun 予約はまだ先の時だけ_過ぎたか停止なら取り消す() {
        assertEquals(2_000L, TimerAlarmPolicy.scheduleAt(2_000L, nowMillis = 1_000L))
        assertNull(TimerAlarmPolicy.scheduleAt(1_000L, nowMillis = 1_000L))
        assertNull(TimerAlarmPolicy.scheduleAt(null, nowMillis = 1_000L))
        // 描画器の計算と合わせて: 開始 0 秒・60 秒の countdown は 60 秒後
        val end = TimerRun.notificationAt(true, TimerRun(0, 1_000L), 60, nowMillis = 1_000L)
        assertEquals(61_000L, TimerAlarmPolicy.scheduleAt(end, nowMillis = 1_000L))
    }

    @Test
    fun 見えているタイマーは数えて外す() {
        TimerVisibility.set("v/home/t", true)
        TimerVisibility.set("v/home/t", true)
        TimerVisibility.set("v/home/t", false)
        assertTrue(TimerVisibility.isShowing("v/home/t"))
        TimerVisibility.set("v/home/t", false)
        assertFalse(TimerVisibility.isShowing("v/home/t"))
        TimerVisibility.set("v/home/t", false) // 余計に外しても負にならない
        assertFalse(TimerVisibility.isShowing("v/home/t"))
    }
}
