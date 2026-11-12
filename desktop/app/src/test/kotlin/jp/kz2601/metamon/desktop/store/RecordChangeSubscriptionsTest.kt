package jp.kz2601.metamon.desktop.store

import jp.kz2601.metamon.desktop.AppHub
import jp.kz2601.metamon.services.sync.DocumentSync
import jp.kz2601.metamon.services.sync.SyncUpdate
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.MutableSharedFlow
import kotlinx.coroutines.test.TestScope
import kotlinx.coroutines.test.advanceTimeBy
import kotlinx.coroutines.test.runCurrent
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

/** アプリごとに変化の通知を 1 本張り、記録の変化で表示中の entity を取り直す(#2808)。通知は作り物の DocumentSync。 */
@OptIn(ExperimentalCoroutinesApi::class)
class RecordChangeSubscriptionsTest {
    /** アプリごとの通知の口。events() を何回呼んだか(= 何本張ったか)を数える。 */
    private class FakeSync : DocumentSync {
        val flows = HashMap<String, MutableSharedFlow<SyncUpdate>>()
        val calls = mutableListOf<String>()

        fun flow(appId: String) = flows.getOrPut(appId) { MutableSharedFlow() }

        override fun events(appId: String, afterVersion: Int): Flow<SyncUpdate> {
            calls += appId
            return flow(appId)
        }

        fun listeners(appId: String) = flow(appId).subscriptionCount.value
    }

    private class Harness(scope: CoroutineScope, onServer: (String) -> Boolean = { true }) {
        val sync = FakeSync()
        val refetched = mutableListOf<Pair<String, Set<String>>>()
        val subs = RecordChangeSubscriptions(
            sync = sync,
            scope = scope,
            onServer = onServer,
            version = { 1 },
            visible = { setOf("posts", "todo") },
            refetch = { appId, entities -> refetched += appId to entities },
            log = {},
        )
    }

    private fun TestScope.harness(onServer: (String) -> Boolean = { true }) = Harness(backgroundScope, onServer)

    @Test
    fun batchesChangesFor250msAndRefetchesOnlyVisible() = runTest {
        val h = harness()
        h.subs.open("a")
        runCurrent()
        h.sync.flow("a").emit(SyncUpdate.Records("posts"))
        h.sync.flow("a").emit(SyncUpdate.Records("hidden"))
        advanceTimeBy(100)
        h.sync.flow("a").emit(SyncUpdate.Records("todo"))
        h.sync.flow("a").emit(SyncUpdate.Records("posts"))
        advanceTimeBy(140)
        runCurrent()
        assertTrue(h.refetched.isEmpty())

        advanceTimeBy(20)
        runCurrent()
        assertEquals(listOf("a" to setOf("posts", "todo")), h.refetched)

        // 表示していない entity だけの変化は取り直さない
        h.sync.flow("a").emit(SyncUpdate.Records("hidden"))
        advanceTimeBy(300)
        runCurrent()
        assertEquals(1, h.refetched.size)
    }

    @Test
    fun oneSubscriptionPerAppAndStopsWhenAllWindowsClose() = runTest {
        val h = harness()
        h.subs.open("a")
        h.subs.open("a")
        h.subs.open("b")
        runCurrent()
        assertEquals(listOf("a", "b"), h.sync.calls)
        assertEquals(1, h.sync.listeners("a"))

        h.subs.close("a")
        runCurrent()
        assertEquals(setOf("a", "b"), h.subs.active())
        assertEquals(1, h.sync.listeners("a"))

        h.subs.close("a")
        runCurrent()
        assertEquals(setOf("b"), h.subs.active())
        assertEquals(0, h.sync.listeners("a"))

        // 閉じた後の通知は取り直さない
        h.sync.flow("a").emit(SyncUpdate.Records("posts"))
        advanceTimeBy(300)
        runCurrent()
        assertTrue(h.refetched.isEmpty())
    }

    @Test
    fun localOnlyAppIsNotSubscribed() = runTest {
        val h = harness(onServer = { it != "local" })
        h.subs.open("local")
        runCurrent()
        assertTrue(h.sync.calls.isEmpty())
        assertTrue(h.subs.active().isEmpty())
        h.subs.close("local")
    }

    @Test
    fun reconnectResubscribesAndRefetchesAllVisible() = runTest {
        val h = harness()
        h.subs.open("a")
        runCurrent()
        h.subs.reconnectAll()
        runCurrent()
        assertEquals(listOf("a", "a"), h.sync.calls)
        assertEquals(1, h.sync.listeners("a"))

        advanceTimeBy(300)
        runCurrent()
        assertEquals(listOf("a" to setOf("posts", "todo")), h.refetched)
    }

    @Test
    fun appHubOpensOnFirstWindowAndClosesOnLast() = runTest {
        val h = harness()
        val hub = AppHub(live = h.subs)
        hub.acquire("shopping")
        hub.acquire("shopping")
        runCurrent()
        assertEquals(listOf("shopping"), h.sync.calls)

        hub.release("shopping")
        assertEquals(setOf("shopping"), h.subs.active())
        hub.release("shopping")
        assertTrue(h.subs.active().isEmpty())
    }

    @Test
    fun resumeDetectorSeesSleepAndNetworkReturn() {
        var clock = 0L
        var nets = setOf("en0")
        val d = ResumeDetector(now = { clock }, networks = { nets })

        clock += 5_000
        assertFalse(d.tick())

        // スリープ明け(時計が大きく飛んだ)
        clock += 10 * 60_000
        assertTrue(d.tick())

        // 切れただけでは張り直さない・戻ったら張り直す
        clock += 5_000
        nets = emptySet()
        assertFalse(d.tick())
        clock += 5_000
        nets = setOf("en0")
        assertTrue(d.tick())

        // 別のネットワークに替わった
        clock += 5_000
        nets = setOf("en1")
        assertTrue(d.tick())
        clock += 5_000
        assertFalse(d.tick())
    }
}
