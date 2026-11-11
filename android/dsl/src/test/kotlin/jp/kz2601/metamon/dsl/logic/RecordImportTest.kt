package jp.kz2601.metamon.dsl.logic

import jp.kz2601.metamon.dsl.logic.ImportRowFailure.Kind
import jp.kz2601.metamon.dsl.logic.RecordImport.CellValue
import jp.kz2601.metamon.dsl.records.Entity
import jp.kz2601.metamon.dsl.records.Field
import jp.kz2601.metamon.dsl.records.FieldCoding
import jp.kz2601.metamon.dsl.records.FieldType
import jp.kz2601.metamon.dsl.records.Fixtures
import jp.kz2601.metamon.dsl.records.JSONValue
import jp.kz2601.metamon.dsl.records.Record
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test
import java.time.Instant
import java.util.Locale

/** iOS の RecordImportTests: CSV の読み取り(引用符・改行・BOM・Shift_JIS)、列の当て方、型の変換、上限、export の読み戻し、本文の記録。 */
class RecordImportTest {
    private val savedZone = FieldCoding.zone

    @Before
    fun setUp() {
        FieldCoding.zone = Fixtures.TOKYO
    }

    @After
    fun tearDown() {
        FieldCoding.zone = savedZone
    }

    private val member = Entity.parse(
        """
        {"id":"member","name":"部員","titleField":"name","fields":[
          {"id":"name","name":"名前","type":"string","required":true},
          {"id":"grade","name":"学年","type":"integer"},
          {"id":"fee","name":"部費","type":"currency","unit":"円"},
          {"id":"height","name":"身長","type":"number","unit":"cm"},
          {"id":"active","name":"在籍","type":"boolean"},
          {"id":"part","name":"パート","type":"enum","options":["前衛","後衛"]},
          {"id":"joined","name":"入部日","type":"date"},
          {"id":"photo","name":"写真","type":"photo"},
          {"id":"label","name":"表示","type":"string","computed":{"ref":"name"}}
        ]}
        """,
    )

    /** 2026-11-12 12:00 JST */
    private val now = Instant.ofEpochSecond(1_794_452_400)

    private val noRef: (Field, String) -> String? = { _, _ -> null }

    private fun plan(csv: String, existing: Int = 0) = RecordImport.plan(CsvReader.parse(csv), member, emptyList(), existing, now, noRef)

    private fun value(v: JSONValue) = CellValue.Value(v)
    private val invalid = CellValue.Invalid(Kind.INVALID)

    // MARK: - CSV の読み取り

    @Test
    fun parseQuotesNewlinesAndEscapes() {
        val rows = CsvReader.parse("a,\"b,c\",\"he said \"\"hi\"\"\"\r\n\"1行目\n2行目\",,x\n")
        assertEquals(listOf(listOf("a", "b,c", "he said \"hi\""), listOf("1行目\n2行目", "", "x")), rows)
    }

    @Test
    fun parseSkipsBlankLinesAndHandlesCrAndNoTrailingNewline() {
        assertEquals(listOf(listOf("a", "b"), listOf("c", "d")), CsvReader.parse("a,b\r\rc,d"))
        assertEquals(listOf(listOf("a", "b"), listOf("c", "")), CsvReader.parse("a,b\n\n\nc,"))
        assertEquals(emptyList<List<String>>(), CsvReader.parse(""))
    }

    @Test
    fun decodeUtf8WithBomAndShiftJis() {
        val bom = CsvWriter.BOM + "名前,学年\r\n".toByteArray(Charsets.UTF_8)
        assertEquals("名前,学年\r\n", CsvReader.decode(bom))
        assertEquals(listOf(listOf("名前", "学年")), CsvReader.parse(CsvReader.decode(bom)!!))
        val sjis = "名前,学年\r\n山田,2\r\n".toByteArray(charset("Shift_JIS"))
        assertEquals("名前,学年\r\n山田,2\r\n", CsvReader.decode(sjis))
        assertEquals(listOf(listOf("a", "b")), CsvReader.parse("﻿a,b"))
    }

    // MARK: - 列の当て方

    @Test
    fun columnsByNameIdAndMap() {
        val cols = RecordImport.columns(listOf("氏名", " 学年 ", "GRADE", "fee", "写真", "表示", "メモ"), member, listOf(CsvColumnMapping("氏名", "name")))
        assertEquals("name", cols[0]?.id)
        assertEquals("grade", cols[1]?.id)
        assertNull(cols[2])
        assertEquals("fee", cols[3]?.id)
        assertNull(cols[4])
        assertNull(cols[5])
        assertNull(cols[6])
    }

