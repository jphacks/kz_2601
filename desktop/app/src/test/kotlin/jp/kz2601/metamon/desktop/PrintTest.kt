package jp.kz2601.metamon.desktop

import jp.kz2601.metamon.desktop.io.PdfIo
import jp.kz2601.metamon.dsl.logic.ExportTable
import jp.kz2601.metamon.dsl.records.FieldCoding
import jp.kz2601.metamon.dsl.records.JSONValue
import jp.kz2601.metamon.dsl.records.entityModels
import jp.kz2601.metamon.renderer.form.FormDraft
import org.apache.pdfbox.Loader
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test
import java.awt.Color
import java.awt.image.BufferedImage
import java.awt.print.Printable
import java.time.LocalDate
import java.time.LocalDateTime
import java.time.ZoneId
import java.util.TimeZone

/** 印刷(#1767): 頁の割り方・画面 → 表・プリンターに渡す頁。本物のプリンターは呼ばない。 */
class PrintTest {
    init {
        System.setProperty("java.awt.headless", "true")
        TimeZone.setDefault(TimeZone.getTimeZone("Asia/Tokyo"))
        FieldCoding.zone = ZoneId.of("Asia/Tokyo")
    }

    private val printedAt = LocalDateTime.of(2026, 11, 12, 9, 30)

    private fun table(rows: Int, notes: List<String> = listOf("印刷日 2026/11/12 09:30", "$rows 件")) =
        ExportTable("支出", listOf("日付", "金額"), (1..rows).map { listOf("11/$it", "${it * 100}") }, notes, listOf(false, true))

    @Test
    fun tableRowsSplitAcrossPagesInOrder() {
        val pages = PdfIo.pages(table(300))
        assertTrue("300 行は 1 頁に入らない", pages.size > 1)
        // 全行が抜けも重なりもなく順に載る
        assertEquals((0 until 300).toList(), pages.flatMap { it.rows.toList() })
        pages.zipWithNext().forEach { (a, b) -> assertEquals(a.rows.last + 1, b.rows.first) }
        // 1 頁目は見出し・補足のぶん少ない。2 頁目以降は同じ数(最後を除く)
        assertTrue(pages[0].rows.count() < pages[1].rows.count())
        pages.drop(1).dropLast(1).forEach { assertEquals(pages[1].rows.count(), it.rows.count()) }
        // 頁は A4 を SCALE 倍した画像
        pages.forEach {
            assertEquals((595.27563f * PdfIo.SCALE).toInt(), it.image.width)
            assertEquals((841.8898f * PdfIo.SCALE).toInt(), it.image.height)
        }
    }

    @Test
    fun shortAndEmptyTablesAreOnePage() {
        assertEquals(listOf(0 until 3), PdfIo.pages(table(3)).map { it.rows })
        val empty = PdfIo.pages(table(0))
        assertEquals(1, empty.size)
        assertTrue(empty[0].rows.isEmpty())
        assertTrue("空でも見出しは描く", darkPixels(empty[0].image) > 0)
    }

    @Test
    fun exportPdfHasSamePagesAsPrint() {
        val doc = PrintDocument("支出", listOf(table(120), table(5)))
        val pdfPages = PdfIo.pages(table(120)).size + 1
        assertEquals(pdfPages, doc.pages.size)
        Loader.loadPDF(doc.pdf()).use { assertEquals(doc.pages.size, it.numberOfPages) }
        // 表 1 本の PDF は export(PdfIo.data)と同じ頁数
        Loader.loadPDF(PdfIo.data(table(120))).use { assertEquals(pdfPages - 1, it.numberOfPages) }
        assertEquals(listOf(0, 1), doc.pages.map { it.section }.distinct())
    }

    @Test
    fun printableDrawsSamePageOnA4() {
        val doc = PrintDocument("支出", listOf(table(120)))
        val printable = PrintPages(doc)
        assertEquals(doc.pages.size, printable.numberOfPages)
        val pf = printable.getPageFormat(0)
        assertEquals(PrintPages.A4_W, pf.width, 0.01)
        assertEquals(PrintPages.A4_H, pf.height, 0.01)
        assertEquals(0.0, pf.imageableX, 0.0)
        assertEquals(PrintPages.A4_W, pf.imageableWidth, 0.01)

        // 紙の代わりに画像へ「印刷」する(2 倍 = 頁の画像と同じ画素数)
        val page = doc.pages[0].pdf.image
        val paper = BufferedImage(page.width, page.height, BufferedImage.TYPE_INT_RGB)
        val g = paper.createGraphics()
        g.color = Color.WHITE
        g.fillRect(0, 0, paper.width, paper.height)
        g.scale(PdfIo.SCALE.toDouble(), PdfIo.SCALE.toDouble())
        assertEquals(Printable.PAGE_EXISTS, printable.getPrintable(0).print(g, pf, 0))
        assertEquals(Printable.NO_SUCH_PAGE, printable.print(g, pf, doc.pages.size))
        g.dispose()
        // 余白・位置が PDF と同じ(文字のある範囲が 4px 以内で一致)。拡大の補間で字の濃さは少し変わる
        val expected = inkBounds(page)
        val actual = inkBounds(paper)
        listOf(expected.x - actual.x, expected.y - actual.y, expected.width - actual.width, expected.height - actual.height).forEach {
            assertTrue("印刷した紙の中身が頁と同じ($actual vs $expected)", kotlin.math.abs(it) <= 4)
        }
        assertTrue(kotlin.math.abs(darkPixels(paper) - darkPixels(page)) < darkPixels(page) * 0.25)
    }

