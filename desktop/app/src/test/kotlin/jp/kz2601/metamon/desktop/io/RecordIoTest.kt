package jp.kz2601.metamon.desktop.io

import jp.kz2601.metamon.desktop.Examples
import jp.kz2601.metamon.desktop.MockStore
import jp.kz2601.metamon.dsl.ImportColumn
import jp.kz2601.metamon.dsl.logic.CsvWriter
import jp.kz2601.metamon.dsl.records.FieldCoding
import jp.kz2601.metamon.dsl.records.JSONValue
import jp.kz2601.metamon.dsl.records.entityModels
import org.apache.pdfbox.pdmodel.PDDocument
import org.apache.pdfbox.pdmodel.PDPage
import org.apache.pdfbox.pdmodel.PDPageContentStream
import org.apache.pdfbox.pdmodel.encryption.AccessPermission
import org.apache.pdfbox.pdmodel.encryption.StandardProtectionPolicy
import org.apache.pdfbox.pdmodel.font.PDType1Font
import org.apache.pdfbox.pdmodel.font.Standard14Fonts
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.ByteArrayOutputStream
import java.time.Instant
import java.time.ZoneId
import java.util.Locale
import java.util.TimeZone

class RecordIoTest {
    private val dsl = Examples.load("file_import")
    private val entities = dsl.entityModels()
    private val member = entities.first { it.id == "member" }

    // 2026-10-24(金)
    private val now: Instant = Instant.parse("2026-10-24T03:00:00Z")

    init {
        TimeZone.setDefault(TimeZone.getTimeZone("Asia/Tokyo"))
        FieldCoding.zone = ZoneId.of("Asia/Tokyo")
    }

    private fun store(): MockStore = MockStore()
        .save("member", null, mapOf("name" to "山田 太郎", "grade" to "2", "part" to "前衛", "joined" to "2026-04-10"))
        .save("member", null, mapOf("name" to "=SUM(A1)", "grade" to "1", "part" to "後衛"))
        .save("member", null, mapOf("name" to "佐藤, \"花子\"", "part" to "未定", "joined" to "2025-04-01"))

    @Test
    fun exportCsvIsIosShape() {
        val table = RecordIo.exportTable("部員", member, store(), entities, null, locale = Locale.JAPAN, now = now)
        val text = String(RecordIo.csv(table), Charsets.UTF_8)
        assertTrue("UTF-8 BOM", RecordIo.csv(table).copyOfRange(0, 3).contentEquals(CsvWriter.BOM))
        // 日付の列があるので古い順(日付なしは後ろ)・式に見えるセルは ' で止める・CRLF
        assertEquals(
            "﻿名前,学年,パート,入部日\r\n" +
                "\"佐藤, \"\"花子\"\"\",,未定,2025/4/1(火)\r\n" +
                "山田 太郎,2,前衛,4/10(金)\r\n" +
                "'=SUM(A1),1,後衛,\r\n",
            text,
        )
    }

    @Test
    fun csvRoundTripRestoresRecords() {
        // 空の日付は取り込みで今日が入る(iOS と同じ既定値)ので、往復は全部の行に日付を入れて比べる
        val src = store().save("member", "new-2", mapOf("name" to "=SUM(A1)", "grade" to "1", "part" to "後衛", "joined" to "2026-01-05"))
        val table = RecordIo.exportTable("部員", member, src, entities, null, locale = Locale.JAPAN, now = now)
        val parsed = RecordIo.csvTable(RecordIo.csv(table))!!
        val plan = RecordIo.plan(parsed, member, emptyList(), MockStore(), entities, now)
        assertEquals(emptyList<Any>(), plan.failures)
        val back = RecordIo.apply(MockStore(), "member", plan.rows).list("member").map { it.data }.toSet()
        assertEquals(src.list("member").map { it.data }.toSet(), back)
    }

    @Test
    fun mapUsesHeaderAlias() {
        val table = listOf(listOf("氏名", "学年", "メモ"), listOf("鈴木", "３", "x"), listOf("", "1", "y"))
        val plan = RecordIo.plan(table, member, listOf(ImportColumn("氏名", "name")), MockStore(), entities, now)
        assertEquals(listOf("氏名" to "名前", "学年" to "学年"), plan.matched)
        assertEquals(listOf("メモ"), plan.unused)
        assertEquals(1, plan.rows.size)
        assertEquals(JSONValue.Num(3.0), plan.rows[0]["grade"])
        assertEquals(3, plan.failures.single().line)
        val summary = RecordIo.summary(plan)
        assertEquals("1 件を追加します", summary.first())
        assertTrue(summary.any { it.contains("3 行目 名前: 必須が空") })
    }

