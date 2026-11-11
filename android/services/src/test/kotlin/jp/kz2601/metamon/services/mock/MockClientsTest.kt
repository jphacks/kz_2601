package jp.kz2601.metamon.services.mock

import jp.kz2601.metamon.services.ChatMessage
import jp.kz2601.metamon.services.ChatMode
import jp.kz2601.metamon.services.ChatTurn
import jp.kz2601.metamon.services.GenerateProgress
import jp.kz2601.metamon.services.http.ApiError
import jp.kz2601.metamon.services.http.FailureKind
import kotlinx.coroutines.test.runTest
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Assert.fail
import org.junit.Test
import java.io.File
import java.time.Instant

val testSamples: DslSamples = run {
    val dir = File(requireNotNull(System.getProperty("metamon.dslExamplesDir")) { "metamon.dslExamplesDir が未設定(Gradle から実行する)" })
    DslSamples { name -> File(dir, "$name.json").takeIf(File::isFile)?.readText() }
}

class MockAIClientTest {
    private val ai = MockAIClient(testSamples, delayMillis = 0, stepDelayMillis = 0)

    @Test
    fun sampleNameFollowsIosOrder() {
        assertEquals("receipt_ai", MockAIClient.sampleName("レシートの写真を読んで"))
        assertEquals("training", MockAIClient.sampleName("筋トレの記録"))
        assertEquals("shopping", MockAIClient.sampleName("買い物リストが欲しい"))
        assertEquals("initial", MockAIClient.sampleName("最初に戻して"))
        assertEquals("kakeibo", MockAIClient.sampleName("なにか"))
    }

    @Test
    fun morphReturnsSampleWithNextVersion() = runTest {
        val r = ai.morph("家計簿アプリが欲しい。", appId = "a1", currentVersion = 2)
        assertEquals("a1", r.document.appId)
        assertEquals(3, r.document.version)
        assertTrue(r.envelopeJson.contains("\"appId\":\"a1\""))
        assertTrue(r.summary!!.contains("反映しました")) // 作った後(version > 0)は改変扱い
    }

    @Test
    fun morphStreamingEmitsAppThenComponents() = runTest {
        val events = mutableListOf<GenerateProgress>()
        val r = ai.morphStreaming("買い物リストのアプリを作って。", appId = "a1", currentVersion = 0) { events += it }
        val app = events.first() as GenerateProgress.App
        assertEquals(r.document.dsl.app.name, app.name)
        val comps = events.drop(1).map { (it as GenerateProgress.Component).component }
        assertTrue("部品が流れていない", comps.isNotEmpty())
        assertEquals(comps.indices.toList(), comps.map { it.index })
        assertTrue(comps.all { it.screen == r.document.dsl.initialScreen })
    }

    @Test
    fun chatBranches() = runTest {
        fun user(vararg t: String) = t.map { ChatMessage(ChatMessage.Role.USER, it) }
        assertEquals(ChatTurn.Kind.ANSWER, ai.chat("a", user("WebView ですか?")).kind)
        assertEquals(ChatTurn.Kind.QUESTION, ai.chat("a", user("家計簿")).kind)
        val multi = ai.chat("a", user("グラフを足したい"))
        assertEquals(ChatTurn.Kind.PROPOSAL, multi.kind)
        assertTrue(multi.isMultiSelect)
        val ready = ai.chat("a", user("家計簿", "お任せ"))
        assertEquals(ChatTurn.Kind.READY, ready.kind)
        assertEquals("家計簿", ready.transcript)
        val paid = ai.chat("a", user("みんなで書き込める掲示板を作りたいです"))
        assertEquals(true, paid.requiresPro)
        assertEquals(ChatTurn.Kind.READY, ai.chat("a", user("金額を大きく"), mode = ChatMode.EDIT).kind)
    }
}

class InMemoryAppConfigClientTest {
    private fun client(sharing: Boolean = false) = InMemoryAppConfigClient(
        seed = MockSeed.demoRecords(Instant.parse("2026-11-11T00:00:00Z")),
        sharing = sharing,
        initialDocument = { testSamples.document(DslSamples.INITIAL, it, 1) },
    )

    private val data = JsonObject(mapOf("amount" to JsonPrimitive(100)))

    @Test
    fun recordsCrudAndIdempotentCreate() = runTest {
        val c = client()
        assertEquals(2, c.listRecords("a", "expense").size)
        val r1 = c.createRecord("a", "expense", data, clientId = "cid-1")
        val r2 = c.createRecord("a", "expense", data, clientId = "cid-1")
        assertEquals(r1, r2)
        assertEquals(3, c.listRecords("a", "expense").size)
        val u = c.updateRecord("a", "expense", "cid-1", JsonObject(mapOf("memo" to JsonPrimitive("x"))))
        assertEquals(setOf("amount", "memo"), u.data.keys)
        c.deleteRecord("a", "expense", "cid-1")
        assertEquals(2, c.listRecords("a", "expense").size)
        val on = c.setMark("a", "expense", "1", "like", true)
        assertEquals(1, on.count)
        assertTrue(on.mine)
        assertEquals(on, c.setMark("a", "expense", "1", "like", true))
        assertFalse(c.setMark("a", "expense", "1", "like", false).mine)
    }

