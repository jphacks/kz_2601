package jp.kz2601.metamon.services.local

import kotlinx.coroutines.test.TestScope
import kotlinx.coroutines.test.advanceTimeBy
import kotlinx.coroutines.test.runCurrent
import kotlinx.coroutines.test.runTest
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.File
import java.nio.file.Files
import kotlin.coroutines.EmptyCoroutineContext

/** 記録・アプリを消した時の端末内の録音ファイル(#1098・iOS `store.audios.remove`)。 */
class RecordSyncEngineAudioTest {
    private val client = FakeRecordsClient()
    private val net = ManualConnectivity(online = true)
    private val dir: File = Files.createTempDirectory("audio").toFile().also { d ->
        listOf("a1", "a2", "a3").forEach { File(d, "$it.m4a").writeText(it) }
    }

    /** `LocalAudioStore.remove` と同じく `<id>.m4a` を消す */
    private val removeAudios: (Set<String>) -> Unit = { ids -> ids.forEach { File(dir, "$it.m4a").delete() } }

    private fun exists(id: String) = File(dir, "$id.m4a").isFile

    @After
    fun cleanUp() {
        dir.deleteRecursively()
    }

    private fun TestScope.engine(store: RecordStore = RecordStore(null, EmptyCoroutineContext)): RecordSyncEngine {
        val queue = SyncQueue(null, store, client, BackoffPolicy.immediate, sleep = {}, io = EmptyCoroutineContext)
        return RecordSyncEngine(store, queue, client, net, backgroundScope, removeAudios = removeAudios).also { it.start() }
    }

    private fun audio(id: String) = JsonObject(mapOf("title" to JsonPrimitive("a"), "memo" to JsonPrimitive("audio:$id|12")))

    @Test
    fun deletingRecordRemovesOnlyItsAudio() = runTest {
        val e = engine()
        e.write("app", "todo", null, audio("a1"))
        e.write("app", "todo", null, audio("a2"))
        runCurrent()
        e.delete("app", "todo", "srv-1")
        runCurrent()
        assertFalse("消した記録の録音は消える", exists("a1"))
        assertTrue("ほかの記録の録音は残る", exists("a2"))
    }

    @Test
    fun undoKeepsAudioAndCommitRemovesIt() = runTest {
        val e = engine()
        e.write("app", "todo", null, audio("a1"))
        runCurrent()
        val (_, ticket) = e.deleteUndoably("app", "todo", "srv-1", graceMillis = 5_000)
        advanceTimeBy(4_000)
        assertTrue("取り消し期間の間は消さない", exists("a1"))
        e.undoDelete(ticket)
        advanceTimeBy(5_000)
        runCurrent()
        assertTrue("取り消したら残る", exists("a1"))

        e.deleteUndoably("app", "todo", "srv-1", graceMillis = 5_000)
        advanceTimeBy(5_001)
        runCurrent()
        assertFalse("取り消し期間が過ぎたら消す", exists("a1"))
    }

    @Test
    fun audioSharedWithAnotherRecordIsKept() = runTest {
        val e = engine()
        e.write("app", "todo", null, audio("a1"))
        e.write("other", "memo", null, audio("a1")) // 別のアプリの記録も同じ録音を指す
        runCurrent()
        e.delete("app", "todo", "srv-1")
        runCurrent()
        assertTrue("まだ使う記録があるので残す", exists("a1"))

        e.delete("other", "memo", "srv-2")
        runCurrent()
        assertFalse("最後の記録を消したら消す", exists("a1"))
    }

    @Test
    fun purgeRemovesOnlyThatAppsAudio() = runTest {
        val e = engine()
        e.write("app", "todo", null, audio("a1"))
        e.write("app", "note", null, audio("a2"))
        e.write("app", "note", null, audio("a3"))
        e.write("other", "memo", null, audio("a3")) // 別のアプリも使う
        runCurrent()
        e.purge("app")
        assertFalse(exists("a1"))
        assertFalse(exists("a2"))
        assertTrue("別のアプリが使う録音は残す", exists("a3"))
        assertEquals(1, e.store.records("other", "memo").size)
    }

    @Test
    fun purgeSeesEntitiesNotLoadedYet() = runTest {
        val db = Files.createTempDirectory("records").toFile()
        try {
            engine(RecordStore(db, EmptyCoroutineContext)).run {
                write("app", "todo", null, audio("a1"))
                write("other", "memo", null, audio("a2"))
            }
            runCurrent()
            // 起動し直した後(どの entity もまだ読んでいない)
            val fresh = engine(RecordStore(db, EmptyCoroutineContext))
            fresh.purge("app")
            assertFalse("ファイルにだけある記録の録音も消す", exists("a1"))
            assertTrue(exists("a2"))
        } finally {
            db.deleteRecursively()
        }
    }
}
