package jp.kz2601.metamon.renderer

import jp.kz2601.metamon.dsl.ComponentType
import jp.kz2601.metamon.dsl.records.Entity
import jp.kz2601.metamon.dsl.records.Field
import jp.kz2601.metamon.dsl.records.FieldType
import jp.kz2601.metamon.dsl.records.JSONValue
import jp.kz2601.metamon.renderer.components.lists.LeafData
import jp.kz2601.metamon.renderer.components.social.AuthorNameHost
import jp.kz2601.metamon.renderer.components.social.ChatThreadSend
import jp.kz2601.metamon.renderer.registry.ComponentRegistry
import kotlinx.coroutines.runBlocking
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import java.time.LocalDate
import java.time.ZoneId

/** みんなで話す部品(#875): 登録・送るかどうか・名前を聞くか・時刻・名簿に無い人の出し方。 */
class SocialLeavesTest {
    private val shared = Entity(id = "messages", scope = "shared", fields = listOf(Field("text"), Field("room", defaultValue = JSONValue.Str("general"))))
    private val notes = Entity(id = "notes", fields = listOf(Field("text")))

    private class FakeRuntime(private val ask: Boolean) : DSLRuntime, AuthorNameHost {
        val added = mutableListOf<Pair<String, Map<String, JSONValue>>>()
        val answers = mutableListOf<String?>()
        val events = mutableListOf<String>()
        override val formMembers = listOf(FormMember("u1", "たろう"), FormMember("u2", ""))

        override fun onVoiceInput(componentId: String?) = Unit
        override fun needsAuthorName(entity: String) = ask && entity == "messages"
        override fun answerAuthorName(name: String?) {
            answers += name
            events += "answer"
        }
        override suspend fun addRecord(entity: String, data: Map<String, JSONValue>): Boolean {
            added += entity to data
            events += "add"
            return true
        }
    }

    @Test
    fun chatThreadIsRegistered() {
        assertTrue(ComponentType.CHAT_THREAD.wire in ComponentRegistry.standard.registeredTypes)
    }

    @Test
    fun blankDraftDoesNotSend() {
        assertEquals(ChatThreadSend.Plan.Empty, ChatThreadSend.plan(FakeRuntime(ask = true), shared, "   ", "text", null))
    }

    @Test
    fun firstSharedPostAsksNameThenSavesWithDefaults() = runBlocking {
        val rt = FakeRuntime(ask = true)
        val plan = ChatThreadSend.plan(rt, shared, " やあ ", "text", null)
        plan as ChatThreadSend.Plan.AskName
        assertEquals(" やあ ", plan.pending.text) // 閉じた時に入力欄へ戻す元の文
        assertEquals(mapOf("room" to JSONValue.Str("general"), "text" to JSONValue.Str("やあ")), plan.pending.data)
        assertTrue(ChatThreadSend.post(rt, "messages", plan.pending.data, ChatThreadSend.NameAnswer("はなこ")))
        assertEquals(listOf<String?>("はなこ"), rt.answers)
        assertEquals(listOf("answer", "add"), rt.events) // 名前を残してから送る(送る行に名前が付く)
    }

    @Test
    fun privateEntityOrAskedSendsDirectly() = runBlocking {
        val rt = FakeRuntime(ask = true)
        val plan = ChatThreadSend.plan(rt, notes, "memo", "text", null)
        plan as ChatThreadSend.Plan.Send
        ChatThreadSend.post(rt, "notes", plan.pending.data, null)
        assertTrue(rt.answers.isEmpty())
        assertEquals(listOf("notes" to mapOf<String, JSONValue>("text" to JSONValue.Str("memo"))), rt.added)
        assertTrue(ChatThreadSend.plan(FakeRuntime(ask = false), shared, "hi", "text", null) is ChatThreadSend.Plan.Send)
        // 名前を聞けない runtime(プレビュー)はそのまま送る
        assertTrue(ChatThreadSend.plan(NoopDSLRuntime, shared, "hi", "text", null) is ChatThreadSend.Plan.Send)
    }

    @Test
    fun anonymousAnswerIsRecorded() = runBlocking {
        val rt = FakeRuntime(ask = true)
        ChatThreadSend.post(rt, "messages", mapOf("text" to JSONValue.Str("x")), ChatThreadSend.NameAnswer(null))
        assertEquals(listOf<String?>(null), rt.answers)
    }

    @Test
    fun timeIsClockTodayAndDateOtherwise() {
        val tokyo = ZoneId.of("Asia/Tokyo")
        val today = LocalDate.of(2026, 10, 24)
        assertEquals("19:05", ChatThreadSend.time("2026-10-24T10:05:00Z", tokyo, today))
        assertEquals("10/23 9:05", ChatThreadSend.time("2026-10-23T00:05:00Z", tokyo, today))
        assertNull(ChatThreadSend.time(null, tokyo, today))
    }

    @Test
    fun unknownOrUnnamedMemberShowsParticipant() {
        val rt = FakeRuntime(ask = false)
        val data = LeafData(rt, emptyMap())
        val member = Field("who", type = FieldType.MEMBER)
        assertEquals("たろう", data.display(JSONValue.Str("u1"), member))
        assertEquals(LeafData.MEMBER_FALLBACK, data.display(JSONValue.Str("u2"), member)) // 表示名が空
        assertEquals("参加者", data.display(JSONValue.Str("gone"), member)) // 名簿に無い(iOS と同じ・#1076)
        assertEquals("", data.display(JSONValue.Null, member))
    }
}
