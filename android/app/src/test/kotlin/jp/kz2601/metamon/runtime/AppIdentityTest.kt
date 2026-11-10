package jp.kz2601.metamon.runtime

import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.AppDocument
import jp.kz2601.metamon.dsl.decode
import jp.kz2601.metamon.services.AppSummary
import jp.kz2601.metamon.services.local.AppListCache
import jp.kz2601.metamon.services.local.BackoffPolicy
import jp.kz2601.metamon.services.local.CurrentAppStore
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
import org.junit.Assert.assertSame
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.IOException
import kotlin.coroutines.EmptyCoroutineContext

/**
 * appId が "local" のままにならないこと(#881・android-110)。保存した appId で起動し直し、
 * 生成・切り替え・新しいアプリで appId が変わったら保存して、ホットリロード(#1064)を張り直す。
 */
class AppIdentityTest {
    private val warikan = RuntimeFixture.dsl("warikan")
    private val kakeibo = RuntimeFixture.dsl("kakeibo")
    private val voice = AppDSL.decode(
        """{"schemaVersion":1,"app":{"name":"Tailor It","voiceEntry":"fullscreen"},"initialScreen":"voice",
           "screens":[{"id":"voice","components":[{"type":"voiceInput"}]}]}""",
    )

    private class FakeSync : DocumentSync {
        val subscriptions = mutableListOf<Pair<String, Int>>()
        var channel = Channel<SyncUpdate>(Channel.UNLIMITED)

        override fun events(appId: String, afterVersion: Int): Flow<SyncUpdate> {
            subscriptions += appId to afterVersion
            channel = Channel(Channel.UNLIMITED)
            return channel.consumeAsFlow()
        }
    }

    private class Server {
        val docs = mutableMapOf<String, AppDocument>()
        var apps: List<AppSummary> = emptyList()
        var online = true
        val fetched = mutableListOf<String>()

        suspend fun fetch(appId: String): AppDocument {
            fetched += appId
            if (!online) throw IOException("offline")
            return docs[appId] ?: throw IOException("404")
        }

        suspend fun list(): List<AppSummary> = if (online) apps else throw IOException("offline")
    }

    private fun minting(vararg ids: String): () -> String {
        val queue = ArrayDeque(ids.toList())
        return { queue.removeFirst() }
    }

    private class Setup(val rt: AppRuntime, val store: CurrentAppStore, val sync: FakeSync, val cache: MutableMap<String, AppDocument>)

    private fun TestScope.setup(
        store: CurrentAppStore,
        server: Server,
        cache: MutableMap<String, AppDocument> = mutableMapOf(),
        appCache: AppListCache = AppListCache(null),
    ): Setup {
        val client = RuntimeFixture.offlineClient
        val records = RecordStore(null, EmptyCoroutineContext)
        val queue = SyncQueue(null, records, client, BackoffPolicy.immediate, sleep = {}, io = EmptyCoroutineContext)
        val engine = RecordSyncEngine(records, queue, client, RuntimeFixture.Offline(), backgroundScope)
        val sync = FakeSync()
        // AppContainer と同じ: 保存した appId・話しかける前の画面(version 0)で始める
        val rt = AppRuntime(
            AppDocument(store.current, 0, voice), engine, backgroundScope,
            restore = server::fetch,
            hotReloader = HotReload(sync, backgroundScope),
            cachedDocument = { cache[it] },
            directory = AppDirectory(store, server::list, appCache, fresh = { AppDocument(it, 0, voice) }),
        )
        return Setup(rt, store, sync, cache)
    }

    private fun TestScope.settle() {
        advanceTimeBy(1_000)
        runCurrent()
    }

    @Test
    fun relaunchReopensThePreviousAppFromTheServerAndHotReloadsIt() = runTest {
        val store = CurrentAppStore(null, minting("unused")).apply { set("kakeibo-app") }
        val server = Server().apply { docs["kakeibo-app"] = AppDocument("kakeibo-app", 4, kakeibo) }
        val s = setup(store, server)

        s.rt.bootstrap()
        settle()

        assertEquals(listOf("kakeibo-app"), server.fetched)
        assertSame(kakeibo, s.rt.dsl)
        assertEquals(listOf("kakeibo-app" to 4), s.sync.subscriptions)
        assertFalse(s.rt.state.value.offline)
    }

    @Test
    fun offlineRelaunchShowsTheSavedDslAndStillSubscribes() = runTest {
        val store = CurrentAppStore(null).apply { set("kakeibo-app") }
        val server = Server().apply { online = false }
        val s = setup(store, server, cache = mutableMapOf("kakeibo-app" to AppDocument("kakeibo-app", 4, kakeibo)))

        s.rt.bootstrap()
        settle()

        assertSame("保存した DSL を出す", kakeibo, s.rt.dsl)
        assertTrue(s.rt.state.value.offline)
        assertEquals(RuntimeCopy.OFFLINE_RESTORED, s.rt.toast)
        assertEquals("つながったら追いつけるよう購読は張る", listOf("kakeibo-app" to 4), s.sync.subscriptions)
    }

