package jp.kz2601.metamon.desktop.store

import jp.kz2601.metamon.desktop.Examples
import jp.kz2601.metamon.desktop.MockStore
import jp.kz2601.metamon.desktop.Row
import jp.kz2601.metamon.desktop.net.ClientPlatform
import jp.kz2601.metamon.desktop.net.DesktopApi
import jp.kz2601.metamon.dsl.records.entityModels
import jp.kz2601.metamon.services.live.HttpAppConfigClient
import jp.kz2601.metamon.services.local.BackoffPolicy
import jp.kz2601.metamon.services.local.LocalRecord
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.withTimeout
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotEquals
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.rules.TemporaryFolder

/** SessionPersistence の id の付け替え(#3197): newRecordId・screenId・Reload.applyTo・named(断られた行だけ戻す)。通信は MockWebServer だけ。 */
class SessionPersistenceIds3197Test {
    @get:Rule
    val tmp = TemporaryFolder()

    private val models = Examples.load("shopping").entityModels()

    private fun rec(recordId: String, clientId: String? = null) =
        LocalRecord(appId = "a1", entity = "groceries", recordId = recordId, clientId = clientId)

    private fun row(id: String, name: String = id) = Row(id, mapOf("name" to name))

    @Test
    fun newRecordIdIsTemporaryAndUnique() {
        val p = SessionPersistence(DesktopStore(tmp.newFolder()), "a1", client = null)
        val a = p.newRecordId()
        val b = p.newRecordId()
        assertTrue(a.startsWith(LocalRecord.TEMP_PREFIX))
        assertTrue("clientId 部分が空でない", a.length > LocalRecord.TEMP_PREFIX.length)
        assertEquals(a.lowercase(), a)
        assertNotEquals(a, b)
    }

    @Test
    fun screenIdPrefersClientId() {
        assertEquals("local-c1", SessionPersistence.screenId(rec("server-1", clientId = "c1")))
        assertEquals("server-2", SessionPersistence.screenId(rec("server-2")))
        assertEquals("local-c3", SessionPersistence.screenId(rec("local-c3", clientId = "c3")))
    }

    @Test
    fun reloadApplyToUpsertsAndRemovesOnlyNamedRows() {
        val s = MockStore(mapOf("groceries" to listOf(row("a", "古い"), row("b"), row("c")), "other" to listOf(row("x"))))
        val reload = SessionPersistence.Reload("groceries", upserts = listOf(row("a", "戻した"), row("d")), removed = setOf("b"))
        val out = reload.applyTo(s)
        assertEquals(listOf("a", "c", "d"), out.list("groceries").map { it.id })
        assertEquals("戻した", out.list("groceries").first().data["name"])
        assertEquals("他の entity はそのまま", s.list("other"), out.list("other"))
        assertTrue(SessionPersistence.Reload("groceries", emptyList(), emptySet()).isEmpty)
        assertEquals(s, SessionPersistence.Reload("groceries", emptyList(), emptySet()).applyTo(s))
    }

    @Test
    fun rejectedCreateReloadsOnlyTheNamedRow() = runBlocking {
        val server = MockWebServer().apply { start() }
        try {
            server.enqueue(MockResponse().setResponseCode(400).setHeader("Content-Type", "application/json").setBody("""{"error":"bad"}"""))
            server.enqueue(MockResponse().setHeader("Content-Type", "application/json").setBody("""{"id":"s2","entity":"groceries","data":{"name":"卵"}}"""))
            val client = HttpAppConfigClient(DesktopApi.http(server.url("/").toString(), DesktopApi.devTokens(null), ClientPlatform.LINUX))
            val p = SessionPersistence(
                DesktopStore(tmp.newFolder()), "a1", client,
                policy = BackoffPolicy(baseMillis = 0, multiplier = 1.0, maxAttempts = 1), retryEveryMillis = null,
            )
            val got = CompletableDeferred<SessionPersistence.Reload>()
            p.onReload = { got.complete(it) }
            val first = MockStore(mapOf("groceries" to listOf(row("new-1", "牛乳"))))
            p.changed(models, MockStore(), first)
            p.idle()
            val both = MockStore(mapOf("groceries" to listOf(row("new-1", "牛乳"), row("new-2", "卵"))))
            p.changed(models, first, both)
            p.idle()

            val delta = withTimeout(10_000) { got.await() }
            assertEquals("groceries", delta.entity)
            assertEquals(setOf("new-1"), delta.removed)
            assertTrue(delta.upserts.none { it.id == "new-2" })
            assertEquals(listOf("new-2"), delta.applyTo(both).list("groceries").map { it.id })
        } finally {
            server.shutdown()
        }
    }
}
