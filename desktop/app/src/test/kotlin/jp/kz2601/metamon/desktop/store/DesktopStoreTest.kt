package jp.kz2601.metamon.desktop.store

import jp.kz2601.metamon.desktop.net.ClientPlatform
import jp.kz2601.metamon.desktop.net.DesktopApi
import jp.kz2601.metamon.services.consent.AiConsentStatus
import jp.kz2601.metamon.services.live.HttpAppConfigClient
import jp.kz2601.metamon.services.local.BackoffPolicy
import jp.kz2601.metamon.services.local.LocalRecord
import jp.kz2601.metamon.services.local.PendingOp
import jp.kz2601.metamon.services.local.SyncOperation
import jp.kz2601.metamon.services.local.SyncQueue
import kotlinx.coroutines.runBlocking
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.put
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.rules.TemporaryFolder
import java.io.File

/** 端末の保存(#1754): 置き場・再起動をまたいだ読み戻し・送信待ちの順と畳み込み・オフライン・サインアウトの後片付け。 */
class DesktopStoreTest {
    @get:Rule
    val tmp = TemporaryFolder()

    private val noWait = BackoffPolicy(baseMillis = 0, multiplier = 1.0, maxAttempts = 1)

    private fun appConfig(baseUrl: String) = HttpAppConfigClient(
        DesktopApi.http(baseUrl, DesktopApi.devTokens(null), ClientPlatform.LINUX),
    )

    private fun data(vararg pairs: Pair<String, String>) = JsonObject(pairs.associate { (k, v) -> k to JsonPrimitive(v) })

    private fun create(id: String, vararg pairs: Pair<String, String>) =
        SyncOperation(id = id, appId = "a1", entity = "items", recordId = LocalRecord.temporaryId(id), kind = PendingOp.CREATE, data = data(*pairs))

    @Test
    fun dataDirPerOs() {
        val env = mapOf("APPDATA" to "C:\\Users\\k\\AppData\\Roaming")
        assertEquals(File("C:\\Users\\k\\AppData\\Roaming", "TailorIt"), AppDataDir.resolve("Windows 11", env, "C:\\Users\\k", null))
        assertEquals(File("/home/k/.local/share/tailorit"), AppDataDir.resolve("Linux", emptyMap(), "/home/k", null))
        assertEquals(
            File("/home/k/.var/app/jp.tailorit/data/tailorit"),
            AppDataDir.resolve("Linux", mapOf("XDG_DATA_HOME" to "/home/k/.var/app/jp.tailorit/data"), "/home/k", null),
        )
        // 相対の XDG_DATA_HOME は使わない
        assertEquals(File("/home/k/.local/share/tailorit"), AppDataDir.resolve("Linux", mapOf("XDG_DATA_HOME" to "rel"), "/home/k", null))
        assertEquals(File("/Users/k/Library/Application Support/TailorIt"), AppDataDir.resolve("Mac OS X", emptyMap(), "/Users/k", null))
        assertEquals(File("/tmp/x"), AppDataDir.resolve("Linux", emptyMap(), "/home/k", "/tmp/x"))
    }

    @Test
    fun settingsAndConsentSurviveRestart() {
        val dir = tmp.newFolder()
        DesktopStore(dir).apply {
            lastAppId = "a1"
            aiConsent.grant()
            crashReporting.setEnabled(false)
        }
        val again = DesktopStore(dir) // 別インスタンス = 再起動後
        assertEquals("a1", again.lastAppId)
        assertEquals(AiConsentStatus.GRANTED, again.aiConsent.state.value.status)
        assertFalse(again.crashReporting.isEnabled)
        assertTrue(File(dir, DesktopStore.CONSENT_FILE).readText().contains("metamon.aiConsent.version"))
    }

    @Test
    fun keyValueStoreTypesAndBrokenFile() {
        val f = File(tmp.newFolder(), "kv.json")
        JsonKeyValueStore(f).apply {
            putInt("i", 3)
            putLong("l", 1_700_000_000_000)
            putBoolean("b", true)
            putString("s", "12") // 数に見える文字は文字のまま
            remove("missing")
        }
        val kv = JsonKeyValueStore(f)
        assertEquals(3, kv.getInt("i"))
        assertEquals(1_700_000_000_000, kv.getLong("l"))
        assertEquals(true, kv.getBoolean("b"))
        assertEquals("12", kv.getString("s"))
        assertNull(kv.getString("i"))

        f.writeText("{壊れた")
        assertEquals(emptySet<String>(), JsonKeyValueStore(f).keys())
    }

    @Test
    fun recordsCacheSurvivesRestart() = runBlocking {
        val dir = tmp.newFolder()
        DesktopStore(dir).records.upsert(
            LocalRecord(appId = "a1", entity = "items", recordId = "r1", data = data("name" to "牛乳"), createdAt = "2026-11-12T00:00:00Z"),
        )
        val rows = DesktopStore(dir).records.records("a1", "items")
        assertEquals(listOf("r1"), rows.map { it.id })
        assertEquals(JsonPrimitive("牛乳"), rows.single().data["name"])
    }

