package jp.kz2601.metamon.dsl.ai

import jp.kz2601.metamon.dsl.AIChatSaveTo
import jp.kz2601.metamon.dsl.records.Entity
import jp.kz2601.metamon.dsl.records.Field
import jp.kz2601.metamon.dsl.records.FieldType
import jp.kz2601.metamon.dsl.records.JSONValue
import jp.kz2601.metamon.dsl.records.Record
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import java.time.Instant

/** aiChat の並び・依頼・保存の data・saveTo・参考の記録(iOS の AIChatTests / AIChatSaveToTests / AIChatMaterialTests)。 */
class AiChatTest {
    private fun rec(id: String, text: String, role: String? = null, at: String? = null, author: String? = null, unsent: Boolean = false) =
        Record(
            id = id,
            data = buildMap {
                put("text", JSONValue.Str(text))
                role?.let { put("role", JSONValue.Str(it)) }
            },
            createdAt = at,
            author = author,
            unsent = unsent,
        )

    @Test
    fun turnsReadRoleFieldOrAlternateForOldDsl() {
        val rs = listOf(
            rec("b", "こんにちは", "assistant", "2026-01-01T00:00:02Z"),
            rec("a", "やあ", "user", "2026-01-01T00:00:01Z"),
            rec("c", "   ", "user", "2026-01-01T00:00:03Z"),
        )
        val withRole = AiChat.turns(rs, "text", "role")
        assertEquals(listOf("a", "b"), withRole.map { it.id })
        assertEquals(listOf(AiChat.Role.USER, AiChat.Role.ASSISTANT), withRole.map { it.role })
        // roleField 無し: 空の本文は数えずに交互
        val old = AiChat.turns(rs + rec("d", "次", at = "2026-01-01T00:00:04Z"), "text", null)
        assertEquals(listOf(AiChat.Role.USER, AiChat.Role.ASSISTANT, AiChat.Role.USER), old.map { it.role })
    }

    @Test
    fun unsavedRecordsSortLast() {
        val rs = listOf(rec("new", "x"), rec("old", "y", at = "2026-01-01T00:00:00Z"))
        assertEquals(listOf("old", "new"), AiChat.ordered(rs).map { it.id })
    }

    @Test
    fun ownRecordsOnly() {
        val mine = rec("1", "a", author = "me")
        val other = rec("2", "b", author = "you")
        val ruleMade = rec("3", "c")
        val pending = rec("4", "d", unsent = true)
        assertEquals(listOf("1", "4"), AiChat.own(listOf(mine, other, ruleMade, pending), "me", shared = true).map { it.id })
        assertEquals(listOf("1", "3", "4"), AiChat.own(listOf(mine, other, ruleMade, pending), "me", shared = false).map { it.id })
        assertFalse(AiChat.isOwn(mine, null, shared = false))
    }

    @Test
    fun requestKeepsRecentTurnsAndStartsWithUser() {
        val history = (1..30).map { i ->
            AiChat.Turn("$i", if (i % 2 == 1) AiChat.Role.USER else AiChat.Role.ASSISTANT, "発言$i")
        }
        val r = AiChat.request(" 先生です ", history, " 次の質問 ", long = true, locale = "ja")
        assertEquals("chat", r.task)
        assertEquals("long", r.length)
        assertEquals("先生です", r.instruction)
        assertTrue(r.isChat)
        val ms = r.messages!!
        assertTrue(ms.size <= AiChat.MAX_TURNS)
        assertEquals("user", ms.first().role)
        assertEquals(AiTextMessage("user", "次の質問"), ms.last())
    }

    @Test
    fun requestFitsMaterialIntoTheBudget() {
        val big = "あ".repeat(5000) // 15 KB
        val history = listOf(AiChat.Turn("1", AiChat.Role.USER, big), AiChat.Turn("2", AiChat.Role.ASSISTANT, "ok"))
        val material = AiTextMaterial("e", "記録", "x".repeat(8 * 1024))
        val r = AiChat.request("", history, "質問", long = false, locale = "ja", material = material)
        // 会話は 16 KB - 8 KB に収める: 大きい発言は落ち、先頭の assistant も落とす
        assertEquals(listOf("質問"), r.messages!!.map { it.text })
        assertNull(r.length)
        assertEquals(material, r.material)
    }

    @Test
    fun recordData() {
        val d = AiChat.record(" 返事 ", AiChat.Role.ASSISTANT, "text", "role", mapOf("day" to JSONValue.Str("2026-01-01")))!!
        assertEquals(JSONValue.Str("返事"), d["text"])
        assertEquals(JSONValue.Str("assistant"), d["role"])
        assertEquals(JSONValue.Str("2026-01-01"), d["day"])
        assertNull(AiChat.record("  ", AiChat.Role.USER, "text", null))
        assertFalse(AiChat.record("a", AiChat.Role.USER, "text", null)!!.containsKey("role"))
    }

