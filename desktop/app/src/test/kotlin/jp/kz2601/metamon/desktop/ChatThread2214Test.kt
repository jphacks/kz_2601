package jp.kz2601.metamon.desktop

import jp.kz2601.metamon.desktop.store.DesktopStore
import jp.kz2601.metamon.desktop.store.SessionPersistence
import jp.kz2601.metamon.dsl.ComponentType
import jp.kz2601.metamon.dsl.records.ChatThread
import jp.kz2601.metamon.dsl.records.JSONValue
import jp.kz2601.metamon.dsl.records.Record
import jp.kz2601.metamon.dsl.screen
import jp.kz2601.metamon.services.local.LocalRecord
import jp.kz2601.metamon.services.local.PendingOp
import kotlinx.coroutines.runBlocking
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import java.time.Instant
import java.time.LocalDate
import java.time.ZoneId

/** #2214: chatThread の取り直し・自分の発言を二重に出さない・並び・間隔(サーバーは作り物)。 */
class ChatThread2214Test {
    private val dsl = Examples.load("chat")

    private fun remote(id: String, text: String, at: String, author: String = "u-other", name: String? = "花子") =
        Record(id, mapOf("text" to JSONValue.Str(text)), createdAt = at, author = author, authorName = name)

    private class FakeRemote(var rows: List<Record>?, var aliases: Map<String, String> = emptyMap()) : ChatRemote {
        var fetched = 0
        override suspend fun fetch(entity: String): List<Record>? {
            fetched++
            return rows
        }

        override suspend fun aliases(entity: String) = aliases
    }

    @Test
    fun rendersOnDesktop() {
        val c = dsl.screen("home")!!.components.single()
        assertEquals(ComponentType.CHAT_THREAD, c.type)
        assertEquals("rendered", DesktopLeaves.state(c))
    }

    // MARK: - 取り直し

    @Test
    fun pollingShowsNewMessages() = runBlocking {
        val session = DesktopSession(dsl)
        val fake = FakeRemote(listOf(remote("s1", "こんにちは", "2026-11-13T01:00:00Z")))
        assertTrue(ChatSync.pull(fake, session.records, "messages", selfUid = null))
        assertEquals(listOf("s1"), session.store.list("messages").map { it.id })

        fake.rows = fake.rows!! + remote("s2", "元気?", "2026-11-13T01:01:00Z", author = "u-third", name = "太郎")
        ChatSync.pull(fake, session.records, "messages", selfUid = null)
        val rows = ChatRows.ordered(session.records("messages"))
        assertEquals(listOf("こんにちは", "元気?"), rows.map { (it["text"] as JSONValue.Str).value })
        val entity = session.entityModel("messages")
        assertFalse("他の人の分は左", ChatThread.isMine(rows[1], entity, session.uid))
        assertEquals("太郎", rows[1].authorName)
        assertTrue("別の人が続いたら名前を出す", ChatThread.showsSender(rows[1], rows[0], entity, session.uid))
    }

    @Test
    fun ownMessageIsNotDuplicated() = runBlocking {
        val p = SessionPersistence(DesktopStore(null), "chat", client = null)
        val session = DesktopSession(dsl, persistence = p)
        val id = session.postMessage("messages", mapOf("text" to JSONValue.Str("送ったよ")), Instant.parse("2026-11-13T02:00:00Z"))
        assertTrue(id.startsWith(LocalRecord.TEMP_PREFIX))
        // 送った直後: サーバーにはもうあるが、端末の対応(サーバーの id → 仮 id)はまだ無い
        val fake = FakeRemote(listOf(remote("srv-9", "送ったよ", "2026-11-13T02:00:01Z", author = "u-me", name = "私")))
        ChatSync.pull(fake, session.records, "messages", selfUid = null)
        assertEquals("二重に出さない", listOf(id), session.store.list("messages").map { it.id })
        // 対応が付いた後: 仮 id のまま 1 通(開いている物が迷子にならない)・自分の分
        fake.aliases = mapOf("srv-9" to id)
        ChatSync.pull(fake, session.records, "messages", selfUid = null)
        val rows = session.store.list("messages")
        assertEquals(listOf(id), rows.map { it.id })
        assertNull("自分の分は author を付けない(右に出す)", rows.single().author)
        assertEquals("サーバーの時刻にそろえる", "2026-11-13T02:00:01Z", rows.single().createdAt)
        assertTrue(ChatThread.isMine(session.records("messages").single(), session.entityModel("messages"), session.uid))
        p.shutdown()
    }

    @Test
    fun unsentStaysAndDeletedGoes() = runBlocking {
        val session = DesktopSession(dsl)
        val fake = FakeRemote(listOf(remote("s1", "a", "2026-11-13T01:00:00Z"), remote("s2", "b", "2026-11-13T01:01:00Z")))
        ChatSync.pull(fake, session.records, "messages", selfUid = null)
        val mine = "${LocalRecord.TEMP_PREFIX}abc"
        session.records.mergeRemote("messages") { it + Row(mine, mapOf("text" to "まだ送れていない"), "2026-11-13T01:02:00Z") }
        fake.rows = listOf(remote("s2", "b", "2026-11-13T01:01:00Z"))
        ChatSync.pull(fake, session.records, "messages", selfUid = null)
        assertEquals("s1 は消された・送り待ちは残る", listOf("s2", mine), session.store.list("messages").map { it.id })
    }

