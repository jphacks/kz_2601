package jp.kz2601.metamon.services.sync

import kotlinx.coroutines.flow.MutableSharedFlow
import kotlinx.coroutines.flow.flow
import kotlinx.coroutines.flow.toList
import kotlinx.coroutines.launch
import kotlinx.coroutines.test.TestScope
import kotlinx.coroutines.test.runCurrent
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class HotReloadTest {

    /** 集め始めた (appId, version) を残し、[updates] を流す。 */
    private class FakeSync : DocumentSync {
        val starts = mutableListOf<Pair<String, Int>>()
        val updates = MutableSharedFlow<SyncUpdate>()
        var active = 0

        override fun events(appId: String, afterVersion: Int) = flow {
            starts += appId to afterVersion
            active++
            try {
                updates.collect { emit(it) }
            } finally {
                active--
            }
        }
    }

    private class Harness(val sync: FakeSync, val hot: HotReload, val docs: List<Int>, val records: List<RecordsChange>)

    private fun TestScope.harness(): Harness {
        val sync = FakeSync()
        val hot = HotReload(sync, backgroundScope)
        val docs = mutableListOf<Int>()
        val records = mutableListOf<RecordsChange>()
        backgroundScope.launch { hot.documents.collect { docs += it.version } }
        backgroundScope.launch { hot.recordsChange.toList(records) }
        runCurrent()
        return Harness(sync, hot, docs, records)
    }

    @Test
    fun forwardsOnlyNewerDocuments() = runTest {
        val h = harness()
        h.hot.start("app1", 3)
        runCurrent()
        h.sync.updates.emit(SyncUpdate.Document(fetched(4)))
        h.sync.updates.emit(SyncUpdate.Document(fetched(4)))
        h.hot.noteVersion(6) // 自分で生成した版
        h.sync.updates.emit(SyncUpdate.Document(fetched(6)))
        h.sync.updates.emit(SyncUpdate.Document(fetched(2, appId = "other"))) // 別アプリの残り
        runCurrent()
        assertEquals(listOf(4), h.docs)
    }

    @Test
    fun stopsInBackgroundAndResumesFromCurrentVersion() = runTest {
        val h = harness()
        h.hot.start("app1", 1)
        runCurrent()
        h.sync.updates.emit(SyncUpdate.Realtime(true))
        h.sync.updates.emit(SyncUpdate.Document(fetched(2)))
        runCurrent()
        assertTrue(h.hot.realtime.value)

        h.hot.setForeground(false)
        runCurrent()
        assertEquals(0, h.sync.active) // 背面では購読しない
        assertFalse(h.hot.realtime.value)

        h.hot.setForeground(true)
        runCurrent()
        assertEquals(1, h.sync.active)
        assertEquals(listOf("app1" to 1, "app1" to 2), h.sync.starts) // 当てた版から張り直す
        assertEquals(listOf<RecordsChange>(RecordsChange.All), h.records) // 背面の間の記録を読み直す
    }

    @Test
    fun realtimeRecoveryAsksToReloadRecords() = runTest {
        val h = harness()
        h.hot.start("app1", 1)
        runCurrent()
        h.sync.updates.emit(SyncUpdate.Realtime(true))
        h.sync.updates.emit(SyncUpdate.Records("post"))
        h.sync.updates.emit(SyncUpdate.Realtime(false))
        h.sync.updates.emit(SyncUpdate.Realtime(true))
        runCurrent()
        assertEquals(listOf(RecordsChange.Entity("post"), RecordsChange.All), h.records)
    }

    @Test
    fun switchingAppsRestartsSubscription() = runTest {
        val h = harness()
        h.hot.start("app1", 1)
        runCurrent()
        h.hot.start("app2", 7)
        runCurrent()
        assertEquals(1, h.sync.active)
        assertEquals(listOf("app1" to 1, "app2" to 7), h.sync.starts)
        h.hot.stop()
        runCurrent()
        assertEquals(0, h.sync.active)
    }

    @Test
    fun noSubscriptionWhileStartedInBackground() = runTest {
        val h = harness()
        h.hot.setForeground(false)
        h.hot.start("app1", 1)
        runCurrent()
        assertTrue(h.sync.starts.isEmpty())
    }
}
