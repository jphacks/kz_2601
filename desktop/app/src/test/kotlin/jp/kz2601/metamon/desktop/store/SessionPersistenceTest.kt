package jp.kz2601.metamon.desktop.store

import jp.kz2601.metamon.desktop.ActionContext
import jp.kz2601.metamon.desktop.DesktopSession
import jp.kz2601.metamon.desktop.Examples
import jp.kz2601.metamon.desktop.MockStore
import jp.kz2601.metamon.desktop.Row
import jp.kz2601.metamon.services.http.PlanLimit
import jp.kz2601.metamon.services.http.ProRequired
import jp.kz2601.metamon.services.http.RecordWriteDenial
import jp.kz2601.metamon.services.local.SyncEvent
import kotlinx.serialization.json.add
import kotlinx.serialization.json.buildJsonArray
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.put
import jp.kz2601.metamon.desktop.net.ClientPlatform
import jp.kz2601.metamon.desktop.net.DesktopApi
import jp.kz2601.metamon.desktop.net.DesktopApiConfig
import jp.kz2601.metamon.dsl.Action
import jp.kz2601.metamon.dsl.ActionType
import jp.kz2601.metamon.dsl.Presentation
import jp.kz2601.metamon.dsl.records.JSONValue
import jp.kz2601.metamon.dsl.records.entityModels
import jp.kz2601.metamon.services.live.HttpAppConfigClient
import jp.kz2601.metamon.services.local.BackoffPolicy
import jp.kz2601.metamon.services.local.LocalRecord
import jp.kz2601.metamon.services.local.PendingOp
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancelAndJoin
import kotlinx.coroutines.job
import kotlinx.coroutines.runBlocking
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.rules.TemporaryFolder
import java.util.concurrent.TimeUnit

/** 画面と端末の保存をつなぐ(#1887): 再起動をまたいだ記録・送り待ちの送り直し・消す。通信は MockWebServer だけ。 */
class SessionPersistenceTest {
    @get:Rule
    val tmp = TemporaryFolder()

    private val dsl = Examples.load("shopping")
    private val models = dsl.entityModels()
    private val noWait = BackoffPolicy(baseMillis = 0, multiplier = 1.0, maxAttempts = 1)
    private val add = Action(type = ActionType.NAVIGATE, screen = "item_form", presentation = Presentation.SHEET)
    private val save = Action(type = ActionType.SAVE_RECORD)

    private fun client(url: String) = HttpAppConfigClient(DesktopApi.http(url, DesktopApi.devTokens(null), ClientPlatform.LINUX))

    private fun session(p: SessionPersistence) = DesktopSession(dsl, runBlocking { p.load(models) }, persistence = p)

    /** 画面から 1 件入れて保存する。 */
    private fun DesktopSession.addItem(name: String) {
        dispatch(add, ActionContext(top, null))
        val form = form(top)!!
        form.values["name"] = JSONValue.Str(name)
        dispatch(save, ActionContext(top, form))
    }

    @Test
    fun recordsSurviveANewSession() = runBlocking {
        val dir = tmp.newFolder()
        val p = SessionPersistence(DesktopStore(dir), "shopping", client = null)
        assertFalse("既定はモック(送り先なし)", p.isLive)
        val s = session(p)
        s.addItem("牛乳")
        s.addItem("卵")
        val milk = s.store.list("groceries").first { it.data["name"] == "牛乳" }.id
        assertTrue(milk.startsWith(LocalRecord.TEMP_PREFIX))
        assertTrue(s.updateRecord("groceries", milk, mapOf("done" to "true")))
        val egg = s.store.list("groceries").first { it.data["name"] == "卵" }.id
        s.dispatch(Action(type = ActionType.DELETE_RECORD, entity = "groceries"), ActionContext(s.top, null, egg))
        p.idle()

        // 再起動 = 新しい DesktopStore と新しい画面
        val again = session(SessionPersistence(DesktopStore(dir), "shopping", client = null))
        val rows = again.store.list("groceries")
        assertEquals(listOf(milk), rows.map { it.id })
        assertEquals("牛乳", rows.single().data["name"])
        assertEquals("true", rows.single().data["done"])
        // 型は DSL どおりに残す(boolean は真偽・integer は数)
        val saved = DesktopStore(dir).records.rows("shopping", "groceries").single()
        assertEquals(JsonPrimitive(true), saved.data["done"])
        assertEquals(PendingOp.NONE, saved.pendingOp)
        assertEquals(1, again.records("groceries").size)
    }

