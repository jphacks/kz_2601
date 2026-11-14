package jp.kz2601.metamon.desktop

import jp.kz2601.metamon.desktop.i18n.Lang
import jp.kz2601.metamon.desktop.i18n.Strings
import jp.kz2601.metamon.desktop.net.ClientPlatform
import jp.kz2601.metamon.desktop.net.DesktopApi
import jp.kz2601.metamon.desktop.store.DesktopStore
import jp.kz2601.metamon.desktop.store.JsonKeyValueStore
import jp.kz2601.metamon.desktop.store.MOVE_RETRIES
import jp.kz2601.metamon.desktop.store.SessionPersistence
import jp.kz2601.metamon.desktop.store.writeAtomically
import jp.kz2601.metamon.dsl.Action
import jp.kz2601.metamon.dsl.ActionType
import jp.kz2601.metamon.dsl.Presentation
import jp.kz2601.metamon.dsl.records.JSONValue
import jp.kz2601.metamon.dsl.records.entityModels
import jp.kz2601.metamon.services.live.HttpAppConfigClient
import jp.kz2601.metamon.services.local.BackoffPolicy
import jp.kz2601.metamon.services.local.LocalRecord
import jp.kz2601.metamon.services.local.PendingOp
import jp.kz2601.metamon.services.local.SyncEvent
import jp.kz2601.metamon.services.local.SyncOperation
import jp.kz2601.metamon.services.local.SyncQueue
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.CoroutineStart
import kotlinx.coroutines.Deferred
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.async
import kotlinx.coroutines.asCoroutineDispatcher
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.withTimeout
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Assert.fail
import org.junit.Rule
import org.junit.Test
import org.junit.rules.TemporaryFolder
import java.io.File
import java.nio.file.AccessDeniedException
import java.util.concurrent.CountDownLatch
import java.util.concurrent.Executors
import java.util.concurrent.TimeUnit

/** 同期・複数のウィンドウの見直し(#1908 の続き): 止まった送り待ち・断られた時の混ぜ方・消えた記録・id・終わる時・閉じる・すぐ記録・JSON・後片付け。 */
class SyncWindowsFixTest {
    @get:Rule
    val tmp = TemporaryFolder()

    private val dsl = Examples.load("shopping")
    private val models = dsl.entityModels()
    private val oneTry = BackoffPolicy(baseMillis = 0, multiplier = 1.0, maxAttempts = 1)
    private val add = Action(type = ActionType.NAVIGATE, screen = "item_form", presentation = Presentation.SHEET)
    private val save = Action(type = ActionType.SAVE_RECORD)
    private val servers = mutableListOf<MockWebServer>()

    @After
    fun tearDown() = servers.forEach { runCatching { it.shutdown() } }

    private fun server(vararg responses: MockResponse) = MockWebServer().apply {
        responses.forEach(::enqueue)
        start()
        servers += this
    }

    private fun created(id: String, name: String) =
        MockResponse().setHeader("Content-Type", "application/json").setBody("""{"id":"$id","entity":"groceries","data":{"name":"$name"}}""")

    private fun client(s: MockWebServer) = HttpAppConfigClient(DesktopApi.http(s.url("/").toString(), DesktopApi.devTokens(null), ClientPlatform.LINUX))

    private fun DesktopSession.addItem(name: String) {
        dispatch(add, ActionContext(top, null))
        val form = form(top)!!
        form.values["name"] = JSONValue.Str(name)
        dispatch(save, ActionContext(top, form))
    }

    /** 列の知らせを、起きる前から待つ。 */
    private fun CoroutineScope.next(q: SyncQueue, match: (SyncEvent) -> Boolean): Deferred<SyncEvent> =
        async(start = CoroutineStart.UNDISPATCHED) { q.events.first(match) }

    private suspend fun <T> Deferred<T>.within(): T = withTimeout(5_000) { await() }

    // MARK: - 1. 止まった送り待ちを起こす

    @Test
    fun pausedQueueResumesOnLocalChangeAndOnFocus() = runBlocking {
        val s = server(MockResponse().setResponseCode(503), created("s1", "牛乳"), MockResponse().setResponseCode(503), created("s2", "卵"))
        val p = SessionPersistence(DesktopStore(tmp.newFolder()), "a1", client(s), policy = oneTry, retryEveryMillis = null)
        val q = p.queue!!
        val session = DesktopSession(dsl, p.load(models), persistence = p)

        val paused = next(q) { it is SyncEvent.Paused }
        session.addItem("牛乳")
        paused.within()
        assertTrue(q.isPaused)

        // 画面で書き換えたら起こして送る(前は止まったまま送らなかった)
        val drained = next(q) { it == SyncEvent.Drained }
        val id = session.store.list("groceries").single().id
        assertTrue(session.updateRecord("groceries", id, mapOf("done" to "true")))
        drained.within()
        assertFalse(q.isPaused)
        assertEquals(0, q.count())

        // ウィンドウが前に出た時も起こす
        val pausedAgain = next(q) { it is SyncEvent.Paused }
        session.addItem("卵")
        pausedAgain.within()
        val drainedAgain = next(q) { it == SyncEvent.Drained }
        p.resume()!!.join()
        drainedAgain.within()
        assertEquals(0, q.count())
        assertEquals(4, s.requestCount)
    }