    @Test
    fun generatedDocumentWithANewAppIdIsSavedAndResubscribed() = runTest {
        val store = CurrentAppStore(null, minting("first"))
        val server = Server()
        val s = setup(store, server)
        s.rt.bootstrap()
        settle()
        assertEquals("first", store.current)

        s.rt.adoptGenerated(AppDocument("generated-app", 2, warikan))
        settle()

        assertEquals("generated-app", store.current)
        assertSame(warikan, s.rt.dsl)
        assertEquals("generated-app" to 2, s.sync.subscriptions.last())

        // 同じアプリの古い版(ホットリロードが先に当てた後に届いた)は捨てる
        s.rt.hotReload(AppDocument("generated-app", 3, kakeibo))
        s.rt.adoptGenerated(AppDocument("generated-app", 2, warikan))
        assertSame(kakeibo, s.rt.dsl)
    }

    @Test
    fun switchAppOpensTheChosenAppAndKeepsItForTheNextLaunch() = runTest {
        val store = CurrentAppStore(null).apply { set("a") }
        val server = Server().apply {
            docs["a"] = AppDocument("a", 3, warikan)
            docs["b"] = AppDocument("b", 7, kakeibo)
        }
        val s = setup(store, server)
        s.rt.bootstrap()
        settle()

        assertTrue(s.rt.switchApp("b"))
        settle()

        assertSame(kakeibo, s.rt.dsl)
        assertEquals("b", store.current)
        assertEquals("b" to 7, s.sync.subscriptions.last())
        assertEquals(RuntimeCopy.switched(kakeibo.app.name, offline = false), s.rt.toast)
    }

    @Test
    fun switchAppFallsBackToTheSavedDslAndFailsWithoutIt() = runTest {
        val store = CurrentAppStore(null).apply { set("a") }
        val server = Server().apply { docs["a"] = AppDocument("a", 3, warikan) }
        val s = setup(store, server, cache = mutableMapOf("b" to AppDocument("b", 7, kakeibo)))
        s.rt.bootstrap()
        settle()
        server.online = false

        assertFalse(s.rt.switchApp("c"))
        assertEquals("a", store.current)
        assertEquals(RuntimeCopy.SWITCH_FAILED, s.rt.toast)

        assertTrue(s.rt.switchApp("b"))
        assertSame(kakeibo, s.rt.dsl)
        assertTrue(s.rt.state.value.offline)
        assertEquals("b", store.current)
    }

    @Test
    fun newAppRotatesTheAppIdAndShowsTheVoiceScreen() = runTest {
        val store = CurrentAppStore(null, minting("next")).apply { set("a") }
        val server = Server().apply { docs["a"] = AppDocument("a", 3, warikan) }
        val s = setup(store, server)
        s.rt.bootstrap()
        settle()

        s.rt.startNewApp()
        settle()

        assertEquals("next", store.current)
        assertEquals("next", s.rt.document.appId)
        assertSame(voice, s.rt.dsl)
        assertEquals("next" to 0, s.sync.subscriptions.last())
    }

    @Test
    fun freshInstallOpensTheLatestServerApp() = runTest {
        val store = CurrentAppStore(null, minting("fresh-install"))
        val server = Server().apply {
            docs["fresh-install"] = AppDocument("fresh-install", 1, voice)
            docs["mine"] = AppDocument("mine", 5, kakeibo)
            apps = listOf(
                AppSummary("fresh-install", "", 1),
                AppSummary("mine", "家計簿", 5, updatedAt = "2026-11-10T00:00:00Z"),
            )
        }
        val s = setup(store, server)
        s.rt.bootstrap()
        settle()

        assertEquals("mine", store.current)
        assertSame(kakeibo, s.rt.dsl)
    }

    @Test
    fun bootstrapAgainAfterRecreationKeepsTheNewApp() = runTest {
        val store = CurrentAppStore(null, minting("fresh-install", "brand-new"))
        val server = Server().apply {
            docs["fresh-install"] = AppDocument("fresh-install", 1, voice)
            docs["mine"] = AppDocument("mine", 5, kakeibo)
            apps = listOf(AppSummary("mine", "家計簿", 5, updatedAt = "2026-11-10T00:00:00Z"))
        }
        val s = setup(store, server)
        s.rt.bootstrap()
        settle()
        s.rt.startNewApp()
        settle()
        val fetches = server.fetched.size

        // 画面の作り直し(回転など)で LaunchedEffect がもう一度 bootstrap を呼ぶ
        s.rt.bootstrap()
        settle()

        assertEquals("最近のアプリへ戻さない", "brand-new", s.rt.document.appId)
        assertSame(voice, s.rt.dsl)
        assertEquals("brand-new", store.current)
        assertEquals("取得を重ねない", fetches, server.fetched.size)
    }

    @Test
    fun appsListFallsBackToTheSavedList() = runTest {
        val cache = AppListCache(null)
        val store = CurrentAppStore(null).apply { set("a") }
        val server = Server().apply { apps = listOf(AppSummary("a", "割り勘", 3), AppSummary("b", "家計簿", 4)) }
        val s = setup(store, server, appCache = cache)

        assertEquals(2, s.rt.appsList().size)
        server.online = false
        assertEquals(listOf("a", "b"), s.rt.appsList().map { it.id })
        assertEquals(RuntimeCopy.APPS_OFFLINE, s.rt.toast)
        assertEquals(listOf("a", "b"), s.rt.visibleApps.map { it.id })
    }
}
