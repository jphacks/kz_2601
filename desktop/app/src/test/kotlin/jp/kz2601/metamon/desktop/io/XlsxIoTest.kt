package jp.kz2601.metamon.desktop.io

import jp.kz2601.metamon.dsl.logic.RecordImport
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.ByteArrayOutputStream
import java.math.BigDecimal
import java.util.zip.ZipEntry
import java.util.zip.ZipOutputStream

/** .xlsx の最初のシート → 行 × 列の文字(testdata は src/test/resources/xlsx/sample.xlsx・作り方は make_sample.py)。 */
class XlsxIoTest {
    private val sample: ByteArray = javaClass.getResourceAsStream("/xlsx/sample.xlsx")!!.use { it.readBytes() }

    private fun table(bytes: ByteArray = sample, sheet: Int = 0) = XlsxIo.read(bytes, sheet) as XlsxIo.Result.Table

    @Test
    fun firstSheetBecomesRowsOfText() {
        val t = table()
        assertEquals(listOf("家計", "メモ", "空"), t.sheetNames)
        assertEquals("家計", t.sheet)
        assertEquals(
            listOf(
                listOf("日付", "品目", "金額", "メモ", "済"),
                // 日付の書式・空のセル(D2)・真偽
                listOf("2024-10-24", "ラーメン", "980", "", "TRUE"),
                // 自前の日時の書式・その場の文字・式(計算済みの値を 15 桁で)
                listOf("2024-10-25 14:30", "コーヒー", "0.3", "", "FALSE"),
                // 空の行(4 行目)は飛ばす。先頭の空のセルは残し、ふりがなは入れない。式のエラーはそのまま
                listOf("", "特盛 餃子", "980.3", "", "#DIV/0!"),
                // 桁区切りの書式の数・時刻だけ・文字の式・色つきの数(日付ではない)・指数の数
                listOf("1234567.5", "12:00", "abc", "2.5", "0.001"),
            ),
            t.rows,
        )
    }

    @Test
    fun otherSheetsCanBeChosen() {
        assertEquals(listOf(listOf("", "two")), table(sheet = 1).rows)
        assertEquals(emptyList<List<String>>(), table(sheet = 2).rows)
        assertEquals(XlsxIo.Result.Unreadable, XlsxIo.read(sample, 3))
    }

    @Test
    fun overTheImportLimitIsRefusedBeforeReading() {
        assertEquals(XlsxIo.Result.TooLarge, XlsxIo.read(ByteArray(RecordImport.MAX_TEXT_BYTES + 1)))
    }

    @Test
    fun hugeExpandedPartIsRefused() {
        val bomb = zip("xl/worksheets/sheet1.xml" to ByteArray((XlsxIo.MAX_PART_BYTES + 1).toInt()))
        assertTrue("圧縮で小さい", bomb.size < RecordImport.MAX_TEXT_BYTES)
        assertEquals(XlsxIo.Result.TooLarge, XlsxIo.read(bomb))
    }

    @Test
    fun brokenFilesAreUnreadable() {
        assertEquals("zip でない(.xls・暗号化)", XlsxIo.Result.Unreadable, XlsxIo.read("not a zip".toByteArray()))
        assertEquals("workbook が無い", XlsxIo.Result.Unreadable, XlsxIo.read(zip("xl/styles.xml" to "<styleSheet/>".toByteArray())))
        val dtd = """<?xml version="1.0"?><!DOCTYPE w [<!ENTITY x SYSTEM "file:///etc/passwd">]><workbook>&x;</workbook>"""
        assertEquals("DTD は受け付けない", XlsxIo.Result.Unreadable, XlsxIo.read(zip("xl/workbook.xml" to dtd.toByteArray())))
    }

    @Test
    fun serialDatesFollowBothEpochs() {
        assertEquals("1900-01-01", XlsxIo.date(1.0, date1904 = false))
        assertEquals("1900-02-28", XlsxIo.date(59.0, date1904 = false))
        assertEquals("1900-03-01", XlsxIo.date(61.0, date1904 = false))
        assertEquals("2024-10-24", XlsxIo.date(45589.0, date1904 = false))
        assertEquals("2024-10-24", XlsxIo.date(45589.0 - 1462, date1904 = true))
        assertEquals("2024-10-24 09:15:30", XlsxIo.date(45589.0 + (9 * 3600 + 15 * 60 + 30) / 86400.0, date1904 = false))
        assertEquals("06:00", XlsxIo.date(0.25, date1904 = false))
        assertEquals(null, XlsxIo.date(-1.0, date1904 = false))
    }

    @Test
    fun dateFormatsAreRecognized() {
        assertTrue(XlsxIo.isBuiltInDate(14))
        assertTrue(XlsxIo.isBuiltInDate(57))
        assertFalse(XlsxIo.isBuiltInDate(4))
        assertTrue(XlsxIo.isDateFormat("yyyy/mm/dd"))
        assertTrue(XlsxIo.isDateFormat("[\$-411]ggge\"年\"m\"月\"d\"日\""))
        assertTrue(XlsxIo.isDateFormat("[h]:mm"))
        assertFalse(XlsxIo.isDateFormat("#,##0.00"))
        assertFalse(XlsxIo.isDateFormat("0.0\"days\""))
        assertFalse(XlsxIo.isDateFormat("[Red]0.00;[Blue]-0.00"))
        assertFalse(XlsxIo.isDateFormat("0.00E+00"))
    }

    @Test
    fun numbersReadLikeExcelShowsThem() {
        assertEquals("0.3", XlsxIo.number(BigDecimal("0.30000000000000004")))
        assertEquals("1200", XlsxIo.number(BigDecimal("1.2E3")))
        assertEquals("-5", XlsxIo.number(BigDecimal("-5.000")))
        assertEquals("0", XlsxIo.number(BigDecimal("-0.0")))
        assertEquals(2, XlsxIo.column("C12"))
        assertEquals(26, XlsxIo.column("AA1"))
        assertEquals(16383, XlsxIo.column("XFD1"))
    }

    private fun zip(vararg parts: Pair<String, ByteArray>): ByteArray {
        val out = ByteArrayOutputStream()
        ZipOutputStream(out).use { z ->
            parts.forEach { (name, bytes) ->
                z.putNextEntry(ZipEntry(name))
                z.write(bytes)
                z.closeEntry()
            }
        }
        return out.toByteArray()
    }
}
