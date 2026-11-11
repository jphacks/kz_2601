package jp.kz2601.metamon.runtime

import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.AppDocument
import jp.kz2601.metamon.dsl.records.JSONValue
import jp.kz2601.metamon.dsl.records.Record
import jp.kz2601.metamon.services.AppConfigClient
import jp.kz2601.metamon.services.local.BackoffPolicy
import jp.kz2601.metamon.services.local.ConnectivityMonitor
import jp.kz2601.metamon.services.local.RecordStore
import jp.kz2601.metamon.services.local.RecordSyncEngine
import jp.kz2601.metamon.services.local.SyncQueue
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.test.TestScope
import kotlinx.coroutines.test.advanceTimeBy
import kotlinx.coroutines.test.runCurrent
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertSame
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.IOException
import java.lang.reflect.Proxy
import kotlin.coroutines.EmptyCoroutineContext
import jp.kz2601.metamon.services.Record as StoredRecord

/**
 * #891: 前面に戻った時の取り直し(iOS の `setForeground(true)` → `catchUpAfterBackground`)と背面での確定。
 * 順番は 送信待ち → DSL → 表示中の記録 → 通知の予約し直し。
 */
class ForegroundCatchUpTest {
    private val training = RuntimeFixture.dsl("training")
    private val kakeibo = RuntimeFixture.dsl("kakeibo")

    private class Online : ConnectivityMonitor {
        override val online: StateFlow<Boolean> = MutableStateFlow(true)
    }

    /** 呼ばれた順を [events] に残す appconfig。[serverDown] の間は送信が通信断になる。 */
    private class Server(val events: MutableList<String>) {
        var serverDown = false
        val client: AppConfigClient = Proxy.newProxyInstance(
            AppConfigClient::class.java.classLoader,
            arrayOf(AppConfigClient::class.java),
        ) { _, method, args ->
            when (method.name) {
                "listRecords" -> {
                    events += "pull:${args[1]}"
                    emptyList<StoredRecord>()
                }
                "createRecord" -> {
                    if (serverDown) throw IOException("down")
                    events += "send:${args[1]}"
                    StoredRecord(id = "srv-1", entity = args[1] as String)
                }
                "toString" -> "server"
                else -> throw IOException("offline")
            }
        } as AppConfigClient
    }

    private class Host(val events: MutableList<String>) : RuntimeHost {
        override suspend fun notificationsChanged(appId: String, dsl: AppDSL, records: Map<String, List<Record>>) {
            events += "notify"
        }
    }

    private class Fixture(val rt: AppRuntime, val server: Server, val events: MutableList<String>)

    private fun TestScope.fixture(dsl: AppDSL = training, remote: () -> AppDocument? = { null }): Fixture {
        val events = mutableListOf<String>()
        val server = Server(events)
        val store = RecordStore(null, EmptyCoroutineContext)
        val queue = SyncQueue(null, store, server.client, BackoffPolicy.immediate, sleep = {}, io = EmptyCoroutineContext)
        val engine = RecordSyncEngine(store, queue, server.client, Online(), backgroundScope)
        val rt = AppRuntime(
            AppDocument(APP, 1, dsl), engine, backgroundScope, Host(events),
            restore = { appId ->
                events += "dsl"
                remote()?.copy(appId = appId) ?: throw IOException("unchanged")
            },
        )
        return Fixture(rt, server, events)
    }

    private fun TestScope.settle() {
        advanceTimeBy(1_000)
        runCurrent()
    }

    @Test
    fun foregroundSendsPendingThenFetchesDslThenRecordsThenNotifications() = runTest {
        val f = fixture()
        f.rt.bootstrap()
        settle()
        f.server.serverDown = true
        f.rt.createRecord("workout", mapOf("name" to JSONValue.Str("squat")))
        settle()
        f.rt.setForeground(false)
        settle()
        f.server.serverDown = false
        f.events.clear()

        f.rt.setForeground(true)
        settle()

        // 記録が変わった後の通知の見張り(#886)も予約し直すので notify は重なってよい。順番だけを見る
        assertEquals(listOf("send:workout", "dsl", "pull:workout", "notify"), f.events.distinct())
    }

    @Test
    fun newerDslFromAnotherDeviceIsShownOnForeground() = runTest {
        var remote: AppDocument? = null
        val f = fixture(remote = { remote })
        f.rt.bootstrap()
        settle()
        f.rt.setForeground(false)
        remote = AppDocument(APP, 5, kakeibo)

        f.rt.setForeground(true)
        settle()

        assertSame(kakeibo, f.rt.dsl)
        assertEquals(5, f.rt.document.version)
    }

    @Test
    fun recordsAreNotPulledTwiceOnForeground() = runTest {
        val f = fixture()
        f.rt.bootstrap()
        settle()
        f.rt.setForeground(false)
        f.events.clear()

        f.rt.setForeground(true)
        settle()

        assertEquals(1, f.events.count { it == "pull:workout" })
    }

    @Test
    fun noCatchUpBeforeBootstrap() = runTest {
        val f = fixture()
        f.rt.setForeground(false)
        f.rt.setForeground(true)
        settle()
        assertTrue("起動の前は bootstrap が読む", f.events.isEmpty())
    }

    @Test
    fun backgroundCommitsDeletionsAndDropsUndo() = runTest {
        val f = fixture()
        f.rt.bootstrap()
        settle()
        val id = f.rt.createRecord("workout", mapOf("name" to JSONValue.Str("squat")))!!
        settle()
        f.rt.deleteUndoably("workout", id)
        assertTrue(f.rt.state.value.recordUndo != null)

        f.rt.setForeground(false)
        settle()

        assertNull("背面で確定したので戻せない", f.rt.state.value.recordUndo)
        assertTrue(f.rt.records("workout").none { it.id == id })
    }

    @Test
    fun backgroundStopsAnUnfinishedCatchUp() = runTest {
        var calls = 0
        val f = fixture(remote = {
            calls++
            null
        })
        f.rt.bootstrap()
        settle()
        f.rt.setForeground(false)
        f.rt.setForeground(true)
        f.rt.setForeground(false) // 取り直しを始める前に背面へ
        settle()
        assertEquals("起動の 1 回だけ", 1, calls)
    }

    private companion object {
        const val APP = "test-app"
    }
}
