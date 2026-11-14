package jp.kz2601.metamon.services.sync

import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.NonCancellable
import kotlinx.coroutines.awaitCancellation
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.FlowCollector
import kotlinx.coroutines.flow.MutableSharedFlow
import kotlinx.coroutines.flow.flow
import kotlinx.coroutines.flow.toList
import kotlinx.coroutines.launch
import kotlinx.coroutines.test.TestScope
import kotlinx.coroutines.test.runCurrent
import kotlinx.coroutines.test.runTest
import kotlinx.coroutines.withContext
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
    fun statusFollowsSubscriptionAndSse() = runTest {
        val h = harness()
        assertEquals("張る前は出さない", HotReloadStatus.OFF, h.hot.status.value)

        h.hot.start("app1", 1)
        runCurrent()
        assertEquals("SSE の知らせが来るまではポーリング", HotReloadStatus.POLLING, h.hot.status.value)

        h.sync.updates.emit(SyncUpdate.Realtime(true))
        runCurrent()
        assertEquals(HotReloadStatus.REALTIME, h.hot.status.value)

        h.sync.updates.emit(SyncUpdate.Realtime(false))
        runCurrent()
        assertEquals("切れたらポーリングに落ちる", HotReloadStatus.POLLING, h.hot.status.value)

        h.sync.updates.emit(SyncUpdate.Realtime(true))
        h.hot.setForeground(false)
        runCurrent()
        assertEquals("背面では購読しない", HotReloadStatus.OFF, h.hot.status.value)

        h.hot.setForeground(true)
        runCurrent()
        assertEquals("張り直した直後はポーリング", HotReloadStatus.POLLING, h.hot.status.value)

        h.hot.stop()
        runCurrent()
        assertEquals(HotReloadStatus.OFF, h.hot.status.value)
        assertFalse(h.hot.realtime.value)
    }

    @Test
    fun lateRealtimeFromCancelledSubscriptionOfSameAppIsIgnored() = runTest {
        // 取り消しが collect の途中で効かず、前の購読が張り直しの後に知らせを流してしまう場合
        val release = CompletableDeferred<Unit>()
        var calls = 0
        val sync = DocumentSync { _, _ ->
            val first = calls++ == 0
            object : Flow<SyncUpdate> {
                override suspend fun collect(collector: FlowCollector<SyncUpdate>) {
                    if (first) {
                        withContext(NonCancellable) { release.await() }
                        collector.emit(SyncUpdate.Realtime(true))
                    } else {
                        awaitCancellation()
                    }
                }
            }
        }
        val hot = HotReload(sync, backgroundScope)
        hot.start("app1", 1)
        runCurrent()
        hot.start("app1", 1) // 同じアプリに張り直す
        runCurrent()
        release.complete(Unit)
        runCurrent()
        assertEquals("前の購読の知らせで REALTIME にしない", HotReloadStatus.POLLING, hot.status.value)
        assertFalse(hot.realtime.value)
    }

    @Test
    fun foregroundWithoutAppStaysOff() = runTest {
        val h = harness()
        h.hot.setForeground(false)
        h.hot.setForeground(true)
        runCurrent()
        assertEquals(HotReloadStatus.OFF, h.hot.status.value)
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