    @Test
    fun mapToUnknownFieldIsIgnored() {
        assertEquals("name", RecordImport.columns(listOf("名前"), member, listOf(CsvColumnMapping("名前", "zzz")))[0]?.id)
    }

    // MARK: - 型の変換

    @Test
    fun valueConversions() {
        fun v(s: String, id: String) = RecordImport.value(s, member.field(id)!!, now, noRef)
        assertEquals(value(JSONValue.Num(2.0)), v("２", "grade"))
        assertEquals(value(JSONValue.Num(3.0)), v("2.6", "grade"))
        assertEquals(value(JSONValue.Num(1200.0)), v("¥1,200", "fee"))
        assertEquals(value(JSONValue.Num(-500.0)), v("-¥500", "fee"))
        assertEquals(value(JSONValue.Num(-500.0)), v("'-¥500", "fee"))
        assertEquals(value(JSONValue.Num(1200.0)), v("1,200 円", "fee"))
        assertEquals(value(JSONValue.Num(172.5)), v("172.5 cm", "height"))
        assertEquals(invalid, v("abc", "grade"))
        assertEquals(value(JSONValue.Bool(true)), v("はい", "active"))
        assertEquals(value(JSONValue.Bool(false)), v("No", "active"))
        assertEquals(invalid, v("たぶん", "active"))
        assertEquals(value(JSONValue.Str("後衛")), v("後衛", "part"))
        assertEquals(invalid, v("中衛", "part"))
        assertEquals(CellValue.Empty, v("", "grade"))
        assertEquals(CellValue.Empty, v("—", "grade"))
        assertEquals(value(JSONValue.Str("=SUM(A1)")), v("'=SUM(A1)", "name"))
        assertEquals(CellValue.Invalid(Kind.TOO_LONG), v("あ".repeat(4001), "name"))
    }

    @Test
    fun dates() {
        val f = member.field("joined")!!
        for (s in listOf("2026-04-08", "2026/4/8", "2026/4/8(水)", "2026年4月8日", "4/8(水)", "Wed, Apr 8", "Apr 8, 2026")) {
            assertEquals(s, value(JSONValue.Str("2026-04-08")), RecordImport.value(s, f, now, noRef))
        }
        assertEquals(invalid, RecordImport.value("2/30", f, now, noRef))
        val dt = Field("at", "日時", FieldType.DATETIME)
        val read = RecordImport.value("2026/4/8(水) 14:30", dt, now, noRef) as CellValue.Value
        val d = FieldCoding.date(read.value)
        assertNotNull(d)
        assertEquals("2026-04-08", FieldCoding.encode(d!!, Field("d", "d", FieldType.DATE)))
    }

    @Test
    fun refsByTitle() {
        val e = Entity.parse("""{"id":"t","name":"T","fields":[{"id":"who","name":"担当","type":"ref","refEntity":"m"},{"id":"all","name":"参加","type":"refs","refEntity":"m"}]}""")
        val ids = mapOf("山田" to "r1", "佐藤" to "r2")
        val look: (Field, String) -> String? = { _, name -> ids[name] }
        assertEquals(value(JSONValue.Str("r1")), RecordImport.value("山田", e.field("who")!!, refId = look))
        assertEquals(invalid, RecordImport.value("鈴木", e.field("who")!!, refId = look))
        assertEquals(value(JSONValue.Arr(listOf(JSONValue.Str("r1"), JSONValue.Str("r2")))), RecordImport.value("山田、佐藤", e.field("all")!!, refId = look))
        assertEquals(CellValue.Empty, RecordImport.value("全員", e.field("all")!!, refId = look))
    }

    // MARK: - 計画

    @Test
    fun planCountsFailuresAndFillsDefaults() {
        val p = plan("名前,学年,パート,メモ\n山田,1,前衛,x\n,2,後衛,\n佐藤,二,後衛,\n鈴木,,,\n")
        assertEquals(listOf("名前", "学年", "パート"), p.matched.map { it.second })
        assertEquals(listOf("メモ"), p.unused)
        assertEquals(2, p.rows.size)
        assertEquals(JSONValue.Str("山田"), p.rows[0]["name"])
        assertEquals(JSONValue.Num(1.0), p.rows[0]["grade"])
        assertEquals(JSONValue.Str("前衛"), p.rows[1]["part"])
        assertEquals(JSONValue.Bool(false), p.rows[1]["active"])
        assertNull(p.rows[1]["grade"])
        assertEquals(
            listOf(ImportRowFailure(3, "名前", Kind.REQUIRED, ""), ImportRowFailure(4, "学年", Kind.INVALID, "二")),
            p.failures,
        )
        assertEquals(0, p.overLimit)
    }