    @Test
    fun queuedWritesAreResentOnStartup() = runBlocking {
        val dir = tmp.newFolder()
        val dead = MockWebServer().apply { start() }
        val deadUrl = dead.url("/").toString()
        dead.shutdown()

        // 圏外で保存: 端末に残り、送り待ちに積まれる
        val offlineScope = CoroutineScope(SupervisorJob() + Dispatchers.IO)
        val offline = SessionPersistence(DesktopStore(dir), "a1", client(deadUrl), policy = noWait, scope = offlineScope)
        assertTrue(offline.isLive)
        session(offline).addItem("牛乳")
        offline.idle()
        offlineScope.coroutineContext.job.cancelAndJoin() // アプリを終える
        val local = DesktopStore(dir).records.rows("a1", "groceries").single()
        assertEquals(PendingOp.CREATE, local.pendingOp)

        val server = MockWebServer().apply { start() }
        try {
            server.enqueue(MockResponse().setHeader("Content-Type", "application/json").setBody("""{"id":"s1","entity":"groceries","data":{"name":"牛乳"}}"""))
            // 再起動: 入力は消えておらず、起動時に送り直す
            val restarted = SessionPersistence(DesktopStore(dir), "a1", client(server.url("/").toString()), policy = noWait)
            val s = session(restarted)
            assertEquals(listOf("牛乳"), s.store.list("groceries").map { it.data["name"] })
            assertEquals(1, restarted.pending)
            restarted.resendPending()!!.join()

            val req = server.takeRequest(5, TimeUnit.SECONDS)!!
            assertEquals("POST /v1/apps/a1/entities/groceries/records", "${req.method} ${req.path}")
            assertEquals("linux", req.getHeader("X-Client-Platform"))
            val body = Json.parseToJsonElement(req.body.readUtf8()).jsonObject
            assertEquals(local.clientId, body["clientId"]!!.jsonPrimitive.content)
            assertEquals(JsonPrimitive("牛乳"), body["data"]!!.jsonObject["name"])

            assertEquals(0, restarted.queue!!.count())
            assertEquals(0, restarted.pending)
            val synced = restarted.store.records.rows("a1", "groceries").single()
            assertEquals("s1", synced.recordId)
            assertEquals(PendingOp.NONE, synced.pendingOp)
            // 画面はまだ仮 id で持っているが、仮 id でも同じ行を引ける
            assertNotNull(restarted.store.records.current("a1", "groceries", local.recordId))
        } finally {
            server.shutdown()
        }
    }

    @Test
    fun wipeClearsDeviceAndScreen() = runBlocking {
        val dir = tmp.newFolder()
        val store = DesktopStore(dir)
        store.lastAppId = "shopping"
        val p = SessionPersistence(store, "shopping", client = null)
        val s = session(p)
        s.addItem("牛乳")
        s.dispatch(add, ActionContext(s.top, null))

        s.wipeLocalData()

        assertTrue(s.store.list("groceries").isEmpty())
        assertEquals("home", s.top.screen)
        val after = DesktopStore(dir)
        assertNull(after.lastAppId)
        assertTrue(after.records.rows("shopping", "groceries").isEmpty())
        assertTrue(session(SessionPersistence(after, "shopping", client = null)).store.list("groceries").isEmpty())
    }

    @Test
    fun sendsOnlyWithStgAndToken() {
        val store = DesktopStore(null)
        assertNull(SessionPersistence.client(DesktopApiConfig.fromEnvironment(emptyMap()), store))
        assertNull(SessionPersistence.client(DesktopApiConfig.fromEnvironment(mapOf("TAILORIT_API" to "stg")), store))
        assertNull(SessionPersistence.client(DesktopApiConfig.fromEnvironment(mapOf("TAILORIT_DEV_TOKEN" to "t")), store))
        assertNotNull(SessionPersistence.client(DesktopApiConfig.fromEnvironment(mapOf("TAILORIT_API" to "stg", "TAILORIT_DEV_TOKEN" to "t")), store))
    }

    // MARK: - diff・remap・needsReload(#2622)

    private fun mock() = SessionPersistence(DesktopStore(tmp.newFolder()), "a1", client = null, retryEveryMillis = null)

