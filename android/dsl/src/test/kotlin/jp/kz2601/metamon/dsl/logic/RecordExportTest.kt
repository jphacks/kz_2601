package jp.kz2601.metamon.dsl.logic

import jp.kz2601.metamon.dsl.records.Entity
import jp.kz2601.metamon.dsl.records.Field
import jp.kz2601.metamon.dsl.records.JSONValue
import jp.kz2601.metamon.dsl.records.Record
import org.junit.Assert.assertArrayEquals
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Test

/** iOS の RecordExportTests: CSV のエスケープ・BOM、列の選び方(写真・hidden)、行の並び、ファイル名。 */
class RecordExportTest {
    private val quiz = Entity.parse(
        """
        {"id":"q","name":"問題","fields":[
          {"id":"text","name":"問題文","type":"string"},
          {"id":"correct","name":"正解","type":"enum","options":["A","B"],"hidden":true},
          {"id":"photo","name":"写真","type":"photo"},
          {"id":"date","name":"日付","type":"date"},
          {"id":"points","name":"配点","type":"integer"},
          {"id":"double","name":"倍","type":"integer","computed":{"op":"mul","args":[{"ref":"points"},2]}}
        ]}
        """,
    )

    private fun rec(id: String, vararg data: Pair<String, Any?>, author: String? = null) =
        Record(id, data.associate { it.first to JSONValue.of(it.second) }, author = author)

    @Test
    fun escapePlainCellsUntouched() {
        assertEquals("ラーメン", CsvWriter.escape("ラーメン"))
        assertEquals("", CsvWriter.escape(""))
        assertEquals("¥1200", CsvWriter.escape("¥1200"))
    }

    @Test
    fun escapeCommasQuotesNewlines() {
        assertEquals("\"¥1,200\"", CsvWriter.escape("¥1,200"))
        assertEquals("\"彼は\"\"はい\"\"と言った\"", CsvWriter.escape("彼は\"はい\"と言った"))
        assertEquals("\"1行目\n2行目\"", CsvWriter.escape("1行目\n2行目"))
        assertEquals("\"a\r\nb\"", CsvWriter.escape("a\r\nb"))
    }

    @Test
    fun escapeFormulaInjection() {
        assertEquals("'=SUM(A1:A9)", CsvWriter.escape("=SUM(A1:A9)"))
        assertEquals("'+81 90", CsvWriter.escape("+81 90"))
        assertEquals("'@cmd", CsvWriter.escape("@cmd"))
        assertEquals("'-cmd", CsvWriter.escape("-cmd"))
        assertEquals("式除けの後でも区切りを含めば囲む", "\"'=1,2\"", CsvWriter.escape("=1,2"))
        assertEquals("-500", CsvWriter.escape("-500"))
        assertEquals("-¥500", CsvWriter.escape("-¥500"))
        assertEquals("-", CsvWriter.escape("-"))
    }

    @Test
    fun csvHasBomHeaderAndCrlf() {
        val table = ExportTable("家計簿", listOf("日付", "金額", "メモ"), listOf(listOf("6/1", "¥1,200", "ランチ, 同僚と"), listOf("6/2", "¥300", "コーヒー")))
        val data = CsvWriter.data(table)
        assertArrayEquals("Excel が UTF-8 と分かるよう BOM を付ける", CsvWriter.BOM, data.copyOfRange(0, 3))
        val text = String(data.copyOfRange(3, data.size), Charsets.UTF_8)
        assertEquals("日付,金額,メモ\r\n6/1,\"¥1,200\",\"ランチ, 同僚と\"\r\n6/2,¥300,コーヒー\r\n", text)
        assertFalse("BOM は 1 回だけ", text.startsWith("﻿"))
    }

    @Test
    fun emptyTableIsHeaderOnly() {
        assertEquals("a,b\r\n", CsvWriter.string(ExportTable("x", listOf("a", "b"), emptyList())))
    }

    @Test
    fun defaultColumnsIncludeComputedSkipMediaAndHidden() {
        assertEquals(listOf("text", "date", "points", "double"), RecordExport.columns(quiz, null, false).map { it.id })
        assertEquals(listOf("text", "correct", "date", "points", "double"), RecordExport.columns(quiz, null, true).map { it.id })
    }

    @Test
    fun explicitFieldsKeepOrderAndDropUnknown() {
        assertEquals(listOf("points", "text"), RecordExport.columns(quiz, listOf("points", "nope", "photo", "text"), false).map { it.id })
    }

    @Test
    fun hiddenValuesOnlyForAuthorRows() {
        val mine = rec("1", "text" to "富士山は？", "correct" to "A", "points" to 10, "double" to 20, author = "me")
        val theirs = rec("2", "text" to "琵琶湖は？", "correct" to "B", "points" to 5, "double" to 10, author = "other")
        val display: (JSONValue, Field) -> String = { v, _ -> v.displayString }

        val others = RecordExport.table("t", quiz, listOf(theirs), null, "me", display = display)
        assertFalse("他人の行しか無ければ正解の列ごと出さない", "正解" in others.headers)
        assertFalse("B" in others.rows.flatten())

        val mixed = RecordExport.table("t", quiz, listOf(mine, theirs), null, "me", display = display)
        val col = mixed.headers.indexOf("正解")
        assertEquals("自分が書いた行は正解を出す", "A", mixed.rows[0][col])
        assertEquals("他人の行は空欄", "", mixed.rows[1][col])
        assertEquals(listOf(false, false, false, true, true), mixed.numericColumns)

        val unknownUser = RecordExport.table("t", quiz, listOf(mine), null, null, display = display)
        assertFalse("自分が分からなければ出さない", "正解" in unknownUser.headers)
    }

    @Test
    fun missingStoredValueIsBlank() {
        val t = RecordExport.table("t", quiz, listOf(rec("1", "text" to "x")), listOf("text", "points"), null) { _, _ -> "NG" }
        assertEquals("値の無い保存項目は display を通さず空欄", listOf(listOf("NG", "")), t.rows)
    }

    @Test
    fun chronologicalByFirstDateField() {
        val rs = listOf(rec("b", "date" to "2026-06-02"), rec("none"), rec("a", "date" to "2026-06-01"), rec("c", "date" to "2026-06-02"))
        assertEquals(listOf("a", "b", "c", "none"), RecordExport.chronological(rs, quiz).map { it.id })
    }

    @Test
    fun fileName() {
        assertEquals("6 月の家計簿.csv", RecordExport.fileName("6 月の家計簿", "csv"))
        assertEquals("a_b_c.pdf", RecordExport.fileName("a/b:c", "pdf"))
        assertEquals("_x.csv", RecordExport.fileName("../x", "csv"))
        assertEquals("書き出し.csv", RecordExport.fileName("  ", "csv"))
    }
}
