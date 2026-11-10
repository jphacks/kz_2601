package jp.kz2601.metamon.services.local

import jp.kz2601.metamon.services.http.MINI_DSL
import jp.kz2601.metamon.services.http.json
import jp.kz2601.metamon.services.http.metamonHttp
import jp.kz2601.metamon.services.live.HttpAppConfigClient
import kotlinx.coroutines.runBlocking
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.rules.TemporaryFolder

class DocumentCacheTest {
    @get:Rule
    val tmp = TemporaryFolder()

    private fun envelope(version: Int, appId: String = "app1") =
        """{"appId":"$appId","version":$version,"dsl":$MINI_DSL,"source":{"kind":"ai"}}"""

    @Test
    fun roundTripsThroughFileKeepingUnknownKeys() = runBlocking {
        val dir = tmp.newFolder("metamon-dsl")
        DocumentCache(dir).store("app1", envelope(3))
        val loaded = DocumentCache(dir).load("app1")!! // 別インスタンス = 再起動後
        assertEquals(3, loaded.version)
        assertEquals("家計簿", loaded.document.dsl.app.name)
        assertTrue(loaded.envelopeJson.contains("\"source\"")) // 知らないキーも残る
    }

    @Test
    fun doesNotRollBackToOlderVersion() = runBlocking {
        val cache = DocumentCache(tmp.newFolder())
        cache.store("app1", envelope(5))
        cache.store("app1", envelope(4))
        assertEquals(5, cache.load("app1")!!.version)
        cache.store("app1", envelope(6))
        assertEquals(6, cache.load("app1")!!.version)
    }

    @Test
    fun ignoresEnvelopeWithoutVersion() = runBlocking {
        val cache = DocumentCache(tmp.newFolder())
        cache.store("app1", "{\"appId\":\"app1\"}")
        assertNull(cache.loadJson("app1"))
    }

    @Test
    fun brokenFileIsDropped() = runBlocking {
        val dir = tmp.newFolder()
        dir.resolve("app1.json").writeText("{\"version\":2,\"dsl\":") // 書く途中で落ちた古い形式など
        val cache = DocumentCache(dir)
        assertNull(cache.load("app1"))
        assertFalse(dir.resolve("app1.json").exists())
    }

    @Test
    fun appIdIsSanitizedAndRemovable() = runBlocking {
        val dir = tmp.newFolder()
        val cache = DocumentCache(dir)
        cache.store("../evil", envelope(1, "../evil"))
        assertTrue(dir.resolve("___evil.json").exists()) // フォルダの外に書かない
        cache.store("app2", envelope(1, "app2"))
        cache.remove("../evil")
        assertNull(cache.load("../evil"))
        assertEquals(1, cache.load("app2")!!.version)
        cache.removeAll()
        assertNull(DocumentCache(dir).load("app2"))
    }

    @Test
    fun memoryOnlyCacheWorksWithoutDirectory() = runBlocking {
        val cache = DocumentCache(null)
        cache.store("app1", envelope(2))
        assertEquals(2, cache.load("app1")!!.version)
    }

    @Test
    fun appConfigClientStoresFetchedDocument() = runBlocking {
        val server = MockWebServer().apply { start() }
        try {
            val cache = DocumentCache(tmp.newFolder())
            val client = HttpAppConfigClient(server.metamonHttp(), cache)
            server.enqueue(json(200, envelope(7)))
            client.fetchDocument("app1")
            assertEquals(7, cache.load("app1")!!.version)
            server.enqueue(MockResponse().setResponseCode(304))
            client.fetchDocumentIfChanged("app1", 7)
            server.enqueue(json(200, envelope(9)))
            client.fetchDocumentIfChanged("app1", 7)
            assertEquals(9, cache.load("app1")!!.version)
        } finally {
            server.shutdown()
        }
    }
}
