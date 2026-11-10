package jp.kz2601.metamon.runtime

import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.AppDocument
import jp.kz2601.metamon.renderer.navigation.NavigationState
import jp.kz2601.metamon.renderer.navigation.Route
import jp.kz2601.metamon.services.AppConfigClient
import jp.kz2601.metamon.services.FetchedDocument
import jp.kz2601.metamon.services.local.BackoffPolicy
import jp.kz2601.metamon.services.local.RecordStore
import jp.kz2601.metamon.services.local.RecordSyncEngine
import jp.kz2601.metamon.services.local.SyncQueue
import jp.kz2601.metamon.services.sync.DocumentSync
import jp.kz2601.metamon.services.sync.HotReload
import jp.kz2601.metamon.services.sync.SyncUpdate
import kotlinx.coroutines.channels.Channel
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.consumeAsFlow
import kotlinx.coroutines.test.TestScope
import kotlinx.coroutines.test.advanceTimeBy
import kotlinx.coroutines.test.runCurrent
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertSame
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.IOException
import java.lang.reflect.Proxy
import kotlin.coroutines.EmptyCoroutineContext

/** iOS の AppRuntime のホットリロード(startSync / hotReload / setForeground)と DocumentCache からのオフライン起動(#883)。 */
class HotReloadWiringTest {
    private val warikan = RuntimeFixture.dsl("warikan")
    private val kakeibo = RuntimeFixture.dsl("kakeibo")

    /** events を張るたびに新しい流れを作り、テストから流し込む。 */
    private class FakeSync : DocumentSync {
        val subscriptions = mutableListOf<Pair<String, Int>>()
        var channel = Channel<SyncUpdate>(Channel.UNLIMITED)

        override fun events(appId: String, afterVersion: Int): Flow<SyncUpdate> {
            subscriptions += appId to afterVersion
            channel = Channel(Channel.UNLIMITED)
            return channel.consumeAsFlow()
        }

        fun send(update: SyncUpdate) = check(channel.trySend(update).isSuccess)
    }

    /** listRecords の呼び出しを数える appconfig(他は通信断)。 */
    private class CountingClient {
        val pulls = mutableListOf<String>()
        val client: AppConfigClient = Proxy.newProxyInstance(
            AppConfigClient::class.java.classLoader,
            arrayOf(AppConfigClient::class.java),
        ) { _, method, args ->
            when (method.name) {
                "listRecords" -> {
                    pulls += args[1] as String
                    emptyList<Any>()
                }
                "toString" -> "countingClient"
                else -> throw IOException("offline")
            }
        } as AppConfigClient
    }

    private fun TestScope.runtime(
        dsl: AppDSL,
        sync: FakeSync = FakeSync(),
        client: AppConfigClient = RuntimeFixture.offlineClient,
        restore: (suspend (String) -> AppDocument)? = null,
        cached: (suspend (String) -> AppDocument?)? = null,
    ): AppRuntime {
        val store = RecordStore(null, EmptyCoroutineContext)
        val queue = SyncQueue(null, store, client, BackoffPolicy.immediate, sleep = {}, io = EmptyCoroutineContext)
        val engine = RecordSyncEngine(store, queue, client, RuntimeFixture.Offline(), backgroundScope)
        return AppRuntime(
            AppDocument(APP, 1, dsl), engine, backgroundScope,
            restore = restore,
            hotReloader = HotReload(sync, backgroundScope),
            cachedDocument = cached,
        )
    }

    /** backgroundScope の仕事は advanceUntilIdle では回らないので、時間を進めて流す(記録の取り直しのまとめ 250ms を越える)。 */
    private fun TestScope.settle() {
        advanceTimeBy(1_000)
        runCurrent()
    }

    private fun doc(version: Int, dsl: AppDSL, appId: String = APP) =
        SyncUpdate.Document(FetchedDocument(AppDocument(appId, version, dsl), "{}"))

    // DSL の差し替え

    @Test
    fun newerDslRedrawsTheScreen() = runTest {
        val sync = FakeSync()
        val rt = runtime(warikan, sync)
        rt.bootstrap()
        settle()
        assertEquals(listOf(APP to 1), sync.subscriptions)

        sync.send(doc(2, kakeibo))
        settle()

        assertSame(kakeibo, rt.state.value.dsl)
        assertEquals(2, rt.document.version)
        assertEquals(RuntimeCopy.HOT_RELOADED, rt.toast)
    }

    @Test
    fun olderOrOtherAppDslIsIgnored() = runTest {
        val rt = runtime(warikan)
        rt.bootstrap()
        settle()

        rt.hotReload(AppDocument(APP, 1, kakeibo))
        rt.hotReload(AppDocument("other", 5, kakeibo))

        assertSame(warikan, rt.dsl)
        assertNull(rt.toast)
    }

