package jp.kz2601.metamon.desktop.io

import jp.kz2601.metamon.dsl.records.Entity
import jp.kz2601.metamon.dsl.records.JSONValue
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/** 記録のコピーの形(#2986): 載せる項目・数と日付の文字・JSON の読み戻し・クリップボードの 2 つの型。 */
class RecordClipboardTest {
    private val books = Entity.parse(
        """
        {"id":"books","name":"本","fields":[
          {"id":"title","name":"題名","type":"string"},
          {"id":"author","name":"著者","type":"string"},
          {"id":"pages","name":"ページ数","type":"integer"},
          {"id":"rating","name":"評価","type":"number"},
          {"id":"read","name":"読了","type":"boolean"},
          {"id":"boughtOn","name":"買った日","type":"date"},
          {"id":"finishedAt","name":"読み終えた日時","type":"datetime"},
          {"id":"genre","name":"ジャンル","type":"enum","options":["小説","技術書"]},
          {"id":"memo","name":"メモ","type":"text"},
          {"id":"answer","name":"答え","type":"string","hidden":true},
          {"id":"cover","name":"表紙","type":"photo"},
          {"id":"shelf","name":"本棚","type":"ref","refEntity":"shelves"},
          {"id":"double","name":"倍","type":"number","computed":{"op":"mul","args":[{"ref":"pages"},2]}}
        ]}
        """.trimIndent(),
    )

    private val data: Map<String, JSONValue> = mapOf(
        "title" to JSONValue.Str("走れメロス"),
        "author" to JSONValue.Str("太宰治"),
        "pages" to JSONValue.Num(32.0),
        "rating" to JSONValue.Num(4.5),
        "read" to JSONValue.Bool(true),
        "boughtOn" to JSONValue.Str("2026-11-01"),
        "finishedAt" to JSONValue.Str("2026-11-03T03:00:00Z"),
        "genre" to JSONValue.Str("小説"),
        "memo" to JSONValue.Str("秘密のメモ"),
        "answer" to JSONValue.Str("秘密"),
        "cover" to JSONValue.Str("photo:abc"),
        "shelf" to JSONValue.Str("rec-1"),
        "double" to JSONValue.Num(64.0),
    )

    private fun payload(encrypted: Set<String> = setOf("memo")) =
        RecordClipboard.make("reading", "読書メモ", books, data, encrypted)!!

    @Test
    fun copyExcludesHiddenEncryptedMediaRefAndComputed() {
        val ids = payload().fields.map { it.id }
        assertEquals(listOf("title", "author", "pages", "rating", "read", "boughtOn", "finishedAt", "genre"), ids)
        assertFalse("暗号化した項目の平文は載せない", ids.contains("memo"))
        assertFalse(ids.contains("answer"))
        assertFalse(ids.contains("double"))
    }

    @Test
    fun copySkipsNullAndMistypedValues() {
        val d = data + mapOf("author" to JSONValue.Null, "pages" to JSONValue.Str("三十二"))
        val p = RecordClipboard.make("a", "A", books, d, setOf("memo"))!!
        assertFalse(p.fields.any { it.id == "author" || it.id == "pages" })
    }

    @Test
    fun nothingCopyableIsNull() {
        val d = mapOf("memo" to JSONValue.Str("x"), "cover" to JSONValue.Str("photo:1"))
        assertNull(RecordClipboard.make("a", "A", books, d, setOf("memo")))
    }

    @Test
    fun plainTextHasNameAndValueLines() {
        val text = RecordClipboard.plainText(payload())
        assertEquals(
            listOf(
                "題名: 走れメロス", "著者: 太宰治", "ページ数: 32", "評価: 4.5", "読了: true",
                "買った日: 2026-11-01", "読み終えた日時: 2026-11-03T03:00:00Z", "ジャンル: 小説",
            ),
            text.lines(),
        )
        assertFalse(text.contains("秘密"))
    }