    @Test
    fun spokenReplyIsTheNewestMatchingAssistantTurn() {
        val turns = listOf(
            AiChat.Turn("1", AiChat.Role.ASSISTANT, "はい"),
            AiChat.Turn("2", AiChat.Role.USER, "はい"),
            AiChat.Turn("3", AiChat.Role.ASSISTANT, "はい"),
        )
        assertEquals("3", AiChat.spokenReply(turns, " はい ")?.id)
        assertNull(AiChat.spokenReply(turns, "いいえ"))
    }

    private val meal = Entity(
        id = "meal",
        name = "食事",
        fields = listOf(
            Field("food", "料理", FieldType.STRING, required = true),
            Field("kcal", "カロリー", FieldType.NUMBER, unit = "kcal"),
            Field("day", "日付", FieldType.DATE),
            Field("kind", "種類", FieldType.ENUM, options = listOf("朝", "昼", "夜")),
            Field("note", "メモ", FieldType.TEXT),
        ),
    )

    @Test
    fun saveToRequestDescribesFields() {
        val saveTo = AIChatSaveTo("meal", listOf("food", "kcal", "kind"))
        val r = AiChatSaveToRules.request(saveTo, meal, Instant.parse("2026-03-04T03:00:00Z"))!!
        assertEquals("食事", r.name)
        assertEquals(10, r.today.length)
        assertEquals(listOf("food", "kcal", "kind"), r.fields.map { it.id })
        assertEquals(true, r.fields[0].required)
        assertEquals("kcal", r.fields[1].unit)
        assertEquals(listOf("朝", "昼", "夜"), r.fields[2].options)
        // 使えない field(photo など・知らない id)が 1 つでもあれば案を付けない
        assertNull(AiChatSaveToRules.request(AIChatSaveTo("meal", listOf("nope")), meal))
        assertNull(AiChatSaveToRules.request(AIChatSaveTo("other", listOf("food")), meal))
    }

    @Test
    fun saveToRecordChecksValues() {
        val saveTo = AIChatSaveTo("meal", listOf("food", "kcal", "day", "kind"))
        val suggest = mapOf(
            "food" to JSONValue.Str(" カレー "),
            "kcal" to JSONValue.Num(800.0),
            "day" to JSONValue.Str("2026-13-40"), // 日付として読めない
            "kind" to JSONValue.Str("おやつ"), // 選択肢に無い
        )
        val data = AiChatSaveToRules.record(saveTo, suggest, meal)!!
        assertEquals(JSONValue.Str("カレー"), data["food"])
        assertEquals(JSONValue.Num(800.0), data["kcal"])
        assertTrue(data["kind"] != JSONValue.Str("おやつ")) // 案は捨てて新しいフォームの既定値
        assertEquals(listOf("food", "kcal"), AiChatSaveToRules.values(saveTo, suggest, meal).map { it.first.id })
        // required の料理が埋まらなければカードを出さない
        assertNull(AiChatSaveToRules.record(saveTo, mapOf("kcal" to JSONValue.Num(1.0)), meal))
        assertNull(AiChatSaveToRules.record(saveTo, mapOf("kcal" to JSONValue.Num(2e9)), meal))
    }

    @Test
    fun materialPrefersRelatedRecordsAndSkipsHidden() {
        val diary = Entity(
            id = "diary",
            name = "日記",
            fields = listOf(Field("body", "本文", FieldType.TEXT), Field("secret", "秘密", FieldType.STRING, hidden = true)),
        )
        fun d(id: String, body: String, at: String) =
            Record(id, mapOf("body" to JSONValue.Str(body), "secret" to JSONValue.Str("内緒")), createdAt = at, updatedAt = at)
        val rs = listOf(
            d("1", "カレーを食べた", "2026-01-01T00:00:00Z"),
            d("2", "散歩した", "2026-01-03T00:00:00Z"),
            d("3", "本を読んだ\n面白かった", "2026-01-02T00:00:00Z"),
        )
        val m = AiChatMaterial.request(diary, rs, uid = null, limit = 2, query = "カレーはおいしい")!!
        assertEquals("日記", m.name)
        val lines = m.text.lines()
        assertEquals(2, lines.size)
        assertEquals("- 本文: カレーを食べた", lines[0]) // 語が重なる記録が先
        assertEquals("- 本文: 散歩した", lines[1]) // 残りは新しい順
        assertFalse(m.text.contains("内緒"))
        // 改行は 1 行に
        val all = AiChatMaterial.request(diary, rs, uid = null, limit = null)!!
        assertTrue(all.text.contains("本を読んだ ⏎ 面白かった"))
    }

    @Test
    fun relevanceTokens() {
        assertEquals(setOf("カレ", "レー"), AiChatRelevance.tokens("カレー"))
        assertEquals(setOf("猫"), AiChatRelevance.tokens("猫"))
        assertEquals(setOf("hello", "ab"), AiChatRelevance.tokens("Hello, a ab"))
    }
}
