package jp.kz2601.metamon.runtime

import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.records.Record
import jp.kz2601.metamon.runtime.RuntimeFixture.runtime
import kotlinx.coroutines.test.advanceTimeBy
import kotlinx.coroutines.test.runCurrent
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

/** #886: DSL の読み込み・記録の変化の後、まとめて 1 回 host に通知の予約し直しを頼む。 */
class NotificationsChangedTest {
    private class Recorder : RuntimeHost {
        val calls = mutableListOf<Pair<String, Set<String>>>()
        override suspend fun notificationsChanged(appId: String, dsl: AppDSL, records: Map<String, List<Record>>) {
            calls += appId to records.keys
        }
    }

    @Test
    fun bootstrapAsksHostOnceAfterRecordsLoad() = runTest {
        val host = Recorder()
        val rt = runtime(RuntimeFixture.dsl("training"), appId = "train", host = host)
        rt.bootstrap()
        advanceTimeBy(1_000)
        runCurrent()
        assertEquals("読み込みが続いても 1 回にまとめる", 1, host.calls.size)
        // 端末はオフラインで端末内にも記録が無い = 読めなかった entity は渡さない(#1124: 空で評価して後で誤って鳴らさない)
        assertEquals("train" to emptySet<String>(), host.calls.single())
    }

    @Test
    fun failedEntityIsPassedOnceSavedLocally() = runTest {
        val host = Recorder()
        val rt = runtime(RuntimeFixture.dsl("training"), appId = "train", host = host)
        rt.bootstrap()
        advanceTimeBy(1_000)
        runCurrent()
        rt.createRecord("workout", emptyMap())
        advanceTimeBy(1_000)
        runCurrent()
        assertEquals("端末内に保存した記録で埋める", "train" to setOf("workout"), host.calls.last())
    }

    @Test
    fun appWithoutNotificationsDoesNotCallHost() = runTest {
        val host = Recorder()
        val rt = runtime(RuntimeFixture.dsl("bmi"), host = host)
        rt.bootstrap()
        advanceTimeBy(1_000)
        runCurrent()
        assertTrue(host.calls.isEmpty())
    }
}