    @Test
    fun queueKeepsOrderAndFoldsSameRecord() = runBlocking {
        val dir = tmp.newFolder()
        val q = DesktopStore(dir).queue(appConfig("http://127.0.0.1:9/"), noWait)
        assertEquals(SyncQueue.EnqueueResult.APPENDED, q.enqueue(create("c1", "name" to "牛乳")))
        assertEquals(SyncQueue.EnqueueResult.APPENDED, q.enqueue(create("c2", "name" to "卵")))
        val update = SyncOperation(appId = "a1", entity = "items", recordId = LocalRecord.temporaryId("c1"), kind = PendingOp.UPDATE, data = data("qty" to "2"))
        assertEquals(SyncQueue.EnqueueResult.MERGED, q.enqueue(update))
        val delete = SyncOperation(appId = "a1", entity = "items", recordId = LocalRecord.temporaryId("c2"), kind = PendingOp.DELETE)
        assertEquals(SyncQueue.EnqueueResult.CANCELLED_CREATE, q.enqueue(delete))
        assertEquals(SyncQueue.EnqueueResult.APPENDED, q.enqueue(create("c3", "name" to "パン")))

        val reopened = DesktopStore(dir).queue(appConfig("http://127.0.0.1:9/"), noWait).operations()
        assertEquals(listOf("c1", "c3"), reopened.map { it.id })
        assertEquals(data("name" to "牛乳", "qty" to "2"), reopened.first().data)
    }

    @Test
    fun offlineInputIsKeptAndSentAfterRestart() = runBlocking {
        val dir = tmp.newFolder()
        val dead = MockWebServer().apply { start() }
        val deadUrl = dead.url("/").toString()
        dead.shutdown()

        val store = DesktopStore(dir)
        for (id in listOf("c1", "c2")) {
            store.records.upsert(
                LocalRecord(appId = "a1", entity = "items", recordId = LocalRecord.temporaryId(id), pendingOp = PendingOp.CREATE, clientId = id, data = data("name" to id)),
            )
            store.queue(appConfig(deadUrl), noWait).enqueue(create(id, "name" to id))
        }
        store.queue(appConfig(deadUrl), noWait).drain() // 圏外: 送れずに止まる
        assertEquals(2, store.queue(appConfig(deadUrl), noWait).count())

        val server = MockWebServer().apply { start() }
        try {
            server.enqueue(MockResponse().setBody("""{"id":"s1","entity":"items","data":{"name":"c1"}}"""))
            server.enqueue(MockResponse().setBody("""{"id":"s2","entity":"items","data":{"name":"c2"}}"""))
            val restarted = DesktopStore(dir)
            assertEquals(2, restarted.records.records("a1", "items").size) // 入力は消えていない
            val q = restarted.queue(appConfig(server.url("/").toString()), noWait)
            q.drain()
            assertEquals(0, q.count())
            val sent = listOf(server.takeRequest(), server.takeRequest())
            assertTrue(sent.all { it.method == "POST" })
            assertEquals(listOf("c1", "c2"), sent.map { Regex("\"clientId\":\"([^\"]+)\"").find(it.body.readUtf8())!!.groupValues[1] })
            assertEquals(setOf("s1", "s2"), restarted.records.rows("a1", "items").map { it.recordId }.toSet())
            assertTrue(restarted.records.rows("a1", "items").none { it.unsent })
        } finally {
            server.shutdown()
        }
    }

    @Test
    fun wipeRemovesUserDataButKeepsDevicePreference() = runBlocking {
        val dir = tmp.newFolder()
        val store = DesktopStore(dir)
        store.lastAppId = "a1"
        store.aiConsent.grant()
        store.crashReporting.setEnabled(false)
        store.records.upsert(LocalRecord(appId = "a1", entity = "items", recordId = "r1", data = buildJsonObject { put("name", "牛乳") }))
        store.documents.store("a1", """{"appId":"a1","version":2,"dsl":{}}""")
        store.queue(appConfig("http://127.0.0.1:9/"), noWait).enqueue(create("c1", "name" to "卵"))

        store.wipe()

        val after = DesktopStore(dir)
        assertNull(after.lastAppId)
        assertEquals(AiConsentStatus.NOT_PROMPTED, after.aiConsent.state.value.status)
        assertFalse(after.crashReporting.isEnabled) // 端末の設定は残す
        assertTrue(after.records.records("a1", "items").isEmpty())
        assertNull(after.documents.loadJson("a1"))
        assertEquals(0, after.queue(appConfig("http://127.0.0.1:9/"), noWait).count())
    }
}