    @Test
    fun tsvForSpreadsheets() {
        val table = RecordIo.exportTable("部員", member, store(), entities, null, locale = Locale.JAPAN, now = now)
        val tsv = Tsv.string(table)
        assertEquals("名前\t学年\tパート\t入部日", tsv.lineSequence().first())
        assertTrue(tsv.endsWith("\r\n"))
        assertEquals("\"a\tb\"\t\"x\"\"y\"\t\"1\n2\"\r\n", Tsv.string(null, listOf(listOf("a\tb", "x\"y", "1\n2"))))
        // コピー → 貼り付けで元に戻る(表計算から戻ってきた形)
        assertEquals(listOf(table.headers) + table.rows, Tsv.parse(tsv))
        val plan = RecordIo.plan(RecordIo.pastedTable(tsv), member, emptyList(), MockStore(), entities, now)
        assertEquals(3, plan.rows.size)
    }

    @Test
    fun pastedTableDetectsTabsOrCommas() {
        assertEquals(listOf(listOf("a", "b"), listOf("1", "2")), RecordIo.pastedTable("a\tb\n1\t2\n"))
        assertEquals(listOf(listOf("a", "b"), listOf("1", "2")), RecordIo.pastedTable("a,b\r\n1,2"))
        // LibreOffice は最後の改行なし・空行を挟むことがある
        assertEquals(listOf(listOf("x"), listOf("y")), Tsv.parse("x\n\ny"))
    }

    @Test
    fun storeValuesConvert() {
        val grade = member.fields.first { it.id == "grade" }
        assertEquals(JSONValue.Num(2.0), RecordIo.toJson("2", grade))
        assertEquals("2", RecordIo.toStore(JSONValue.Num(2.0)))
        assertEquals(null, RecordIo.toStore(JSONValue.Null))
        assertEquals("a,b", RecordIo.toStore(JSONValue.Arr(listOf(JSONValue.Str("a"), JSONValue.Str("b")))))
    }

    @Test
    fun fileKinds() {
        assertEquals(FileKind.CSV, FileKind.of("名簿.CSV"))
        assertEquals(FileKind.IMAGE, FileKind.of("a.jpeg"))
        assertEquals(FileKind.PDF, FileKind.of("x.pdf"))
        assertEquals(FileKind.TEXT, FileKind.of("memo.txt"))
        assertEquals(FileKind.OTHER, FileKind.of("noext"))
    }

    @Test
    fun pdfExportAndRead() {
        val table = RecordIo.exportTable("部員", member, store(), entities, null, listOf("2 件"), locale = Locale.JAPAN, now = now)
        val pdf = PdfIo.data(table)
        assertEquals("%PDF", String(pdf, 0, 4, Charsets.US_ASCII))
        // 画像で描くので文字は入っていない(スキャンと同じ扱い)
        assertEquals(PdfIo.TextResult.NoText, PdfIo.text(pdf))
        assertTrue(PdfIo.text(textPdf("hello desktop", encrypt = false)).let { it is PdfIo.TextResult.Text && it.text.contains("hello desktop") })
        assertEquals(PdfIo.TextResult.Locked, PdfIo.text(textPdf("secret", encrypt = true)))
        assertEquals(PdfIo.TextResult.Broken, PdfIo.text("not a pdf".toByteArray()))
    }

    @Test
    fun clipText() {
        assertEquals("abc", PdfIo.clip("abc", 3) { it.length })
        assertEquals("ab…", PdfIo.clip("abcdef", 3) { it.length })
    }

    private fun textPdf(text: String, encrypt: Boolean): ByteArray = PDDocument().use { doc ->
        val page = PDPage()
        doc.addPage(page)
        PDPageContentStream(doc, page).use {
            it.beginText()
            it.setFont(PDType1Font(Standard14Fonts.FontName.HELVETICA), 12f)
            it.newLineAtOffset(50f, 700f)
            it.showText(text)
            it.endText()
        }
        if (encrypt) doc.protect(StandardProtectionPolicy("owner", "user", AccessPermission()).apply { encryptionKeyLength = 128 })
        ByteArrayOutputStream().also { doc.save(it) }.toByteArray()
    }
}
