package jp.kz2601.metamon.runtime

import jp.kz2601.metamon.runtime.RuntimeFixture.runtime
import kotlinx.coroutines.test.runCurrent
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

/** #1128: タイマーのアラームの予約は、サインアウト(アカウントの切り替え)を跨いだら入れない。 */
class TimerAccountChangeTest {
    private class Recorder : RuntimeHost {
        val calls = mutableListOf<Pair<String, Long?>>()
        override suspend fun timerChanged(key: String, appName: String, timerName: String, endsAtEpochMillis: Long?) {
            calls += key to endsAtEpochMillis
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
}
