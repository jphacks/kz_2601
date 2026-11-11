package jp.kz2601.metamon.services.local

import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.test.TestScope
import kotlinx.coroutines.test.advanceTimeBy
import kotlinx.coroutines.test.runCurrent
import kotlinx.coroutines.test.runTest
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import kotlin.coroutines.EmptyCoroutineContext

class RecordSyncEngineTest {
    private val client = FakeRecordsClient()
    private val net = ManualConnectivity(online = false)

    private fun TestScope.engine(scope: CoroutineScope = backgroundScope): RecordSyncEngine {
        val store = RecordStore(null, EmptyCoroutineContext)
        val queue = SyncQueue(null, store, client, BackoffPolicy.immediate, sleep = {}, io = EmptyCoroutineContext)
        return RecordSyncEngine(store, queue, client, net, scope).also { it.start() }
    }

    private fun data(v: String) = JsonObject(mapOf("title" to JsonPrimitive(v)))

    @Test
    fun offlineWriteIsSentWhenBackOnline() = runTest {
        val e = engine()
        client.offline = true
        val (list, id) = e.write("app", "todo", null, data("a"))
        runCurrent()
        assertTrue(id.startsWith("local-"))
        assertEquals(listOf(id), list.map { it.id }) // 圏外でもすぐ一覧に出る
        assertTrue(e.store.rows("app", "todo").single().unsent)
        assertTrue(client.calls.isEmpty()) // オフラインの間は送らない
        assertEquals(RecordSyncEngine.Status.Offline(1), e.status.value)

        client.offline = false
        net.set(true)
        runCurrent()
        assertEquals(listOf("create"), client.calls.map { it.kind })
        assertEquals(listOf("srv-1"), e.cached("app", "todo").map { it.id })
        assertEquals(RecordSyncEngine.Status.Synced, e.status.value)
    }

    @Test
    fun undoBeforeGraceSendsNothing() = runTest {
        net.set(true)
        val e = engine()
        e.write("app", "todo", null, data("a"))
        runCurrent()
        client.calls.clear()

        val (list, ticket) = e.deleteUndoably("app", "todo", "srv-1", graceMillis = 5_000)
        assertTrue(list.isEmpty()) // 隠すだけ
        advanceTimeBy(3_000)
        val back = e.undoDelete(ticket)
        runCurrent()
        assertEquals(listOf("srv-1"), back?.map { it.id })
        assertTrue(client.calls.isEmpty())
        assertEquals(PendingOp.NONE, e.store.row("app", "todo", "srv-1")?.pendingOp)
    }

    @Test
    fun deleteIsSentAfterGrace() = runTest {
        net.set(true)
        val e = engine()
        e.write("app", "todo", null, data("a"))
        runCurrent()
        client.calls.clear()

        val (_, ticket) = e.deleteUndoably("app", "todo", "srv-1", graceMillis = 5_000)
        advanceTimeBy(5_001)
        runCurrent()
        assertEquals(listOf("delete"), client.calls.map { it.kind })
        assertNull(e.undoDelete(ticket)) // もう送った
        assertNull(e.store.row("app", "todo", "srv-1"))
    }

    @Test
    fun undoableDeleteOfUnsentCreateFollowsTheServerId() = runTest {
        val e = engine()
        client.offline = true
        val (_, tempId) = e.write("app", "todo", null, data("a"))
        val (_, ticket) = e.deleteUndoably("app", "todo", tempId, graceMillis = 5_000)
        // 待つ間に復帰して作成が送られる → 仮 id がサーバー id に替わっても隠したまま
        client.offline = false
        net.set(true)
        runCurrent()
        assertTrue(e.cached("app", "todo").isEmpty())
        advanceTimeBy(5_001)
        runCurrent()
        assertEquals(listOf("create", "delete"), client.calls.map { it.kind })
        assertEquals("srv-1", client.calls[1].id)
        assertNull(e.undoDelete(ticket))
    }

    @Test
    fun backgroundCommitsPendingDeletesImmediately() = runTest {
        net.set(true)
        val e = engine()
        e.write("app", "todo", null, data("a"))
        runCurrent()
        client.calls.clear()
        e.deleteUndoably("app", "todo", "srv-1", graceMillis = 60_000)
        e.setForeground(false)
        advanceTimeBy(1_000)
        runCurrent()
        assertEquals(listOf("delete"), client.calls.map { it.kind })
        assertTrue(e.pendingDeletions.isEmpty())
    }

    @Test
    fun offlineDeleteOfSyncedRecordWaitsForReconnect() = runTest {
        net.set(true)
        val e = engine()
        e.write("app", "todo", null, data("a"))
        runCurrent()
        net.set(false)
        client.offline = true
        client.calls.clear()
        assertTrue(e.delete("app", "todo", "srv-1").isEmpty())
        runCurrent()
        assertEquals(PendingOp.DELETE, e.store.row("app", "todo", "srv-1")?.pendingOp)
        assertTrue(client.calls.isEmpty())
        client.offline = false
        net.set(true)
        runCurrent()
        assertEquals(listOf("delete"), client.calls.map { it.kind })
    }

    @Test
    fun pullKeepsUnsentRows() = runTest {
        val e = engine()
        client.offline = true
        val (_, tempId) = e.write("app", "todo", null, data("mine"))
        client.offline = false
        client.server += jp.kz2601.metamon.services.Record(id = "srv-x", data = data("theirs"), createdAt = "2020-01-01T00:00:00Z")
        val list = e.pull("app", "todo")
        assertEquals(listOf(tempId, "srv-x"), list.map { it.id })
    }

    @Test
    fun wipeDropsEverything() = runTest {
        val e = engine()
        e.write("app", "todo", null, data("a"))
        e.deleteUndoably("app", "todo", "whatever", graceMillis = 5_000)
        e.wipe()
        runCurrent()
        assertEquals(0, e.queue.count())
        assertTrue(e.cached("app", "todo").isEmpty())
        assertTrue(e.pendingDeletions.isEmpty())
        assertNotNull(e.status.value)
    }
}