    @Test
    fun deleteAllRecordsClearsOnlyThatEntity() = runTest {
        val c = client()
        c.createRecord("a", "memo", data, clientId = "m1")
        val memos = c.listRecords("a", "memo").size
        c.deleteAllRecords("a", "expense")
        assertEquals(0, c.listRecords("a", "expense").size)
        assertEquals(memos, c.listRecords("a", "memo").size)
        c.deleteAllRecords("a", "unknown") // 無い entity でも落ちない
    }

    @Test
    fun putDocumentChecksVersion() = runTest {
        val c = client()
        val doc = c.fetchDocument("a")
        assertEquals(1, doc.version)
        assertNull(c.fetchDocumentIfChanged("a", 1))
        val dsl = testSamples.json("kakeibo")!!
        assertEquals(2, c.putDocument("a", dsl, baseVersion = 1).version)
        try {
            c.putDocument("a", dsl, baseVersion = 1)
            fail("409 になるはず")
        } catch (e: ApiError) {
            assertEquals(409, e.status)
            assertEquals(2, e.currentVersion)
        }
        assertEquals(2, c.fetchDocumentIfChanged("a", 1)!!.version)
    }

    @Test
    fun appsAndSharing() = runTest {
        val c = client(sharing = true)
        val a = c.createApp(name = "家計簿")
        val b = c.duplicateApp(a.id)
        c.reorderApps(listOf(b.id, a.id))
        assertEquals(listOf(b.id, a.id), c.listApps().map { it.id })
        assertEquals(1, c.listApps().first().sortIndex)
        assertEquals(InMemoryAppConfigClient.SHARE_TOKEN, c.share(a.id).token)
        c.deleteApp(a.id)
        assertEquals(listOf(b.id), c.listApps().map { it.id })
        try {
            client(sharing = false).share("x")
            fail("共有の無いモックは 404")
        } catch (e: ApiError) {
            assertEquals(404, e.status)
        }
    }
}

class OfflineSimulatingAppConfigClientTest {
    @Test
    fun dropsRecordCallsUntilDeadline() = runTest {
        var now = Instant.parse("2026-11-11T00:00:00Z")
        val inner = InMemoryAppConfigClient(initialDocument = { testSamples.document(DslSamples.INITIAL, it, 1) })
        val c = OfflineSimulatingAppConfigClient(inner, offlineUntil = now.plusSeconds(5), now = { now })
        assertTrue(c.isOffline)
        assertEquals(1, c.fetchDocument("a").version) // DSL は通す
        try {
            c.createRecord("a", "memo", JsonObject(emptyMap()), clientId = "c1")
            fail("圏外のはず")
        } catch (e: Exception) {
            assertEquals(FailureKind.Offline, FailureKind.classify(e))
        }
        now = now.plusSeconds(6)
        assertFalse(c.isOffline)
        c.createRecord("a", "memo", JsonObject(emptyMap()), clientId = "c1")
        assertEquals(listOf("drop create memo c1", "pass create memo c1"), c.log.toList())
    }

    @Test
    fun dropsDeleteAllWhileOffline() = runTest {
        val inner = InMemoryAppConfigClient(initialDocument = { testSamples.document(DslSamples.INITIAL, it, 1) })
        inner.createRecord("a", "memo", JsonObject(emptyMap()), clientId = "c1")
        val c = OfflineSimulatingAppConfigClient(inner)
        c.setOffline(true)
        try {
            c.deleteAllRecords("a", "memo")
            fail("圏外のはず")
        } catch (e: Exception) {
            assertEquals(FailureKind.Offline, FailureKind.classify(e))
        }
        assertEquals(1, inner.listRecords("a", "memo").size)
        c.setOffline(false)
        c.deleteAllRecords("a", "memo")
        assertEquals(0, inner.listRecords("a", "memo").size)
        assertEquals(listOf("drop delete-all memo", "pass delete-all memo"), c.log.toList())
    }
}

class MockTranscriberTest {
    @Test
    fun returnsCannedText() = runTest {
        val t = MockTranscriber(delayMillis = 0)
        t.startRecording()
        assertTrue(t.isRecording)
        assertEquals(MockTranscriber.DEFAULT_TEXT, t.stopAndTranscribe())
        assertFalse(t.isRecording)
    }
}