    @Test
    fun kakeiboHomePrintsSummaryAndAllExpenses() {
        val dsl = Examples.load("kakeibo")
        val today = LocalDate.now()
        val store = (1..80).fold(MockStore()) { s, i ->
            s.save("expense", null, mapOf("amount" to "100", "category" to "食費", "memo" to "メモ$i", "date" to today.toString()))
        }
        val doc = ScreenPrint.document(dsl, "home", store, printedAt = printedAt)!!
        assertEquals("家計簿", doc.title)
        assertEquals(listOf("家計簿", "最近の支出"), doc.sections.map { it.title })
        val summary = doc.sections[0]
        assertEquals(listOf("項目", "値"), summary.headers)
        assertEquals(listOf("今月の支出", "今月の件数"), summary.rows.map { it[0] })
        assertTrue(summary.rows[0][1], summary.rows[0][1].contains("8,000"))
        assertTrue(summary.rows[1][1], summary.rows[1][1].startsWith("80"))
        assertEquals("印刷日 2026/11/12 09:30", summary.notes.first())

        // 一覧は書き出しと同じ列・全行(画面の並びに関係なく古い順)
        val list = doc.sections[1]
        assertEquals(listOf("金額", "カテゴリ", "メモ", "日付"), list.headers)
        assertEquals(80, list.rows.size)
        assertEquals(listOf("80 件"), list.notes)
        assertTrue(doc.pages.count { it.section == 1 } >= 2)
        assertEquals((0 until 80).toList(), doc.pages.filter { it.section == 1 }.flatMap { it.pdf.rows.toList() })
    }

    @Test
    fun kakeiboStatsPrintsChartsAsTables() {
        val dsl = Examples.load("kakeibo")
        val today = LocalDate.now().toString()
        val store = MockStore()
            .save("expense", null, mapOf("amount" to "300", "category" to "食費", "date" to today))
            .save("expense", null, mapOf("amount" to "200", "category" to "交通", "date" to today))
        val doc = ScreenPrint.document(dsl, "stats", store, printedAt = printedAt)!!
        assertEquals(listOf("カテゴリ別(今月)", "月ごとの支出"), doc.sections.map { it.title })
        val byCategory = doc.sections[0]
        assertEquals(listOf("カテゴリ", "金額"), byCategory.headers)
        assertEquals(listOf(false, true), byCategory.numericColumns)
        assertTrue(byCategory.rows.toString(), byCategory.rows.any { it[0] == "食費" && it[1].contains("300") })
        assertTrue(byCategory.rows.toString(), byCategory.rows.any { it[0] == "交通" && it[1].contains("200") })
        assertEquals("印刷日 2026/11/12 09:30", byCategory.notes.first())
    }

    @Test
    fun timetablePrintsGrid() {
        val dsl = Examples.load("timetable")
        val store = MockStore()
            .save("lecture", null, mapOf("subject" to "数学", "weekday" to "月", "period" to "1", "room" to "A101"))
            .save("lecture", null, mapOf("subject" to "英語", "weekday" to "水", "period" to "3"))
            .save("lecture", null, mapOf("subject" to "化学", "weekday" to "水", "period" to "3", "room" to "実験室"))
        val doc = ScreenPrint.document(dsl, "home", store, printedAt = printedAt)!!
        assertEquals(1, doc.sections.size)
        val grid = doc.sections[0]
        assertEquals("今週の時間割", grid.title)
        assertEquals(listOf("", "月", "火", "水", "木", "金"), grid.headers)
        assertEquals(listOf("1限", "2限", "3限", "4限", "5限"), grid.rows.map { it[0] })
        assertEquals("数学(A101)", grid.rows[0][1])
        assertEquals("英語、化学(実験室)", grid.rows[2][3])
        assertEquals("", grid.rows[1][2])
        assertEquals(1, doc.pages.size)
    }

    @Test
    fun formScreenPrintsInputValues() {
        val dsl = Examples.load("kakeibo")
        val entity = dsl.entityModels().first { it.id == "expense" }
        val form = FormDraft(entity).apply {
            values["amount"] = JSONValue.Num(1500.0)
            values["memo"] = JSONValue.Str("ランチ")
        }
        val doc = ScreenPrint.document(dsl, "expense_form", MockStore(), form, printedAt = printedAt)!!
        val rows = doc.sections.single().rows.associate { it[0] to it[1] }
        assertTrue(rows.toString(), rows.getValue("金額").contains("1,500"))
        assertEquals("ランチ", rows["メモ"])
        assertTrue(rows.toString(), "カテゴリ" in rows)
    }

    @Test
    fun screenWithoutTablesStillPrintsOnePage() {
        val dsl = Examples.load("kakeibo")
        // フォームが無いフォーム画面: 印刷する値が無いので「記録がありません」の 1 頁
        val doc = ScreenPrint.document(dsl, "expense_form", MockStore(), null, printedAt = printedAt)!!
        assertEquals(1, doc.sections.size)
        assertTrue(doc.sections[0].rows.isEmpty())
        assertEquals(1, doc.pages.size)
    }

    private fun inkBounds(img: BufferedImage): java.awt.Rectangle {
        var x0 = img.width; var y0 = img.height; var x1 = -1; var y1 = -1
        for (y in 0 until img.height) for (x in 0 until img.width) {
            if ((img.getRGB(x, y) and 0xFF) < 128) {
                x0 = minOf(x0, x); y0 = minOf(y0, y); x1 = maxOf(x1, x); y1 = maxOf(y1, y)
            }
        }
        return java.awt.Rectangle(x0, y0, x1 - x0, y1 - y0)
    }

    private fun darkPixels(img: BufferedImage): Int {
        var n = 0
        for (y in 0 until img.height step 2) for (x in 0 until img.width step 2) {
            if ((img.getRGB(x, y) and 0xFF) < 128) n++
        }
        return n
    }
}
