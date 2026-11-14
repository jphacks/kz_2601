package jp.kz2601.metamon.services.sync

import kotlinx.coroutines.awaitCancellation
import kotlinx.coroutines.flow.toList
import kotlinx.coroutines.launch
import kotlinx.coroutines.test.TestScope
import kotlinx.coroutines.test.advanceTimeBy
import kotlinx.coroutines.test.runCurrent
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.IOException

class RealtimeDocumentSyncTest {

    private fun TestScope.run(sync: DocumentSync, after: Int = 1): MutableList<SyncUpdate> {
        val out = mutableListOf<SyncUpdate>()
        backgroundScope.launch { sync.events("app1", after).toList(out) }
        runCurrent()
        return out
    }

    private fun versions(out: List<SyncUpdate>) = out.filterIsInstance<SyncUpdate.Document>().map { it.document.version }
    private fun realtime(out: List<SyncUpdate>) = out.filterIsInstance<SyncUpdate.Realtime>().map { it.listening }

    // 版の比較

    @Test
    fun helloWithNewerVersionRefetchesOnce() = runTest {
        val fetcher = FakeFetcher(testScheduler, serverVersion = 3)
        val source = FakeEventSource(testScheduler).then { on ->
            on(ChangeEvent.Hello(3, listOf("post")))
            on(ChangeEvent.Dsl(3)) // 同じ版の通知が重なっても 2 回は流さない
            awaitCancellation()
        }
        val out = run(RealtimeDocumentSync(fetcher, source))
        assertEquals(listOf(true), realtime(out))
        assertEquals(listOf(3), versions(out))
        assertEquals(1, fetcher.calls.size)
    }

    @Test
    fun sameOrOlderVersionIsNotFetched() = runTest {
        val fetcher = FakeFetcher(testScheduler, serverVersion = 5)
        val source = FakeEventSource(testScheduler).then { on ->
            on(ChangeEvent.Hello(5, emptyList()))
            on(ChangeEvent.Dsl(4))
            awaitCancellation()
        }
        val out = run(RealtimeDocumentSync(fetcher, source), after = 5)
        assertTrue(versions(out).isEmpty())
        assertTrue(fetcher.calls.isEmpty())
    }

    @Test
    fun olderDocumentFromServerIsDropped() = runTest {
        // 遅れて届いた古い版(If-None-Match と別の版)で巻き戻さない
        val fetcher = FakeFetcher(testScheduler, serverVersion = 2)
        val source = FakeEventSource(testScheduler).then { on ->
            on(ChangeEvent.Dsl(9))
            awaitCancellation()
        }
        val out = run(RealtimeDocumentSync(fetcher, source), after = 4)
        assertTrue(versions(out).isEmpty())
    }

    @Test
    fun recordsEventIsForwarded() = runTest {
        val source = FakeEventSource(testScheduler).then { on ->
            on(ChangeEvent.Hello(1, listOf("post")))
            on(ChangeEvent.Records("post"))
            awaitCancellation()
        }
        val out = run(RealtimeDocumentSync(FakeFetcher(testScheduler), source))
        assertEquals(listOf(SyncUpdate.Realtime(true), SyncUpdate.Records("post")), out)
    }

    // 切断と再接続

    @Test
    fun byeReconnectsImmediatelyAndStaysRealtime() = runTest {
        val fetcher = FakeFetcher(testScheduler, serverVersion = 1)
        val source = FakeEventSource(testScheduler)
            .then { on -> on(ChangeEvent.Hello(1, emptyList())); on(ChangeEvent.Dsl(2)); true } // dsl の直後に bye
            .then { on -> on(ChangeEvent.Hello(2, emptyList())); awaitCancellation() }
        fetcher.serverVersion = 2
        val out = run(RealtimeDocumentSync(fetcher, source))
        assertEquals(listOf(0L to false, 0L to false), source.connects) // 待たずに張り直す
        assertEquals(listOf(true), realtime(out)) // ポーリングに落とさない
        assertEquals(listOf(2), versions(out))
    }

    @Test
    fun dropReconnectsWithExponentialBackoff() = runTest {
        val source = FakeEventSource(testScheduler)
            .then { on -> on(ChangeEvent.Hello(1, emptyList())); false } // bye 無しで切れた
            .then { throw IOException("refused") }
            .then { throw IOException("refused") }
            .then { on -> on(ChangeEvent.Hello(1, emptyList())); awaitCancellation() }
        val out = run(RealtimeDocumentSync(FakeFetcher(testScheduler), source))
        advanceTimeBy(10_000)
        runCurrent()
        assertEquals(listOf(0L, 1_000L, 3_000L, 7_000L), source.connects.map { it.first })
        assertEquals(listOf(true, false, true), realtime(out))
    }