    @Test
    fun numberText() {
        assertEquals("32", RecordClipboard.rawText(JSONValue.Num(32.0)))
        assertEquals("4.5", RecordClipboard.rawText(JSONValue.Num(4.5)))
        assertEquals("-3", RecordClipboard.rawText(JSONValue.Num(-3.0)))
        assertEquals("a, b", RecordClipboard.rawText(JSONValue.Arr(listOf(JSONValue.Str("a"), JSONValue.Str("b")))))
    }

    @Test
    fun plainTextSkipsEmptyValuesAndUsesDisplay() {
        val p = RecordClipboard.Payload(
            "a", "A", "e", "E",
            listOf(
                RecordClipboard.FieldValue("x", "空", "string", JSONValue.Str("  ")),
                RecordClipboard.FieldValue("y", "", "string", JSONValue.Str("名前なし")),
                RecordClipboard.FieldValue("z", "金額", "currency", JSONValue.Num(1200.0)),
            ),
        )
        val text = RecordClipboard.plainText(p) { if (it.id == "z") "¥1,200" else RecordClipboard.rawText(it.value) }
        assertEquals("名前なし\n金額: ¥1,200", text)
    }

    @Test
    fun jsonRoundTrip() {
        val p = payload()
        val json = RecordClipboard.encode(p)!!
        assertTrue(json, json.contains("\"value\":32"))
        assertFalse(json, json.contains("32.0"))
        assertEquals(p, RecordClipboard.decode(json))
    }

    @Test
    fun decodeRejectsBrokenInput() {
        val json = RecordClipboard.encode(payload())!!
        assertNull(RecordClipboard.decode("not json"))
        assertNull(RecordClipboard.decode("[]"))
        assertNull("新しすぎる版", RecordClipboard.decode(json.replace("\"v\":1", "\"v\":2")))
        assertNull("entityId が空", RecordClipboard.decode(json.replace("\"entityId\":\"books\"", "\"entityId\":\"\"")))
        assertNull("大きすぎる", RecordClipboard.decode(" ".repeat(RecordClipboard.MAX_BYTES + 1) + json))
    }

    @Test
    fun decodeDropsBrokenFieldsAndClipsNames() {
        val long = "あ".repeat(300)
        val json = """{"v":1,"appId":"a","appName":"$long","entityId":"e","entityName":"E","fields":[
            {"id":"","name":"x","type":"string","value":"v"},
            {"id":"ok","name":"$long","type":"string","value":"v"},
            {"id":"nov","name":"n","type":"string"}]}"""
        val p = RecordClipboard.decode(json)!!
        assertEquals(listOf("ok"), p.fields.map { it.id })
        assertEquals(100, p.appName.length)
        assertEquals(100, p.fields.single().name.length)
    }

    @Test
    fun encodeRefusesOversizedPayload() {
        val big = RecordClipboard.Payload(
            "a", "A", "e", "E",
            listOf(RecordClipboard.FieldValue("t", "T", "text", JSONValue.Str("x".repeat(RecordClipboard.MAX_BYTES)))),
        )
        assertNull(RecordClipboard.encode(big))
        assertNull(RecordClipboard.clip(big))
    }

    @Test
    fun clipCarriesBothTextAndJson() {
        val p = payload()
        val clip = RecordClipboard.clip(p)!!
        assertEquals(clip.text, clip.getTransferData(java.awt.datatransfer.DataFlavor.stringFlavor))
        assertTrue(clip.isDataFlavorSupported(RecordClipboard.jsonFlavor))
        assertEquals(p, RecordClipboard.read(clip))
    }

    @Test
    fun textOnlyClipboardIsNotARecord() {
        assertNull(RecordClipboard.read(java.awt.datatransfer.StringSelection("題名: x")))
        assertNotNull(RecordClipboard.read(RecordClipboard.clip(payload())!!))
    }
}