    @Test
    fun pausedQueueResumesOnTheTimer() = runBlocking {
        val s = server(MockResponse().setResponseCode(503), created("s1", "牛乳"))
        val p = SessionPersistence(DesktopStore(tmp.newFolder()), "a1", client(s), policy = oneTry, retryEveryMillis = 100)
        val q = p.queue!!
        val paused = next(q) { it is SyncEvent.Paused }
        DesktopSession(dsl, p.load(models), persistence = p).addItem("牛乳")
        paused.within()
        // 何もしなくても間隔ごとに起こす
        next(q) { it == SyncEvent.Drained }.within()
        assertEquals(0, q.count())
        p.shutdown()
    }

    // MARK: - 5. 起こしの取りこぼし(SyncQueue.drain の lost wake-up)を間隔の起こしで拾う

    @Test
    fun opLeftBehindByALostWakeUpIsSentByTheTimer() = runBlocking {
        val s = server(created("s1", "牛乳"))
        val p = SessionPersistence(DesktopStore(tmp.newFolder()), "a1", client(s), policy = oneTry, retryEveryMillis = 50)
        val q = p.queue!!
        val drained = next(q) { it == SyncEvent.Drained }
        // 積んだが誰も drain しなかった(drain が終わりかけで isDraining のまま帰った時と同じ)
        q.enqueue(SyncOperation(appId = "a1", entity = "groceries", recordId = LocalRecord.temporaryId("c1"), kind = PendingOp.CREATE, data = JsonObject(mapOf("name" to JsonPrimitive("牛乳")))))
        drained.within()
        assertEquals(0, q.count())
        assertEquals("POST /v1/apps/a1/entities/groceries/records", s.takeRequest(5, TimeUnit.SECONDS)!!.let { "${it.method} ${it.path}" })
        p.shutdown()
    }

    // MARK: - 2. 断られた時: 指した行だけを画面のスレッドで混ぜる・件数はアプリごと

    @Test
    fun refusalMergesOnlyTheNamedRowOnTheUiThread() = runBlocking {
        val s = server(
            MockResponse().setResponseCode(400).setHeader("Content-Type", "application/json").setBody("""{"error":"bad_request","message":"だめ"}"""),
            created("s2", "卵"),
        )
        val store = DesktopStore(tmp.newFolder())
        val pa = SessionPersistence(store, "a1", client(s), policy = oneTry, retryEveryMillis = null)
        val pb = SessionPersistence(store, "b1", client(s), policy = oneTry, retryEveryMillis = null)
        val q = pa.queue!!
        val posted = mutableListOf<() -> Unit>()
        val got = CompletableDeferred<Unit>()
        val records = SharedRecords(pa.load(models), pa, post = { posted += it; got.complete(Unit) })
        var bReloaded = false
        pb.onReload = { bReloaded = true }
        val session = DesktopSession(dsl, shared = records)

        val sentEgg = next(q) { it is SyncEvent.Sent }
        session.addItem("牛乳")
        got.within()
        // 裏のスレッドでは書き換えない(画面のスレッドに頼んだだけ)
        assertEquals(listOf("牛乳"), records.store.list("groceries").map { it.data["name"] })

        // 混ぜる前に画面で足した行は、混ぜても消えない
        session.addItem("卵")
        sentEgg.within()
        pa.idle()
        assertEquals(1, posted.size)
        posted.forEach { it() }
        assertEquals(listOf("卵"), records.store.list("groceries").map { it.data["name"] })
        assertFalse("他のアプリは読み直さない", bReloaded)

        // 送り待ちの件数はアプリごと
        q.enqueue(SyncOperation(appId = "b1", entity = "groceries", recordId = "r1", kind = PendingOp.UPDATE))
        pa.load(models)
        pb.load(models)
        assertEquals(0, pa.pending)
        assertEquals(1, pb.pending)
    }

    // MARK: - 3. 他のウィンドウで消された記録