    @Test
    fun cannotReadKeepsWhatIsShown() = runBlocking {
        val session = DesktopSession(dsl)
        val fake = FakeRemote(listOf(remote("s1", "a", "2026-11-13T01:00:00Z")))
        ChatSync.pull(fake, session.records, "messages", selfUid = null)
        fake.rows = null
        assertFalse(ChatSync.pull(fake, session.records, "messages", selfUid = null))
        assertEquals(listOf("s1"), session.store.list("messages").map { it.id })
        val throwing = object : ChatRemote {
            override suspend fun fetch(entity: String): List<Record>? = error("offline")
            override suspend fun aliases(entity: String) = emptyMap<String, String>()
        }
        assertFalse(ChatSync.pull(throwing, session.records, "messages", selfUid = null))
    }

    @Test
    fun signedInUidMarksOwnRowsFromOtherDevices() {
        val merged = ChatSync.merge(
            emptyList(),
            listOf(remote("s1", "別の端末から", "2026-11-13T01:00:00Z", author = "u-me"), remote("s2", "x", "2026-11-13T01:00:01Z")),
            emptyMap(),
            selfUid = "u-me",
        )
        assertEquals(listOf(null, "u-other"), merged.map { it.author })
    }

    @Test
    fun identicalTextFromSomeoneElseIsShownOnceSignedIn() {
        val pending = Row("${LocalRecord.TEMP_PREFIX}p", mapOf("text" to "ok"), "2026-11-13T01:00:00Z")
        val merged = ChatSync.merge(listOf(pending), listOf(remote("s1", "ok", "2026-11-13T01:00:00Z")), emptyMap(), selfUid = "u-me")
        assertEquals("自分でない人の同じ文は消さない", listOf("s1", pending.id), merged.map { it.id })
    }

    @Test
    fun sendGoesThroughTheSavePath() = runBlocking {
        val p = SessionPersistence(DesktopStore(null), "chat", client = null)
        val session = DesktopSession(dsl, persistence = p)
        val id = session.postMessage("messages", mapOf("text" to JSONValue.Str("保存の道")))
        p.idle()
        val local = p.store.records.rows("chat", "messages").single()
        assertEquals("フォームの保存と同じく端末に残る", id, local.recordId)
        assertEquals("text", local.data.keys.single())
        assertEquals("まだ送れていないので対応は無い", emptyMap<String, String>(), p.sentAliases("messages"))
        // 送れた後(SyncQueue の markSynced が残す形): サーバーの id と作成の冪等キー
        p.store.records.remove("chat", "messages", id)
        p.store.records.upsert(local.copy(recordId = "srv-1", clientId = id.removePrefix(LocalRecord.TEMP_PREFIX), pendingOp = PendingOp.NONE))
        assertEquals(mapOf("srv-1" to id), p.sentAliases("messages"))
        p.shutdown()
    }

    // MARK: - 並び・時刻・間隔

    @Test
    fun orderKeepsUndatedRowsFirstInSavedOrder() {
        val rows = listOf(
            Record("z", createdAt = "000000"),
            Record("b", createdAt = "2026-11-13T01:02:00Z"),
            Record("a", createdAt = "000001"),
            Record("c", createdAt = "2026-11-13T01:01:00.5+09:00"),
        )
        assertEquals(listOf("z", "a", "c", "b"), ChatRows.ordered(rows).map { it.id })
    }

    @Test
    fun timeLabel() {
        val zone = ZoneId.of("Asia/Tokyo")
        assertEquals("10:05", ChatRows.time("2026-11-13T01:05:00Z", zone, LocalDate.of(2026, 11, 13)))
        assertEquals("11/12 10:05", ChatRows.time("2026-11-12T01:05:00Z", zone, LocalDate.of(2026, 11, 13)))
        assertNull(ChatRows.time("000001", zone))
    }

    @Test
    fun pollIntervalWidensInBackgroundAndOnFailure() {
        assertEquals(ChatPolling.FOCUSED_MILLIS, ChatPolling.interval(focused = true, failures = 0))
        assertEquals(ChatPolling.BACKGROUND_MILLIS, ChatPolling.interval(focused = false, failures = 0))
        assertEquals(ChatPolling.FOCUSED_MILLIS * 4, ChatPolling.interval(focused = true, failures = 2))
        assertEquals(ChatPolling.MAX_MILLIS, ChatPolling.interval(focused = false, failures = 50))
        assertTrue(ChatRows.canSend(false, " a "))
        assertFalse(ChatRows.canSend(false, "  "))
        assertFalse(ChatRows.canSend(true, "a"))
    }
}
