package jp.kz2601.metamon.dsl.records

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import java.time.Instant

/** chatThread の並び・自分の判定・送るデータ(iOS の ChatThreadTests と同じ決まり・#875)。 */
class ChatThreadTest {
    private val shared = Entity(id = "messages", scope = "shared", fields = listOf(Field("text"), Field("photo", type = FieldType.PHOTO)))
    private val private = Entity(id = "notes", fields = listOf(Field("text")))

    @Test
    fun orderedIsOldestFirstWithUndatedOnTopAndIdTieBreak() {
        val rows = listOf(
            Record("c", createdAt = "2026-10-24T10:05:00Z"),
            Record("a", createdAt = "2026-10-24T10:00:00.500Z"),
            Record("z"),
            Record("b", createdAt = "2026-10-24T10:05:00Z"),
            Record("y", createdAt = "2026-10-24T19:00:00+09:00"), // = 10:00:00Z
        )
        assertEquals(listOf("z", "y", "a", "b", "c"), ChatThread.ordered(rows).map { it.id })
    }

    @Test
    fun isMineComparesAuthorWithUid() {
        assertTrue(ChatThread.isMine(Record("1", author = "me"), shared, "me"))
        assertFalse(ChatThread.isMine(Record("1", author = "you"), shared, "me"))
        // 未送信の自分の分(author なし)は自分・private は全部自分・uid が分からなければ他人の分は相手
        assertTrue(ChatThread.isMine(Record("1"), shared, "me"))
        assertTrue(ChatThread.isMine(Record("1", author = "you"), private, "me"))
        assertFalse(ChatThread.isMine(Record("1", author = "you"), shared, null))
    }

    @Test
    fun senderNameIsShownOncePerRun() {
        val a1 = Record("1", author = "a")
        val a2 = Record("2", author = "a")
        val b = Record("3", author = "b")
        val mine = Record("4", author = "me")
        assertTrue(ChatThread.showsSender(a1, null, shared, "me"))
        assertFalse(ChatThread.showsSender(a2, a1, shared, "me"))
        assertTrue(ChatThread.showsSender(b, a2, shared, "me"))
        assertTrue(ChatThread.showsSender(a1, mine, shared, "me"))
        assertFalse(ChatThread.showsSender(mine, a1, shared, "me"))
        assertFalse(ChatThread.showsSender(a1, null, private, "me"))
    }

    @Test
    fun messageTrimsAndNeedsTextOrPhoto() {
        assertNull(ChatThread.message("  \n ", null, "text", null))
        assertNull(ChatThread.message("", "data:x", "text", null)) // photoField が無ければ写真は送らない
        assertEquals(mapOf("text" to JSONValue.Str("hi")), ChatThread.message("  hi \n", null, "text", null))
        assertEquals(mapOf("photo" to JSONValue.Str("data:x")), ChatThread.message("", "data:x", "text", "photo"))
        assertEquals(
            mapOf("room" to JSONValue.Str("a"), "text" to JSONValue.Str("hi")),
            ChatThread.message("hi", null, "text", null, base = mapOf("room" to JSONValue.Str("a"))),
        )
    }

    @Test
    fun defaultDataTakesOnlyStoredFieldsWithDefaults() {
        val e = Entity(
            id = "m",
            fields = listOf(
                Field("text", defaultValue = JSONValue.Str("x")),
                Field("room", defaultValue = JSONValue.Str("general")),
                Field("note"),
                Field("at", type = FieldType.DATETIME, defaultValue = JSONValue.Str("now")),
            ),
        )
        val out = ChatThread.defaultData(e, listOf("text"), Instant.parse("2026-10-24T10:00:00Z"))
        assertEquals(mapOf("room" to JSONValue.Str("general"), "at" to JSONValue.Str("2026-10-24T10:00:00Z")), out)
    }

    @Test
    fun dateReadsRfc3339WithAndWithoutFraction() {
        assertEquals(Instant.parse("2026-10-24T10:00:00Z"), ChatThread.date("2026-10-24T10:00:00Z"))
        assertEquals(Instant.parse("2026-10-24T10:00:00.250Z"), ChatThread.date("2026-10-24T10:00:00.250Z"))
        assertNull(ChatThread.date("yesterday"))
        assertNull(ChatThread.date(null))
    }
}