    @Test
    fun staleDetailIsRefusedAndClosesWhenTheRecordIsDeletedElsewhere() = Strings.withLang(Lang.JA) {
        val app = AppHub().app("shopping")
        val list = DesktopSession(app.dsl, shared = app.records)
        list.addItem("牛乳")
        val id = list.records("groceries").single().id
        val detail = DesktopSession(app.dsl, shared = app.records, startScreen = "item_form", startRecordId = id)
        var closed = 0
        detail.onCloseWindow = { closed++ }
        val form = detail.form(detail.top)!!
        form.values["name"] = JSONValue.Str("豆乳")

        list.dispatch(Action(type = ActionType.DELETE_RECORD, entity = "groceries"), ActionContext(list.top, null, id))

        // 開いていた下書きで保存しても、新しい記録として生き返らない
        detail.dispatch(save, ActionContext(detail.top, form))
        assertEquals("この記録は別のウィンドウで消されました", detail.message)
        assertTrue(list.store.list("groceries").isEmpty())
        // 下書きは捨て、保存できない形になる
        val after = detail.form(detail.top)!!
        assertTrue(after.isReadOnly)
        assertNull(after.values["name"]?.stringValue?.takeIf { it == "豆乳" })
        // 詳細のウィンドウは閉じる
        detail.followRemoval()
        assertEquals(1, closed)
    }

    @Test
    fun appWindowLeavesADeletedRecordScreen() = Strings.withLang(Lang.JA) {
        val app = AppHub().app("shopping")
        val a = DesktopSession(app.dsl, shared = app.records)
        a.addItem("牛乳")
        val id = a.records("groceries").single().id
        val b = DesktopSession(app.dsl, shared = app.records)
        b.dispatch(Action(type = ActionType.NAVIGATE, screen = "item_form"), ActionContext(b.top, null, id))
        assertEquals("item_form", b.top.screen)
        b.form(b.top)
        a.dispatch(Action(type = ActionType.DELETE_RECORD, entity = "groceries"), ActionContext(a.top, null, id))
        b.followRemoval()
        assertTrue(b.top.screen != "item_form")
        assertEquals("この記録は別のウィンドウで消されました", b.message)
    }

    // MARK: - 4. 送れた後に読み直しても画面の id は変わらない

    @Test
    fun syncedRecordKeepsItsScreenIdAfterReload() = runBlocking {
        val dir = tmp.newFolder()
        val s = server(created("s1", "牛乳"), created("s1", "豆乳"))
        val first = SessionPersistence(DesktopStore(dir), "a1", client(s), policy = oneTry, retryEveryMillis = null)
        val drained = next(first.queue!!) { it == SyncEvent.Drained }
        val session = DesktopSession(dsl, first.load(models), persistence = first)
        session.addItem("牛乳")
        val temp = session.store.list("groceries").single().id
        drained.within()
        first.idle()
        assertEquals("s1", DesktopStore(dir).records.rows("a1", "groceries").single().recordId)

        // 読み直し(再起動)でも仮 id のまま: 開いているウィンドウ・遷移が同じ行を指し続ける
        val again = SessionPersistence(DesktopStore(dir), "a1", client(s), policy = oneTry, retryEveryMillis = null)
        val reopened = DesktopSession(dsl, again.load(models), persistence = again)
        assertEquals(listOf(temp), reopened.store.list("groceries").map { it.id })

        // 書き換えはサーバーの id に送る
        val sent = next(again.queue!!) { it is SyncEvent.Sent }
        assertTrue(reopened.updateRecord("groceries", temp, mapOf("name" to "豆乳")))
        sent.within()
        s.takeRequest(5, TimeUnit.SECONDS)
        val put = s.takeRequest(5, TimeUnit.SECONDS)!!
        assertEquals("/v1/apps/a1/entities/groceries/records/s1", put.path)
        again.idle()
        assertEquals(listOf(temp), again.load(models).list("groceries").map { it.id })
    }

    // MARK: - 6. 終わる時に積んだ書き込みを書き終える

    @Test
    fun hubCloseWaitsForQueuedWrites() = runBlocking {
        val dir = tmp.newFolder()
        val writer = Executors.newSingleThreadExecutor()
        val gate = CountDownLatch(1)
        writer.execute { gate.await(5, TimeUnit.SECONDS) } // 書き込みの列を少しふさぐ
        val p = SessionPersistence(DesktopStore(dir), "shopping", client = null, scope = CoroutineScope(SupervisorJob() + writer.asCoroutineDispatcher()))
        val hub = AppHub(openRecords = { _, _ -> SharedRecords(MockStore(), p) })
        val app = hub.app("shopping")
        DesktopSession(app.dsl, shared = app.records).addItem("牛乳")
        Thread { Thread.sleep(200); gate.countDown() }.start()

        hub.close()
        assertEquals(1, DesktopStore(dir).records.rows("shopping", "groceries").size)
        writer.shutdown()
    }

    // MARK: - 7. 画面の中の「閉じる」・Esc は終わらせない

