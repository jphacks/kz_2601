package jp.kz2601.metamon.runtime

import jp.kz2601.metamon.notifications.TimerActivityUpdate
import jp.kz2601.metamon.renderer.components.play.TimerRun
import jp.kz2601.metamon.runtime.RuntimeFixture.runtime
import kotlinx.coroutines.test.runCurrent
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/** #1128: タイマーのアラームの予約は、サインアウト(アカウントの切り替え)を跨いだら入れない。 */
class TimerAccountChangeTest {
    private class Recorder : RuntimeHost {
        val calls = mutableListOf<Pair<String, Long?>>()
        override suspend fun timerChanged(key: String, appName: String, timerName: String, endsAtEpochMillis: Long?) {
            calls += key to endsAtEpochMillis
        }

        val activities = mutableListOf<TimerActivityUpdate>()
        var canPostActivity = true

        override suspend fun timerActivity(update: TimerActivityUpdate): Boolean {
            activities += update
            return canPostActivity
        }
    }

    @Test
    fun timerChangeIsForwardedToHost() = runTest {
        val host = Recorder()
        val rt = runtime(RuntimeFixture.dsl("bmi"), appId = "a", host = host)
        rt.timerChanged("t", "タイマー", 1_000L)
        runCurrent()
        assertEquals(listOf("a/t" to 1_000L), host.calls)
    }

    @Test
    fun timerChangeQueuedBeforeSignOutIsDropped() = runTest {
        val host = Recorder()
        val rt = runtime(RuntimeFixture.dsl("bmi"), appId = "a", host = host)
        rt.timerChanged("t", "タイマー", 1_000L) // 予約はまだ走っていない
        rt.resetForAccountChange()
        runCurrent()
        assertTrue("前の人のアラームは予約しない", host.calls.isEmpty())

        rt.timerChanged("t", "タイマー", 2_000L) // 切り替えの後の操作は通す
        runCurrent()
        assertEquals(1, host.calls.size)
        assertEquals(2_000L, host.calls.single().second)
    }

    /** #913: liveActivity の表示もアプリ名・appId を付けてホストに渡す。切り替えを跨いだら出さない。 */
    @Test
    fun timerActivityIsForwardedWithAppIdAndDroppedAcrossAccountChange() = runTest {
        val host = Recorder()
        val rt = runtime(RuntimeFixture.dsl("bmi"), appId = "a", host = host)
        val run = TimerRun(base = 5, startedAtMillis = 1_000L)
        rt.timerActivity("home/t", "タイマー", 60, run)
        runCurrent()
        val update = host.activities.single()
        assertEquals("a/home/t", update.key)
        assertEquals("a", update.appId)
        assertEquals("タイマー", update.label)
        assertEquals(60, update.totalSeconds)
        assertEquals(run, update.run)

        rt.timerActivity("home/t", "タイマー", 60, TimerRun())
        rt.resetForAccountChange()
        runCurrent()
        assertEquals("前の人の表示は出さない", 1, host.activities.size)
    }

    /** #913: 通知が使えない端末 / 許可が無い: 始めた時だけ一言を出し、止めた時は黙る。 */
    @Test
    fun timerActivityToastsOnlyWhenStarting() = runTest {
        val host = Recorder().apply { canPostActivity = false }
        val rt = runtime(RuntimeFixture.dsl("bmi"), appId = "a", host = host)
        rt.timerActivity("home/t", "タイマー", 60, TimerRun(base = 0, startedAtMillis = 1_000L))
        runCurrent()
        assertEquals(RuntimeCopy.ACTIVITY_NOTIFICATIONS_OFF, rt.toast)

        rt.dismissToast()
        rt.timerActivity("home/t", "タイマー", 60, TimerRun(base = 5))
        runCurrent()
        assertNull("止めた時は黙る", rt.toast)
    }
}
