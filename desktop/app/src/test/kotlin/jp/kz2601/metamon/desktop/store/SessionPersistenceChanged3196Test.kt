package jp.kz2601.metamon.desktop.store

import jp.kz2601.metamon.desktop.Examples
import jp.kz2601.metamon.desktop.MockStore
import jp.kz2601.metamon.desktop.Row
import jp.kz2601.metamon.desktop.net.ClientPlatform
import jp.kz2601.metamon.desktop.net.DesktopApi
import jp.kz2601.metamon.dsl.records.entityModels
import jp.kz2601.metamon.services.live.HttpAppConfigClient
import jp.kz2601.metamon.services.local.BackoffPolicy
import jp.kz2601.metamon.services.local.PendingOp
import kotlinx.coroutines.runBlocking
import kotlinx.serialization.json.JsonPrimitive
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.rules.TemporaryFolder
import java.util.concurrent.TimeUnit

/** SessionPersistence.changed(#3196): 同じなら何もしない・差分だけ書く・送り先があれば送信待ちを起こす。通信は MockWebServer だけ。 */
class SessionPersistenceChanged3196Test {
    @get:Rule
    val tmp = TemporaryFolder()

    private val models = Examples.load("shopping").entityModels()
    private val noWait = BackoffPolicy(baseMillis = 0, multiplier = 1.0, maxAttempts = 1)

    private fun store(vararg rows: Row) = MockStore(mapOf("groceries" to rows.toList()))

    private fun row(id: String, name: String) = Row(id, mapOf("name" to name))

    @Test
    fun changedIgnoresIdenticalStore() = runBlocking {
        val p = SessionPersistence(DesktopStore(tmp.newFolder()), "shopping", client = null)
        val s = store(row("new-1", "牛乳"))
        p.changed(models, s, s.copy())
        p.idle()
        assertTrue(p.store.records.rows("shopping", "groceries").isEmpty())
    }

    @Test
    fun changedWritesOnlyDiff() = runBlocking {
        val p = SessionPersistence(DesktopStore(tmp.newFolder()), "shopping", client = null)
        val first = store(row("new-1", "牛乳"), row("new-2", "卵"))
        p.changed(models, MockStore(), first)
        p.idle()
        val before = p.store.records.rows("shopping", "groceries").associateBy { it.recordId }
        assertEquals(setOf("new-1", "new-2"), before.keys)

        Thread.sleep(5) // updatedAt が変わったか見分ける
        val second = store(row("new-1", "牛乳"), row("new-2", "たまご"))
        p.changed(models, first, second)
        p.idle()
        val after = p.store.records.rows("shopping", "groceries").associateBy { it.recordId }
        assertEquals("たまご", (after.getValue("new-2").data["name"] as JsonPrimitive).content)
        assertEquals("触っていない行は書き直さない", before.getValue("new-1"), after.getValue("new-1"))
        assertEquals(PendingOp.NONE, after.getValue("new-2").pendingOp)

        // 消した行だけ端末からも消える
        p.changed(models, second, store(row("new-1", "牛乳")))
        p.idle()
        assertEquals(listOf("new-1"), p.store.records.rows("shopping", "groceries").map { it.recordId })
    }

    @Test
    fun changedWakesQueueWhenLive() = runBlocking {
        val server = MockWebServer().apply { start() }
        try {
            server.enqueue(MockResponse().setHeader("Content-Type", "application/json").setBody("""{"id":"s1","entity":"groceries","data":{"name":"牛乳"}}"""))
            val client = HttpAppConfigClient(DesktopApi.http(server.url("/").toString(), DesktopApi.devTokens(null), ClientPlatform.LINUX))
            val p = SessionPersistence(DesktopStore(tmp.newFolder()), "a1", client, policy = noWait, retryEveryMillis = null)
            assertTrue(p.isLive)
            p.changed(models, MockStore(), store(row("new-1", "牛乳")))
            p.idle()
            val req = server.takeRequest(5, TimeUnit.SECONDS)
            assertNotNull("積んだ操作が送られる", req)
            assertEquals("POST /v1/apps/a1/entities/groceries/records", "${req!!.method} ${req.path}")
        } finally {
            server.shutdown()
        }
    }
}
