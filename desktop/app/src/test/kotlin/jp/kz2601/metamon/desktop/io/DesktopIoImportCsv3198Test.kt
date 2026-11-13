package jp.kz2601.metamon.desktop.io

import jp.kz2601.metamon.desktop.Examples
import jp.kz2601.metamon.desktop.MockStore
import jp.kz2601.metamon.desktop.i18n.Strings
import jp.kz2601.metamon.dsl.logic.CsvWriter
import jp.kz2601.metamon.dsl.logic.RecordImport
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.rules.TemporaryFolder
import java.io.RandomAccessFile

/** DesktopIo.importCsv / importCsvFile(#3198): 大きすぎる・文字コード・確認に渡る行と列の割り当て。 */
class DesktopIoImportCsv3198Test {
    @get:Rule
    val tmp = TemporaryFolder()

    private val dsl = Examples.load("file_import")
    private val io = DesktopIo(dsl, { MockStore() }, {})
    private val member = io.entity("member")!!

    private val csv = "名前,学年,パート\r\n山田 太郎,2,前衛\r\n鈴木,1,後衛\r\n"

    @Test
    fun importCsvRejectsLarge() {
        val big = tmp.newFile("big.csv")
        RandomAccessFile(big, "rw").use { it.setLength(RecordImport.MAX_TEXT_BYTES + 1L) }
        io.importCsvFile(big, member, null, "big.csv")
        assertEquals(Strings["desktop_io_file_too_large"], io.notice)
        assertNull("読む前に断るので確認は出ない", io.pending)
    }

    @Test
    fun importCsvShiftJis() {
        io.importCsv(csv.toByteArray(charset("Shift_JIS")), member, null, "sjis.csv")
        val p = io.pending
        assertNotNull(p)
        assertEquals(2, p!!.plan.rows.size)
        assertEquals(listOf("名前" to "名前", "学年" to "学年", "パート" to "パート"), p.plan.matched)
    }

    @Test
    fun importCsvUtf8WithBom() {
        io.importCsv(CsvWriter.BOM + csv.toByteArray(Charsets.UTF_8), member, null, "utf8.csv")
        val p = io.pending!!
        // BOM が先頭の見出しに残ると「名前」に合わず列が外れる
        assertEquals("名前", p.plan.matched.first().first)
        assertEquals(2, p.plan.rows.size)
    }

    @Test
    fun importCsvFilePassesLabelAndRowsToConfirmation() {
        val f = tmp.newFile("members.csv").apply { writeBytes(csv.toByteArray(Charsets.UTF_8)) }
        io.importCsvFile(f, member, null, "部員表")
        val p = io.pending!!
        assertEquals("member", p.entityId)
        assertEquals("部員表", p.label)
        assertEquals(listOf("山田 太郎", "鈴木"), p.plan.rows.map { (it["name"] as jp.kz2601.metamon.dsl.records.JSONValue.Str).value })
    }

    @Test
    fun importCsvWithoutMatchingColumnsOnlyNotices() {
        io.importCsv("x,y\r\n1,2\r\n".toByteArray(), member, null, "other.csv")
        assertNull(io.pending)
        assertEquals(Strings["desktop_io_no_matching_columns"], io.notice)
    }

    @Test
    fun importCsvUnreadableBytesOnlyNotices() {
        io.importCsv(byteArrayOf(0xFF.toByte(), 0xFE.toByte(), 0x00, 0x81.toByte()), member, null, "bin.csv")
        assertNull(io.pending)
        assertTrue(io.notice == Strings["desktop_io_unreadable_encoding"] || io.notice == Strings["desktop_io_no_matching_columns"])
    }
}
