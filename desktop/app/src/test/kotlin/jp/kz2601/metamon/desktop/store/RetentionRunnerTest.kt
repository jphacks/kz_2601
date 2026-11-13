package jp.kz2601.metamon.desktop.store

import jp.kz2601.metamon.desktop.DesktopSession
import jp.kz2601.metamon.desktop.net.ClientPlatform
import jp.kz2601.metamon.desktop.net.DesktopApi
import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.decode
import jp.kz2601.metamon.dsl.records.JSONValue
import jp.kz2601.metamon.dsl.records.Record
import jp.kz2601.metamon.dsl.records.entityModels
import jp.kz2601.metamon.services.live.HttpAppConfigClient
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import jp.kz2601.metamon.services.local.BackoffPolicy
import jp.kz2601.metamon.services.local.PendingOp
import kotlinx.coroutines.runBlocking
import okhttp3.mockwebserver.MockWebServer
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.rules.TemporaryFolder
import java.time.Instant

/** 記録の保存期間(#482): 確認済みの entity だけ、期限切れの自分の記録を端末と送信待ちから消す。通信は止めたサーバーだけ。 */
class RetentionRunnerTest {
    @get:Rule
    val tmp = TemporaryFolder()

    private val now = Instant.parse("2026-11-13T00:00:00Z")
    private val old = "2026-09-01T00:00:00Z"
    private val fresh = "2026-11-10T00:00:00Z"

    private val dsl = AppDSL.decode(
        """
        {"schemaVersion":1,"app":{"name":"記録"},"initialScreen":"home",
         "entities":[
          {"id":"logs","name":"日記","retentionDays":30,"fields":[{"id":"name","name":"名前","type":"string"}]},
          {"id":"posts","name":"投稿","scope":"shared","retentionDays":30,"fields":[{"id":"name","name":"名前","type":"string"}]}
         ],
         "screens":[{"id":"home","title":"ホーム","components":[]}]}
        """.trimIndent(),
    )
    private val models = dsl.entityModels()
    private fun model(id: String) = models.first { it.id == id }

    private fun runner() = RetentionRunner(KeyValueRetentionConsent(JsonKeyValueStore(null)), now = { now })

    private fun record(id: String, createdAt: String?, author: String? = null) =
        Record(id, mapOf("name" to JSONValue.Str(id)), createdAt = createdAt, author = author)

    private fun server(id: String, createdAt: String, author: String? = null) = jp.kz2601.metamon.services.Record(
        id, data = JsonObject(mapOf("name" to JsonPrimitive(id))), author = author, createdAt = createdAt,
    )

    @Test
    fun unconfirmedEntityIsNotPurged() {
        val r = runner()
        val rows = mapOf("logs" to listOf(record("a", old)))
        assertTrue(r.expired("app", models, rows, "me").isEmpty())
        assertEquals(listOf("logs", "posts"), r.pending("app", models).map { it.id })
    }

    @Test
    fun confirmedEntityPurgesOnlyExpired() {
        val r = runner()
        r.confirm("app", model("logs"))
        val rows = mapOf("logs" to listOf(record("a", old), record("b", fresh), record("c", null)))
        assertEquals(mapOf("logs" to listOf("a")), r.expired("app", models, rows, "me"))
        // 確認は appId ごと
        assertTrue(r.expired("other-app", models, rows, "me").isEmpty())
        // サインインしていなければ消さない
        assertTrue(r.expired("app", models, rows, null).isEmpty())
    }

    @Test
    fun othersSharedRecordsAreKept() {
        val r = runner()
        r.confirm("app", model("posts"))
        val rows = mapOf("posts" to listOf(record("mine", old, "me"), record("theirs", old, "u2"), record("unknown", old, null)))
        assertEquals(mapOf("posts" to listOf("mine")), r.expired("app", models, rows, "me"))
    }

    @Test
    fun purgeQueuesDeleteLikeNormalDelete() = runBlocking {
        val dir = tmp.newFolder()
        val store = DesktopStore(dir)
        store.records.replace("app", "logs", listOf(server("s-old", old), server("s-new", fresh)))
        store.records.replace("app", "posts", listOf(server("p-mine", old, "me"), server("p-theirs", old, "u2")))
        // 送り先は止めたサーバー(送れずに送信待ちに残る)
        val dead = MockWebServer().apply { start() }
        val url = dead.url("/").toString()
        dead.shutdown()
        val client = HttpAppConfigClient(DesktopApi.http(url, DesktopApi.devTokens(null), ClientPlatform.LINUX))
        val p = SessionPersistence(store, "app", client, policy = BackoffPolicy(baseMillis = 0, multiplier = 1.0, maxAttempts = 1), retryEveryMillis = null)
        val r = runner()
        val session = DesktopSession(dsl, p.load(models), persistence = p, retention = r)

        assertEquals("未確認なら消さない", 0, session.purgeExpired())
        r.confirm("app", model("logs"))
        r.confirm("app", model("posts"))
        assertEquals(2, session.purgeExpired())
        assertEquals(listOf("s-new"), session.store.list("logs").map { it.id })
        assertEquals(listOf("p-theirs"), session.store.list("posts").map { it.id })
        p.idle()

        val logs = store.records.rows("app", "logs").associate { it.recordId to it.pendingOp }
        assertEquals(PendingOp.DELETE, logs["s-old"])
        assertEquals(PendingOp.NONE, logs["s-new"])
        val posts = store.records.rows("app", "posts").associate { it.recordId to it.pendingOp }
        assertEquals(PendingOp.DELETE, posts["p-mine"])
        assertEquals(PendingOp.NONE, posts["p-theirs"])
        assertEquals("もう一度呼んでも同じ記録を二度消さない", 0, session.purgeExpired())
        p.close()
    }
}
