package jp.kz2601.metamon.desktop

import jp.kz2601.metamon.desktop.i18n.Strings
import jp.kz2601.metamon.desktop.io.PdfIo
import jp.kz2601.metamon.dsl.records.FieldCoding
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test
import java.awt.image.BufferedImage
import java.awt.print.Printable
import java.time.LocalDate
import java.time.LocalDateTime
import java.time.ZoneId
import java.util.TimeZone

/**
 * 印刷の準備(DesktopPrint.prepare がプリンターに渡す PrintPages): 頁の数と切れ目が、表ごとの PdfIo.pages と同じ。
 * OS の印刷ダイアログは出さない(prepare の中の printDialog は手で確かめる)。
 */
class PrintPrepareTest {
    init {
        System.setProperty("java.awt.headless", "true")
        TimeZone.setDefault(TimeZone.getTimeZone("Asia/Tokyo"))
        FieldCoding.zone = ZoneId.of("Asia/Tokyo")
    }

    private val printedAt = LocalDateTime.of(2026, 11, 12, 9, 30)

    /** 表ごとに PdfIo.pages で割った (表の添字, 行の範囲)。 */
    private fun expected(doc: PrintDocument) =
        doc.sections.flatMapIndexed { i, t -> PdfIo.pages(t, Strings["desktop_print_empty"]).map { i to it.rows } }

    private fun assertSameAsPdf(doc: PrintDocument) {
        assertEquals(expected(doc), doc.pages.map { it.section to it.pdf.rows })
        val pageable = PrintPages(doc)
        assertEquals(doc.pages.size, pageable.numberOfPages)
        val pf = pageable.getPageFormat(0)
        val g = BufferedImage(10, 10, BufferedImage.TYPE_INT_RGB).createGraphics()
        doc.pages.indices.forEach { assertEquals(Printable.PAGE_EXISTS, pageable.getPrintable(it).print(g, pf, it)) }
        assertEquals(Printable.NO_SUCH_PAGE, pageable.print(g, pf, doc.pages.size))
        g.dispose()
    }

    @Test
    fun preparePagesMatchPdf() {
        val kakeibo = Examples.load("kakeibo")
        val today = LocalDate.now().toString()

        // 行の多い list: 一覧は複数頁に割れ、行は抜けも重なりもない
        val many = (1..150).fold(MockStore()) { s, i ->
            s.save("expense", null, mapOf("amount" to "$i", "category" to if (i % 2 == 0) "食費" else "交通", "memo" to "メモ$i", "date" to today))
        }
        val home = ScreenPrint.document(kakeibo, "home", many, printedAt = printedAt)!!
        assertSameAsPdf(home)
        val listPages = home.pages.filter { it.section == 1 }
        assertTrue("150 行は 2 頁以上", listPages.size >= 2)
        assertEquals((0 until 150).toList(), listPages.flatMap { it.pdf.rows.toList() })
        assertEquals("表ごとに頁を改める", 0, listPages.first().pdf.rows.first)

        // 2 つの表(グラフ 2 つ): 表ごとに 1 頁から
        val stats = ScreenPrint.document(kakeibo, "stats", many, printedAt = printedAt)!!
        assertEquals(2, stats.sections.size)
        assertSameAsPdf(stats)
        assertEquals(listOf(0, 1), stats.pages.map { it.section }.distinct())

        // stat だけの画面: 「項目 / 値」の表 1 枚・1 頁
        val walk = ScreenPrint.document(Examples.load("walk_route"), "home", MockStore(), printedAt = printedAt)!!
        assertEquals(1, walk.sections.size)
        assertEquals(listOf("今日の距離", "今日の時間"), walk.sections[0].rows.map { it[0] })
        assertSameAsPdf(walk)
        assertEquals(listOf(0 to (0 until 2)), walk.pages.map { it.section to it.pdf.rows })
    }
}
