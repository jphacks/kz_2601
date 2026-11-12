package jp.kz2601.metamon.services.local

import jp.kz2601.metamon.services.http.ApiError
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.flow.toList
import kotlinx.coroutines.launch
import kotlinx.coroutines.test.UnconfinedTestDispatcher
import kotlinx.coroutines.test.runTest
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.put
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.rules.TemporaryFolder
import java.io.IOException
import kotlin.coroutines.EmptyCoroutineContext

class SyncQueueTest {
    @get:Rule
    val tmp = TemporaryFolder()

    private val client = FakeRecordsClient()
    private val store = RecordStore(null, EmptyCoroutineContext)
    private val sleeps = mutableListOf<Long>()
    private fun queue(dir: java.io.File? = null, s: RecordStore = store) =
        SyncQueue(dir, s, client, BackoffPolicy(baseMillis = 100, multiplier = 2.0, maxAttempts = 3), { sleeps += it }, EmptyCoroutineContext)

    private fun data(vararg pairs: Pair<String, String>) = JsonObject(pairs.associate { it.first to JsonPrimitive(it.second) })

    private suspend fun create(q: SyncQueue, clientId: String, d: JsonObject = data("title" to "a")): String {
        val id = LocalRecord.temporaryId(clientId)
        store.upsert(LocalRecord("app", "todo", id, d, createdAt = "2026-11-11T00:00:00Z", pendingOp = PendingOp.CREATE, clientId = clientId))
        q.enqueue(SyncOperation(id = clientId, appId = "app", entity = "todo", recordId = id, kind = PendingOp.CREATE, data = d))
        return id
    }

    @Test
    fun createThenUpdateIsFoldedIntoOneCreate() = runTest {
        val q = queue()
        val id = create(q, "c1")
        val r = q.enqueue(SyncOperation(appId = "app", entity = "todo", recordId = id, kind = PendingOp.UPDATE, data = data("done" to "yes")))
        assertEquals(SyncQueue.EnqueueResult.MERGED, r)
        q.drain()
        assertEquals(listOf("create"), client.calls.map { it.kind })
        assertEquals(data("title" to "a", "done" to "yes"), client.calls[0].data)
        assertEquals("c1", client.calls[0].clientId) // 冪等キー
        val row = store.rows("app", "todo").single()
        assertEquals("srv-1", row.recordId)
        assertEquals(PendingOp.NONE, row.pendingOp)
        assertEquals("c1", row.clientId)
    }

    @Test
    fun createThenDeleteSendsNothing() = runTest {
        val q = queue()
        val id = create(q, "c1")
        q.enqueue(SyncOperation(appId = "app", entity = "todo", recordId = id, kind = PendingOp.UPDATE, data = data("x" to "1")))
        val r = q.enqueue(SyncOperation(appId = "app", entity = "todo", recordId = id, kind = PendingOp.DELETE))
        assertEquals(SyncQueue.EnqueueResult.CANCELLED_CREATE, r)
        assertEquals(0, q.count())
        q.drain()
        assertTrue(client.calls.isEmpty())
    }

    @Test
    fun keepsOrderAndUsesServerIdAfterCreate() = runTest {
        val q = queue()
        val id = create(q, "c1")
        q.drain()
        // 仮 id のまま積まれた更新・ref の値はサーバー id に引き直して送る
        q.enqueue(SyncOperation(appId = "app", entity = "todo", recordId = id, kind = PendingOp.UPDATE, data = data("ref" to id)))
        q.enqueue(SyncOperation(appId = "app", entity = "todo", recordId = id, kind = PendingOp.DELETE))
        q.drain()
        assertEquals(listOf("create", "delete"), client.calls.map { it.kind }) // 更新は削除で畳まれる
        assertEquals("srv-1", client.calls[1].id)
        assertTrue(store.rows("app", "todo").isEmpty())
    }

    @Test
    fun retriesTransientFailuresWithBackoffThenPauses() = runTest(UnconfinedTestDispatcher()) {
        val q = queue()
        create(q, "c1")
        val events = mutableListOf<SyncEvent>()
        val job = launch { q.events.toList(events) }
        client.offline = true
        q.drain()
        assertEquals(listOf(100L, 200L), sleeps) // 3 回目で止まる
        assertTrue(q.isPaused)
        assertEquals(SyncEvent.Paused(1), events.last())
        assertEquals(1, q.count())

        client.offline = false
        q.resume()
        assertEquals(0, q.count())
        assertEquals(SyncEvent.Drained, events.last())
        job.cancel()
    }

    // 止まった知らせを受けてすぐ resume しても取りこぼさない(前は drain がまだ isDraining のままで、resume が素通りした)
    @Test
    fun resumeRightAfterPausedIsNotLost() = runTest(UnconfinedTestDispatcher()) {
        val q = SyncQueue(null, store, client, BackoffPolicy(baseMillis = 0, multiplier = 1.0, maxAttempts = 1), { }, EmptyCoroutineContext)
        create(q, "c1")
        client.offline = true
        var drainingAtPause: Boolean? = null
        // Unconfined: 知らせを出した drain の中でそのまま受ける(出した直後に別のスレッドで受けたのと同じ)
        val job = launch(Dispatchers.Unconfined) {
            q.events.collect { ev ->
                if (ev is SyncEvent.Paused) {
                    drainingAtPause = q.isDraining
                    client.offline = false
                    q.resume()
                }
            }
        }
        q.drain()
        assertEquals(false, drainingAtPause)
        assertFalse(q.isPaused)
        assertEquals(0, q.count())
        assertEquals(2, client.calls.size)
        job.cancel()
    }

