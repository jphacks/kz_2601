package jp.kz2601.metamon.desktop.store

import jp.kz2601.metamon.desktop.ActionContext
import jp.kz2601.metamon.desktop.DesktopSession
import jp.kz2601.metamon.desktop.Examples
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
}
