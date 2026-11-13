package jp.kz2601.metamon.desktop.io

import jp.kz2601.metamon.dsl.records.Entity
import jp.kz2601.metamon.dsl.records.JSONValue
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/** 項目つきでコピーした記録の貼り付け(#2987): 項目名の一致・型の違い・選択肢に無い値・管理者だけの項目。 */
class RecordPasteTest {
    private fun entity(vararg fields: String, id: String = "e") =
        Entity.parse("""{"id":"$id","name":"n","fields":[${fields.joinToString(",")}]}""")

    private fun f(id: String, name: String, type: String, extra: String = "") = """{"id":"$id","name":"$name","type":"$type"$extra}"""

    private fun one(type: String, extra: String = "") = entity(f("p", "p", type, extra)).fields.first()

    private fun fv(id: String, name: String, type: String, v: JSONValue) = RecordClipboard.FieldValue(id, name, type, v)

    private fun payload(vararg fields: RecordClipboard.FieldValue, appId: String = "appA", entityId: String = "e") =
        RecordClipboard.Payload(appId, "A", entityId, "n", fields.toList())

    private val target = entity(
        f("title", "題名", "string"),
        f("pages", "ページ数", "integer"),
        f("price", "値段", "currency"),
        f("genre", "ジャンル", "enum", ",\"options\":[\"小説\",\"技術書\"]"),
        f("memo", "メモ", "text"),
        f("secret", "秘密", "string", ",\"editableBy\":\"admins\""),
    )

    @Test
    fun matchesByNameIgnoringCaseAndSpaces() {
        val r = RecordPaste.paste(payload(fv("x", " 題名 ", "string", JSONValue.Str("走れメロス"))), target, "appB")
        assertEquals(mapOf("title" to JSONValue.Str("走れメロス")), r.values)
        assertEquals(listOf("題名"), r.matched)
        assertTrue(r.unmatched.isEmpty())
    }

    @Test
    fun sameAppAndEntityMatchesById() {
        val r = RecordPaste.paste(payload(fv("title", "別の名前", "string", JSONValue.Str("A"))), target, "appA")
        assertEquals(mapOf("title" to JSONValue.Str("A")), r.values)
        // 別のアプリなら id は当てにせず、名前が違えば合わない
        val other = RecordPaste.paste(payload(fv("title", "別の名前", "string", JSONValue.Str("A"))), target, "appB")
        assertTrue(other.values.isEmpty())
        assertEquals(listOf("別の名前"), other.unmatched)
    }

    @Test
    fun unmatchedNamesKeepSourceOrderAndFallBackToId() {
        val r = RecordPaste.paste(
            payload(fv("a", "無い 1", "string", JSONValue.Str("x")), fv("b", "", "string", JSONValue.Str("y")), fv("t", "題名", "string", JSONValue.Str("z"))),
            target,
            "appB",
        )
        assertEquals(listOf("無い 1", "b"), r.unmatched)
        assertEquals(1, r.values.size)
    }

    @Test
    fun eachTargetFieldIsFilledOnce() {
        val r = RecordPaste.paste(
            payload(fv("a", "題名", "string", JSONValue.Str("1つ目")), fv("b", "題名", "string", JSONValue.Str("2つ目"))),
            target,
            "appB",
        )
        assertEquals(JSONValue.Str("1つ目"), r.values["title"])
        assertEquals(listOf("題名"), r.unmatched)
    }

    @Test
    fun typesConvertOnlyWhenTheyCan() {
        // 数字だけの文字 → 整数。小数は整数にできない
        assertEquals(JSONValue.Num(32.0), RecordPaste.coerce(JSONValue.Str(" 32 "), "string", one("integer")))
        assertNull(RecordPaste.coerce(JSONValue.Num(1.5), "number", one("integer")))
        // currency は四捨五入
        assertEquals(JSONValue.Num(2.0), RecordPaste.coerce(JSONValue.Num(1.6), "number", one("currency")))
        // 数 → 文字
        assertEquals(JSONValue.Str("32"), RecordPaste.coerce(JSONValue.Num(32.0), "integer", one("string")))
        // 型の違い: 真偽 → 文字・文字 → 真偽は合わない
        assertNull(RecordPaste.coerce(JSONValue.Bool(true), "boolean", one("string")))
        assertNull(RecordPaste.coerce(JSONValue.Str("true"), "string", one("boolean")))
        // 数でない文字 → 数は合わない
        assertNull(RecordPaste.coerce(JSONValue.Str("abc"), "string", one("number")))
    }

    @Test
    fun enumValueMustBeAnOption() {
        val genre = target.fields.first { it.id == "genre" }
        assertEquals(JSONValue.Str("小説"), RecordPaste.coerce(JSONValue.Str("小説"), "enum", genre))
        assertNull(RecordPaste.coerce(JSONValue.Str("漫画"), "enum", genre))
        assertEquals(JSONValue.Str("技術書"), RecordPaste.coerce(JSONValue.Str("技術書"), "string", genre))
    }

    @Test
    fun textLengthIsLimited() {
        val title = target.fields.first { it.id == "title" }
        assertNull(RecordPaste.coerce(JSONValue.Str("あ".repeat(4001)), "text", title))
        assertEquals(JSONValue.Str("あ".repeat(4000)), RecordPaste.coerce(JSONValue.Str("あ".repeat(4000)), "text", title))
    }

    @Test
    fun datesAndLocations() {
        assertEquals(JSONValue.Str("2026-11-03"), RecordPaste.coerce(JSONValue.Str("2026-11-03"), "date", one("date")))
        assertNull("日付 → 日時は合わない", RecordPaste.coerce(JSONValue.Str("2026-11-03"), "date", one("datetime")))
        assertTrue("日時 → 日付", RecordPaste.coerce(JSONValue.Str("2026-11-03T03:00:00Z"), "datetime", one("date")) is JSONValue.Str)
        assertNull(RecordPaste.coerce(JSONValue.Str("not a date"), "date", one("date")))
        assertEquals(JSONValue.Str("35.68,139.76"), RecordPaste.coerce(JSONValue.Str("35.68, 139.76"), "location", one("location")))
        assertNull(RecordPaste.coerce(JSONValue.Str("91,0"), "location", one("location")))
        assertNull(RecordPaste.coerce(JSONValue.Str("0,181"), "location", one("location")))
    }

    @Test
    fun adminOnlyFieldsNeedAdmin() {
        val p = payload(fv("s", "秘密", "string", JSONValue.Str("x")))
        assertTrue(RecordPaste.paste(p, target, "appB").values.isEmpty())
        assertEquals(JSONValue.Str("x"), RecordPaste.paste(p, target, "appB", isAdmin = true).values["secret"])
    }

    @Test
    fun excludedAndUnknownTypesAreDropped() {
        val r = RecordPaste.paste(
            payload(fv("p", "写真", "photo", JSONValue.Str("photo:1")), fv("z", "題名", "weird", JSONValue.Str("x"))),
            target,
            "appB",
        )
        assertTrue(r.values.isEmpty())
        assertEquals(listOf("写真", "題名"), r.unmatched)
    }

    @Test
    fun hiddenTargetFieldsAreNotFilled() {
        val e = entity(f("h", "題名", "string", ",\"hidden\":true"))
        assertTrue(RecordPaste.paste(payload(fv("a", "題名", "string", JSONValue.Str("x"))), e, "appB").values.isEmpty())
    }
}