    // 送っている間に resume されたら、その 1 回が失敗しても止めずにもう一度送る
    @Test
    fun resumeWhileSendingRetriesInsteadOfPausing() = runTest(UnconfinedTestDispatcher()) {
        val q = SyncQueue(null, store, client, BackoffPolicy(baseMillis = 0, multiplier = 1.0, maxAttempts = 1), { }, EmptyCoroutineContext)
        create(q, "c1")
        client.offline = true
        client.onCall = {
            if (client.calls.size == 1) q.resume() else client.offline = false
        }
        q.drain()
        assertFalse(q.isPaused)
        assertFalse(q.isDraining)
        assertEquals(0, q.count())
        assertEquals(2, client.calls.size)
    }

    @Test
    fun conflictRevertsPendingAndKeepsGoing() = runTest(UnconfinedTestDispatcher()) {
        val q = queue()
        store.upsert(LocalRecord("app", "todo", "srv-9", data("t" to "local"), pendingOp = PendingOp.UPDATE))
        q.enqueue(SyncOperation(appId = "app", entity = "todo", recordId = "srv-9", kind = PendingOp.UPDATE, data = data("t" to "local")))
        create(q, "c2")
        val events = mutableListOf<SyncEvent>()
        val job = launch { q.events.toList(events) }
        client.failures += ApiError.Server(409, "version_conflict", "conflict")
        q.drain()
        assertTrue(events.first() is SyncEvent.Conflict)
        assertEquals(PendingOp.NONE, store.row("app", "todo", "srv-9")?.pendingOp)
        assertEquals(listOf("update", "create"), client.calls.map { it.kind })
        job.cancel()
    }

    @Test
    fun rejectedCreateRemovesTheLocalRow() = runTest(UnconfinedTestDispatcher()) {
        val q = queue()
        val id = create(q, "c1")
        val events = mutableListOf<SyncEvent>()
        val job = launch { q.events.toList(events) }
        client.failures += ApiError.Server(422, "invalid_record", "bad", buildJsonObject { put("field", "title") })
        q.drain()
        assertEquals(SyncEvent.Rejected("app", "todo", "bad", "title", id), events.first())
        assertNull(store.row("app", "todo", id))
        job.cancel()
    }

    @Test
    fun refusalsAreToldApart() = runTest(UnconfinedTestDispatcher()) {
        val q = queue()
        val events = mutableListOf<SyncEvent>()
        val job = launch { q.events.toList(events) }
        client.failures += ApiError.Server(403, "forbidden", "x", buildJsonObject { put("reason", "not_member") })
        client.failures += ApiError.Server(403, "forbidden", "x", buildJsonObject { put("reason", "admins_only") })
        client.failures += ApiError.Server(403, "app_stopped", "x")
        client.failures += ApiError.Server(403, "forbidden", "x")
        repeat(4) { create(q, "c$it") }
        q.drain()
        val kinds = events.filterNot { it is SyncEvent.Drained }.map { it::class.simpleName }
        assertEquals(listOf("NotMember", "Denied", "AppStopped", "Forbidden"), kinds)
        assertTrue(store.rows("app", "todo").isEmpty())
        job.cancel()
    }

    @Test
    fun survivesRestart() = runTest {
        val dir = tmp.newFolder()
        val s1 = RecordStore(dir, EmptyCoroutineContext)
        val q1 = SyncQueue(dir, s1, client, io = EmptyCoroutineContext)
        val d = data("title" to "a")
        s1.upsert(LocalRecord("app", "todo", "local-c1", d, pendingOp = PendingOp.CREATE, clientId = "c1"))
        q1.enqueue(SyncOperation(id = "c1", appId = "app", entity = "todo", recordId = "local-c1", kind = PendingOp.CREATE, data = d))

        // 作り直す = 再起動
        val s2 = RecordStore(dir, EmptyCoroutineContext)
        val q2 = SyncQueue(dir, s2, client, io = EmptyCoroutineContext)
        assertEquals(1, q2.count())
        assertEquals(PendingOp.CREATE, s2.row("app", "todo", "local-c1")?.pendingOp)
        q2.drain()
        assertEquals("c1", client.calls.single().clientId)
        assertEquals("srv-1", RecordStore(dir, EmptyCoroutineContext).rows("app", "todo").single().recordId)
    }

    @Test
    fun classify() {
        assertEquals(SyncQueue.Outcome.Transient, SyncQueue.classify(IOException()))
        assertEquals(SyncQueue.Outcome.Transient, SyncQueue.classify(ApiError.Http(503, "")))
        assertEquals(SyncQueue.Outcome.Transient, SyncQueue.classify(ApiError.Http(429, "")))
        assertEquals(SyncQueue.Outcome.Transient, SyncQueue.classify(ApiError.Http(401, "")))
        assertEquals(SyncQueue.Outcome.Conflict, SyncQueue.classify(ApiError.Http(404, "")))
        assertEquals(SyncQueue.Outcome.Forbidden, SyncQueue.classify(ApiError.Http(403, "")))
        assertTrue(SyncQueue.classify(ApiError.Http(400, "")) is SyncQueue.Outcome.Rejected)
    }
}