    @Test
    fun hotReloadWaitsWhileASheetIsOpen() = runTest {
        val sync = FakeSync()
        val rt = runtime(warikan, sync)
        rt.bootstrap()
        settle()
        val open = rt.navigation.value.copy(sheet = Route("add"))
        rt.setNavigation(open)

        sync.send(doc(2, kakeibo))
        sync.send(doc(3, kakeibo))
        settle()
        assertSame("入力中は描き直さない(下書きが消える)", warikan, rt.dsl)

        rt.setNavigation(open.copy(sheet = null))
        settle()
        assertSame(kakeibo, rt.dsl)
        assertEquals("最新の 1 件だけ当てる", 3, rt.document.version)
    }

    @Test
    fun hotReloadKeepsRecordsOfEntitiesThatStillExist() = runTest {
        val counting = CountingClient()
        val rt = runtime(warikan, client = counting.client)
        rt.bootstrap()
        settle()
        val loaded = rt.state.value.records.keys
        assertTrue(loaded.isNotEmpty())

        rt.hotReload(AppDocument(APP, 2, warikan))
        assertEquals(loaded, rt.state.value.records.keys)
        assertEquals(NavigationState.initial(warikan).selectedTab, rt.navigation.value.selectedTab)
    }

    // 記録の取り直し

    @Test
    fun recordsChangeRefetchesThatEntityOnce() = runTest {
        val sync = FakeSync()
        val counting = CountingClient()
        val rt = runtime(warikan, sync, client = counting.client)
        rt.bootstrap()
        settle()
        counting.pulls.clear()

        // 連投はまとめて 1 回
        sync.send(SyncUpdate.Records("expenses"))
        sync.send(SyncUpdate.Records("expenses"))
        // 表示していない entity は画面が開いた時に読むので取りに行かない
        sync.send(SyncUpdate.Records("unknown"))
        settle()

        assertEquals(listOf("expenses"), counting.pulls)
    }

    // 前面・背面

    @Test
    fun backgroundStopsAndForegroundResubscribesAndRefetches() = runTest {
        val sync = FakeSync()
        val counting = CountingClient()
        val rt = runtime(warikan, sync, client = counting.client)
        rt.bootstrap()
        settle()
        sync.send(doc(2, warikan))
        settle()
        val loaded = rt.state.value.records.keys
        counting.pulls.clear()

        val before = sync.subscriptions.size
        rt.setForeground(false)
        settle()
        assertFalse("背面では購読を止める", sync.channel.trySend(doc(3, kakeibo)).isSuccess)
        assertSame(warikan, rt.dsl)

        rt.setForeground(true)
        settle()

        assertEquals(before + 1, sync.subscriptions.size)
        assertEquals("戻ったら手元の版から張り直す", APP to 2, sync.subscriptions.last())
        assertEquals("背面の間の変化を読み直す", loaded, counting.pulls.toSet())
    }

    // オフライン起動

    @Test
    fun unreachableServerStartsFromCachedDsl() = runTest {
        val rt = runtime(
            warikan,
            restore = { throw IOException("offline") },
            cached = { AppDocument(it, 4, kakeibo) },
        )
        rt.bootstrap()
        settle()

        assertSame(kakeibo, rt.dsl)
        assertEquals(4, rt.document.version)
        assertTrue(rt.state.value.offline)
        assertEquals(RuntimeCopy.OFFLINE_RESTORED, rt.toast)
    }

    @Test
    fun olderCacheKeepsTheShownDsl() = runTest {
        val rt = runtime(
            warikan,
            restore = { throw IOException("offline") },
            cached = { AppDocument(it, 0, kakeibo) },
        )
        rt.bootstrap()
        settle()

        assertSame(warikan, rt.dsl)
        assertTrue(rt.state.value.offline)
        assertNull(rt.toast)
    }

    @Test
    fun reachableServerWinsOverCache() = runTest {
        val sync = FakeSync()
        val rt = runtime(
            warikan, sync,
            restore = { AppDocument(it, 5, kakeibo) },
            cached = { error("サーバーに届けばキャッシュは読まない") },
        )
        rt.bootstrap()
        settle()

        assertSame(kakeibo, rt.dsl)
        assertFalse(rt.state.value.offline)
        assertEquals("復元した版から購読する", APP to 5, sync.subscriptions.last())
    }

    @Test
    fun localAppIdDoesNotTalkToServer() = runTest {
        val sync = FakeSync()
        val store = RecordStore(null, EmptyCoroutineContext)
        val client = RuntimeFixture.offlineClient
        val queue = SyncQueue(null, store, client, BackoffPolicy.immediate, sleep = {}, io = EmptyCoroutineContext)
        val engine = RecordSyncEngine(store, queue, client, RuntimeFixture.Offline(), backgroundScope)
        val rt = AppRuntime(
            AppDocument(dsl = warikan), engine, backgroundScope,
            restore = { error("採番前は取りに行かない") },
            hotReloader = HotReload(sync, backgroundScope),
            cachedDocument = { error("採番前は読まない") },
        )
        rt.bootstrap()
        settle()

        assertTrue(sync.subscriptions.isEmpty())
        assertFalse(rt.state.value.offline)
    }

    private companion object {
        const val APP = "test-app"
    }
}
