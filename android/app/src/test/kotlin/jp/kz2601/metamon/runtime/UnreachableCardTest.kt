package jp.kz2601.metamon.runtime

import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.AppDocument
import jp.kz2601.metamon.dsl.decode
import jp.kz2601.metamon.services.local.BackoffPolicy
import jp.kz2601.metamon.services.local.ConnectivityMonitor
import jp.kz2601.metamon.services.local.CurrentAppStore
import jp.kz2601.metamon.services.local.RecordStore
import jp.kz2601.metamon.services.local.RecordSyncEngine
import jp.kz2601.metamon.services.local.SyncQueue
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.test.TestScope
import kotlinx.coroutines.test.advanceTimeBy
import kotlinx.coroutines.test.runCurrent
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.IOException
import kotlin.coroutines.EmptyCoroutineContext

/**
 * 起動時にサーバーへ届かず保存した DSL も無い時の状態カード(#1107・iOS の unreachable)。
 * つながったら(Connectivity が online に戻ったら)サーバーの最新を 1 回取り直し、取れたらカードを下げる。
 */
class UnreachableCardTest {
    private val kakeibo = RuntimeFixture.dsl("kakeibo")
    private val voice = AppDSL.decode(
        """{"schemaVersion":1,"app":{"name":"Tailor It","voiceEntry":"fullscreen"},"initialScreen":"voice",
           "screens":[{"id":"voice","components":[{"type":"voiceInput"}]}]}""",
    )

    private class Net(online: Boolean) : ConnectivityMonitor {
        override val online = MutableStateFlow(online)
    }

    private class Server {
        var reachable = false
        var fetches = 0

        suspend fun fetch(appId: String): AppDocument {
            fetches++
            if (!reachable) throw IOException("offline")
            return AppDocument(appId, 4, RuntimeFixture.dsl("kakeibo"))
        }
    }

    private class Setup(val rt: AppRuntime, val net: Net, val server: Server)

    private fun TestScope.setup(cache: Map<String, AppDocument> = emptyMap()): Setup {
        val client = RuntimeFixture.offlineClient
        val records = RecordStore(null, EmptyCoroutineContext)
        val queue = SyncQueue(null, records, client, BackoffPolicy.immediate, sleep = {}, io = EmptyCoroutineContext)
        val net = Net(online = false)
        val engine = RecordSyncEngine(records, queue, client, net, backgroundScope)
        val server = Server()
        val store = CurrentAppStore(null).apply { set("kakeibo-app") }
        val rt = AppRuntime(
            AppDocument(store.current, 0, voice), engine, backgroundScope,
            restore = server::fetch,
            cachedDocument = { cache[it] },
        )
        return Setup(rt, net, server)
    }

    private fun TestScope.settle() {
        advanceTimeBy(1_000)
        runCurrent()
    }

    @Test
    fun offlineWithoutSavedAppShowsTheCard() = runTest {
        val s = setup()

        s.rt.bootstrap()
        settle()

        assertTrue(s.rt.state.value.offline)
        assertTrue("保存した DSL が無いので状態カードを出す", s.rt.state.value.unreachable)
        assertNull("トーストではなくカード", s.rt.toast)
    }

    @Test
    fun offlineWithSavedAppShowsTheToastNotTheCard() = runTest {
        val s = setup(cache = mapOf("kakeibo-app" to AppDocument("kakeibo-app", 4, kakeibo)))

        s.rt.bootstrap()
        settle()

        assertTrue(s.rt.state.value.offline)
        assertFalse(s.rt.state.value.unreachable)
        assertEquals(RuntimeCopy.OFFLINE_RESTORED, s.rt.toast)
    }

    @Test
    fun reconnectRestoresOnceAndHidesTheCard() = runTest {
        val s = setup()
        s.rt.bootstrap()
        settle()
        assertEquals(1, s.server.fetches)

        s.server.reachable = true
        s.net.online.value = true
        settle()

        assertEquals("つながったら 1 回だけ取り直す", 2, s.server.fetches)
        assertFalse(s.rt.state.value.unreachable)
        assertFalse(s.rt.state.value.offline)
        assertEquals(kakeibo.app.name, s.rt.dsl.app.name)

        // つながったまま・もう一度切れてつながっても、オフラインでなければ取り直さない
        s.net.online.value = false
        settle()
        s.net.online.value = true
        settle()
        assertEquals(2, s.server.fetches)
    }

    @Test
    fun reconnectWhileServerStillDownKeepsTheCard() = runTest {
        val s = setup()
        s.rt.bootstrap()
        settle()

        s.net.online.value = true
        settle()

        assertEquals(2, s.server.fetches)
        assertTrue(s.rt.state.value.unreachable)
    }
}