    @Test
    fun planLimits() {
        val body = (1..520).joinToString("\n") { "人$it" }
        val p = plan("名前\n$body")
        assertEquals(RecordImport.MAX_ROWS, p.rows.size)
        assertEquals(20, p.overLimit)
        val nearlyFull = plan("名前\n$body", existing = 1990)
        assertEquals(10, nearlyFull.rows.size)
        assertEquals(510, nearlyFull.overLimit)
    }

    @Test
    fun planWithoutMatchingHeaderAddsNothing() {
        val p = plan("foo,bar\n1,2\n")
        assertTrue(p.matched.isEmpty())
        assertTrue(p.rows.isEmpty())
        assertEquals(listOf("foo", "bar"), p.unused)
    }

    /** export の CSV(BOM・CRLF・表示の書式)をそのまま読み戻せる。 */
    @Test
    fun exportedCsvReadsBack() {
        val records = listOf(
            Record(
                "a",
                mapOf(
                    "name" to JSONValue.Str("山田, 太郎"), "grade" to JSONValue.Num(2.0), "fee" to JSONValue.Num(1200.0), "height" to JSONValue.Num(172.5),
                    "active" to JSONValue.Bool(true), "part" to JSONValue.Str("後衛"), "joined" to JSONValue.Str("2026-04-08"),
                ),
            ),
            Record(
                "b",
                mapOf("name" to JSONValue.Str("=危ない"), "fee" to JSONValue.Num(-500.0), "active" to JSONValue.Bool(false), "joined" to JSONValue.Str("2025-12-01")),
            ),
        )
        val table = RecordExport.table("部員", member, records, null, null) { v, f -> FieldCoding.display(v, f, Locale.JAPAN, now) }
        val csv = CsvReader.decode(CsvWriter.data(table))!!
        val p = RecordImport.plan(CsvReader.parse(csv), member, emptyList(), 0, now, noRef)
        assertEquals(emptyList<ImportRowFailure>(), p.failures)
        assertEquals(listOf("表示"), p.unused)
        assertEquals(2, p.rows.size)
        for ((r, got) in records.zip(p.rows)) {
            for ((k, v) in r.data) assertEquals("${r.id}.$k", v, got[k])
        }
    }

    // MARK: - テキスト / PDF の本文

    @Test
    fun bodyRecordTitleTruncationAndDefaults() {
        val doc = Entity.parse(
            """
            {"id":"doc","name":"資料","titleField":"title","fields":[
              {"id":"title","name":"題名","type":"string"},
              {"id":"body","name":"本文","type":"text"},
              {"id":"short","name":"要約","type":"string"},
              {"id":"read","name":"読んだ","type":"boolean"}]}
            """,
        )
        val made = RecordImport.bodyRecord("  本文\r\nです  ", "取扱説明書.pdf", doc, "body")!!
        assertEquals(JSONValue.Str("本文\nです"), made.data["body"])
        assertEquals(JSONValue.Str("取扱説明書"), made.data["title"])
        assertEquals(JSONValue.Bool(false), made.data["read"])
        assertFalse(made.truncated)

        val long = "字".repeat(4100)
        val cut = RecordImport.bodyRecord(long, "a.txt", doc, "short")!!
        assertTrue(cut.truncated)
        assertEquals(4000, cut.data["short"]?.stringValue?.length)
        assertFalse(RecordImport.bodyRecord(long, "a.txt", doc, "body")!!.truncated)
        assertNull(RecordImport.bodyRecord("x", "a", doc, "read"))
    }

    @Test
    fun plainTextEncodings() {
        assertEquals("こんにちは", RecordImport.plainText("こんにちは".toByteArray(Charsets.UTF_8)))
        assertEquals("こんにちは", RecordImport.plainText("こんにちは".toByteArray(charset("Shift_JIS"))))
        // Apple の .utf16 と同じ BOM 付きリトルエンディアン
        val utf16 = byteArrayOf(0xFF.toByte(), 0xFE.toByte()) + "こんにちは".toByteArray(Charsets.UTF_16LE)
        assertEquals("こんにちは", RecordImport.plainText(utf16))
    }
}