    @Test
    fun diffFindsAddedUpdatedRemovedAndSkipsUnchanged() = runBlocking {
        val p = mock()
        val old = MockStore(
            mapOf(
                "groceries" to listOf(Row("keep", mapOf("name" to "塩")), Row("edit", mapOf("name" to "牛乳")), Row("gone", mapOf("name" to "卵"))),
                "dropped" to listOf(Row("x", mapOf("n" to "1"))),
            ),
        )
        val new = MockStore(
            mapOf(
                "groceries" to listOf(Row("keep", mapOf("name" to "塩")), Row("edit", mapOf("name" to "豆乳")), Row("add", mapOf("name" to "米"))),
            ),
        )
        val changes = p.diff(old, new).toSet()
        assertEquals(
            setOf(
                SessionPersistence.Change.Put("groceries", Row("edit", mapOf("name" to "豆乳")), created = false),
                SessionPersistence.Change.Put("groceries", Row("add", mapOf("name" to "米")), created = true),
                SessionPersistence.Change.Delete("groceries", "gone"),
                SessionPersistence.Change.Delete("dropped", "x"),
            ),
            changes,
        )
        assertTrue("同じなら差は無い", p.diff(new, new).isEmpty())
        assertTrue("data が同じなら createdAt・author が違っても書かない", p.diff(new, new.copy(rows = mapOf("groceries" to new.list("groceries").map { it.copy(author = "other", createdAt = "t") }))).isEmpty())
        p.shutdown()
    }

    @Test
    fun loadRemapsServerIdsToTemporaryIdsInsideValues() = runBlocking {
        val dir = tmp.newFolder()
        val store = DesktopStore(dir)
        // 送れた作成(サーバーの id srv-1・仮 id は local-c1)と、それを指す行
        store.records.upsert(LocalRecord(appId = "a1", entity = "groceries", recordId = "srv-1", clientId = "c1", data = buildJsonObject { put("name", "牛乳") }))
        store.records.upsert(
            LocalRecord(
                appId = "a1", entity = "groceries", recordId = "srv-2",
                data = buildJsonObject {
                    put("ref", "srv-1")
                    put("refs", buildJsonArray { add("srv-1"); add("srv-9") })
                    put("nested", buildJsonArray { add(buildJsonArray { add("srv-1") }) })
                    put("obj", buildJsonObject { put("k", "srv-1") })
                    put("qty", 2)
                },
            ),
        )
        val p = SessionPersistence(DesktopStore(dir), "a1", client = null, retryEveryMillis = null)
        val rows = p.load(models).list("groceries").associateBy { it.id }
        assertEquals("送れた作成は画面では仮 id のまま", setOf("local-c1", "srv-2"), rows.keys)
        val data = rows.getValue("srv-2").data
        assertEquals("local-c1", data["ref"])
        assertEquals(Json.parseToJsonElement("""["local-c1","srv-9"]"""), Json.parseToJsonElement(data["refs"]!!))
        assertEquals("配列の中の配列も付け替える", Json.parseToJsonElement("""[["local-c1"]]"""), Json.parseToJsonElement(data["nested"]!!))
        assertEquals("オブジェクトの中は付け替えない(今の規則)", Json.parseToJsonElement("""{"k":"srv-1"}"""), Json.parseToJsonElement(data["obj"]!!))
        assertEquals("2", data["qty"])
        p.shutdown()
    }

    @Test
    fun needsReloadOnlyForRejections() = runBlocking {
        val p = mock()
        val noReload = listOf(SyncEvent.Sent("a1", "g"), SyncEvent.Drained, SyncEvent.Paused(3))
        val reload = listOf(
            SyncEvent.Conflict("a1", "g", "m"),
            SyncEvent.Rejected("a1", "g", "m"),
            SyncEvent.Forbidden("a1", "g"),
            SyncEvent.NotMember("a1", "g"),
            SyncEvent.ProRequiredEvent("a1", "g", ProRequired(emptyList())),
            SyncEvent.Denied("a1", "g", RecordWriteDenial.AdminsOnly(null)),
            SyncEvent.PlanLimitEvent("a1", "g", PlanLimit(PlanLimit.Resource.RECORDS_PER_APP)),
            SyncEvent.AppStopped("a1", "g"),
            SyncEvent.TextNotAllowed("a1", "g", null),
        )
        with(p) {
            noReload.forEach { assertFalse("$it", it.needsReload()) }
            reload.forEach { assertTrue("$it", it.needsReload()) }
        }
        p.shutdown()
    }
}