    @Test
    fun inAppCloseOfTheLastDetailOpensTheAppWindowFirst() {
        val r = WindowRegistry()
        val detail = r.open(WindowKey.record("shopping", "item_form", "new-1"))
        r.closeInApp(detail.id)
        assertEquals(listOf(WindowKey.app("shopping")), r.windows.map { it.key })

        // 他にウィンドウがあれば閉じるだけ
        val again = r.open(WindowKey.record("shopping", "item_form", "new-1"))
        r.closeInApp(again.id)
        assertEquals(listOf(WindowKey.app("shopping")), r.windows.map { it.key })

        // アプリのウィンドウの最後の 1 枚は閉じない(× だけがトレイ・終わるへ)
        r.closeInApp(r.windows.single().id)
        assertEquals(1, r.windows.size)
    }

    // MARK: - 8. すぐ記録はアプリのウィンドウへ

    @Test
    fun quickRecordGoesToTheLatestAppWindowOfTheActiveApp() {
        val r = WindowRegistry()
        val older = r.open(WindowKey.app("shopping"))
        r.open(WindowKey.app("kakeibo"))
        val newer = r.open(WindowKey.app("shopping"))
        assertEquals(older.id, newer.id) // 同じアプリは 1 枚
        val detail = r.open(WindowKey.record("shopping", "item_form", "new-1"))
        assertEquals(detail.id, r.active?.id)
        assertEquals(older.id, r.quickRecordWindow()?.id)

        // アプリのウィンドウが無ければ開く
        val only = WindowRegistry()
        only.open(WindowKey.record("kakeibo", "expense_form", "new-1"))
        val opened = only.quickRecordWindow()!!
        assertEquals(WindowKey.app("kakeibo"), opened.key)

        val c = TrayController(Examples.load("kakeibo"), true, MemoryBoolSetting(true), MemoryBoolSetting(true))
        c.findQuickRecordWindow = { 7 }
        c.quickRecord()
        assertFalse("詳細(別のウィンドウ)は受け取らない", c.takeQuickRecord(3) { true })
        assertTrue(c.takeQuickRecord(7) { false })
        assertFalse("1 回だけ", c.takeQuickRecord(7) { true })
    }

    // MARK: - 9. JSON を壊さない

    @Test
    fun atomicWriteRetriesAccessDeniedAndKeepsCorruptFiles() {
        val dir = tmp.newFolder()
        val f = File(dir, "kv.json")
        var denied = 2
        val pauses = mutableListOf<Long>()
        writeAtomically(f, "{}", move = { from, to ->
            if (denied-- > 0) throw AccessDeniedException(to.toString())
            java.nio.file.Files.move(from, to, java.nio.file.StandardCopyOption.REPLACE_EXISTING)
        }, pause = { pauses += it })
        assertEquals("{}", f.readText())
        assertEquals(2, pauses.size)
        try {
            writeAtomically(f, "{\"a\":1}", move = { _, to -> throw AccessDeniedException(to.toString()) }, pause = {})
            fail("試し直しは $MOVE_RETRIES 回まで")
        } catch (_: AccessDeniedException) {
        }
        assertEquals("{}", f.readText())

        // 読めないファイルは上書きせず .corrupt に退ける
        f.writeText("{壊れた")
        val kv = JsonKeyValueStore(f)
        assertTrue(kv.keys().isEmpty())
        assertEquals("{壊れた", File(dir, "kv.json.corrupt").readText())
        kv.putString("s", "x")
        assertEquals("x", JsonKeyValueStore(f).getString("s"))
        assertEquals("{壊れた", File(dir, "kv.json.corrupt").readText())
    }

    // MARK: - 10. 最後のウィンドウを閉じたらアプリの書き込みを閉じる

    @Test
    fun lastWindowReleasesTheAppsPersistence() = runBlocking {
        val dir = tmp.newFolder()
        val opened = mutableListOf<SessionPersistence>()
        val hub = AppHub(openRecords = { id, d ->
            val p = SessionPersistence(DesktopStore(dir), id, client = null)
            opened += p
            SharedRecords(runBlocking { p.load(d.entityModels()) }, p)
        })
        val a = hub.acquire("shopping")
        hub.acquire("shopping")
        DesktopSession(a.dsl, shared = a.records).addItem("牛乳")
        hub.release("shopping")
        assertEquals(listOf("shopping"), hub.opened().map { it.id })
        assertFalse(opened.single().isShutDown)

        hub.release("shopping")
        assertTrue(hub.opened().isEmpty())
        assertTrue(opened.single().isShutDown)
        assertEquals(1, DesktopStore(dir).records.rows("shopping", "groceries").size)

        // 開き直すと端末から読み直す
        val b = hub.acquire("shopping")
        assertEquals(2, opened.size)
        assertEquals(listOf("牛乳"), b.records.store.list("groceries").map { it.data["name"] })
        assertNotNull(hub.dsl("kakeibo"))
        assertTrue("DSL だけでは記録を開かない", hub.opened().none { it.id == "kakeibo" })
    }
}