    @Test
    fun backoffResetsAfterHello() = runTest {
        val source = FakeEventSource(testScheduler)
            .then { throw IOException("x") }
            .then { on -> on(ChangeEvent.Hello(1, emptyList())); false }
            .then { throw IOException("x") }
            .then { awaitCancellation() }
        run(RealtimeDocumentSync(FakeFetcher(testScheduler), source))
        advanceTimeBy(5_000)
        runCurrent()
        // 0 → 1s 待ち → (hello で 1s に戻る) → 1s → 2s
        assertEquals(listOf(0L, 1_000L, 2_000L, 4_000L), source.connects.map { it.first })
    }

    @Test
    fun backoffIsCapped() = runTest {
        val source = FakeEventSource(testScheduler)
        repeat(10) { source.then { throw IOException("x") } }
        run(RealtimeDocumentSync(FakeFetcher(testScheduler), source, maxBackoffMillis = 4_000))
        advanceTimeBy(20_000)
        runCurrent()
        val gaps = source.connects.map { it.first }.zipWithNext { a, b -> b - a }
        assertEquals(listOf(1_000L, 2_000L, 4_000L, 4_000L, 4_000L), gaps.take(5))
    }

    @Test
    fun unauthorizedRefreshesTokenOnNextConnect() = runTest {
        val source = FakeEventSource(testScheduler)
            .then { throw StreamUnauthorized() }
            .then { on -> on(ChangeEvent.Hello(1, emptyList())); true }
            .then { awaitCancellation() }
        run(RealtimeDocumentSync(FakeFetcher(testScheduler), source))
        advanceTimeBy(2_000)
        runCurrent()
        assertEquals(listOf(false, true, false), source.connects.map { it.second })
    }

    // ポーリングへの切り替え

    @Test
    fun pollsSlowlyWhileListening() = runTest {
        val fetcher = FakeFetcher(testScheduler)
        val source = FakeEventSource(testScheduler).then { on -> on(ChangeEvent.Hello(1, emptyList())); awaitCancellation() }
        run(RealtimeDocumentSync(fetcher, source))
        advanceTimeBy(125_000)
        runCurrent()
        assertEquals(listOf(60_000L, 120_000L), fetcher.calls) // 購読中は 60 秒ごとの保険だけ
    }

    @Test
    fun fallsBackToFastPollingWhenStreamDrops() = runTest {
        val fetcher = FakeFetcher(testScheduler)
        val source = FakeEventSource(testScheduler)
            .then { on -> on(ChangeEvent.Hello(1, emptyList())); kotlinx.coroutines.delay(10_000); false }
        repeat(20) { source.then { throw IOException("down") } }
        val out = run(RealtimeDocumentSync(fetcher, source, maxBackoffMillis = 60_000))
        advanceTimeBy(10_001)
        runCurrent()
        assertEquals(listOf(true, false), realtime(out))
        assertTrue(fetcher.calls.isEmpty()) // 60 秒の待ちは打ち切られる
        fetcher.serverVersion = 4 // 切れている間に他の端末が変えた
        advanceTimeBy(5_000)
        runCurrent()
        assertEquals(listOf(15_000L), fetcher.calls)
        assertEquals(listOf(4), versions(out))
    }

    @Test
    fun pollFailureIsRetriedNextCycle() = runTest {
        val fetcher = FakeFetcher(testScheduler, serverVersion = 2).apply { failing = true }
        val out = run(RealtimeDocumentSync(fetcher, FakeEventSource(testScheduler)))
        advanceTimeBy(5_001)
        runCurrent()
        assertTrue(versions(out).isEmpty())
        fetcher.failing = false
        advanceTimeBy(5_000)
        runCurrent()
        assertEquals(listOf(2), versions(out))
    }

    @Test
    fun pollingOnlySyncEmitsNewerVersions() = runTest {
        val fetcher = FakeFetcher(testScheduler, serverVersion = 1)
        val out = run(PollingDocumentSync(fetcher, intervalMillis = 5_000))
        advanceTimeBy(5_001)
        runCurrent()
        fetcher.serverVersion = 3
        advanceTimeBy(5_000)
        runCurrent()
        advanceTimeBy(5_000)
        runCurrent()
        assertEquals(listOf(3), versions(out))
        assertEquals(listOf(5_000L, 10_000L, 15_000L), fetcher.calls)
    }
}
