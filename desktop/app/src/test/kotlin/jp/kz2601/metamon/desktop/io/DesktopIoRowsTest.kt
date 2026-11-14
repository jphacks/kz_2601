package jp.kz2601.metamon.desktop.io

import jp.kz2601.metamon.desktop.Examples
import jp.kz2601.metamon.desktop.MockStore
import jp.kz2601.metamon.desktop.i18n.Strings
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import java.awt.datatransfer.Clipboard
import java.awt.datatransfer.DataFlavor
import java.awt.datatransfer.StringSelection

/** 一覧のコピーと貼り付けの TSV(DesktopIo の copyRows・pasteRows)。作り物のクリップボードで試す。 */
class DesktopIoRowsTest {
    private val dsl = Examples.load("file_import")

    private val store = MockStore()
        .save("member", null, mapOf("name" to "山田 太郎", "grade" to "2", "part" to "前衛"), newId = "m1")
        .save("member", null, mapOf("name" to "鈴木", "grade" to "1", "part" to "後衛"), newId = "m2")
        .save("member", null, mapOf("name" to "佐藤", "grade" to "3", "part" to "前衛"), newId = "m3")

    private val cb = Clipboard("test")

    private fun io(store: MockStore = this.store, clipboard: Clipboard? = cb) = DesktopIo(dsl, { store }, {}, clipboard = { clipboard })

    private fun copied(): List<List<String>> = Tsv.parse(cb.getData(DataFlavor.stringFlavor) as String)

    private fun names(table: List<List<String>>, header: String = "名前"): List<String> {
        val col = table.first().indexOf(header)
        assertTrue("見出しに $header がある: ${table.first()}", col >= 0)
        return table.drop(1).map { it[col] }
    }

    @Test
    fun copyRowsTakesOnlyTheSelectedRowsInTheOrderOnScreen() {
        val io = io()
        io.copyRows("member", listOf("m3", "m1"))
        val table = copied()
        assertEquals(listOf("佐藤", "山田 太郎"), names(table))
        assertEquals("見出しの 1 行 + 選んだ 2 行", 3, table.size)
        assertEquals(Strings.format("desktop_io_copied_rows", 2), io.notice)
    }

    @Test
    fun copyRowsWithoutIdsTakesEveryRow() {
        val io = io()
        io.copyRows("member")
        assertEquals(listOf("山田 太郎", "鈴木", "佐藤"), names(copied()))
        // 何も選んでいない時(空の一覧)も全部(選んだ行だけにしない)
        io.copyRows("member", emptyList())
        assertEquals(listOf("山田 太郎", "鈴木", "佐藤"), names(copied()))
        assertEquals(Strings.format("desktop_io_copied_rows", 3), io.notice)
    }

    @Test
    fun copyRowsOfAnUnknownEntityDoesNothing() {
        cb.setContents(StringSelection("前の中身"), null)
        val io = io()
        io.copyRows("no-such-entity", listOf("m1"))
        assertNull(io.notice)
        assertEquals("前の中身", cb.getData(DataFlavor.stringFlavor))
    }

    @Test
    fun copyRowsWithoutAClipboardOnlyNotices() {
        val io = io(clipboard = null)
        io.copyRows("member", listOf("m1"))
        assertEquals(Strings["desktop_io_copy_failed"], io.notice)
    }

    @Test
    fun pasteRowsTakesTheTsvOfASpreadsheet() {
        cb.setContents(StringSelection("名前\t学年\r\n田中\t2\r\n\r\n高橋\t1\r\n"), null)
        val io = io()
        io.pasteRows("member")
        val p = io.pending
        assertNotNull(p)
        assertEquals("member", p!!.entityId)
        assertEquals(Strings["desktop_io_pasted_table"], p.label)
        assertEquals(listOf("名前" to "名前", "学年" to "学年"), p.plan.matched)
        assertEquals("空の行は入らない", 2, p.plan.rows.size)
        assertEquals(listOf("田中", "高橋"), p.plan.rows.map { it["name"]?.stringValue })
    }

    @Test
    fun pasteRowsTakesCsvWhenThereIsNoTab() {
        cb.setContents(StringSelection("名前,学年\r\n田中,2\r\n"), null)
        val io = io()
        io.pasteRows("member")
        assertEquals(listOf("田中"), io.pending!!.plan.rows.map { it["name"]?.stringValue })
    }

    @Test
    fun pasteRowsWithoutATableOnlyNotices() {
        val io = io()
        // 文字が入っていないクリップボード
        io.pasteRows("member")
        assertNull(io.pending)
        assertEquals(Strings["desktop_io_no_table_on_clipboard"], io.notice)

        // 当たる列が無い表は取り込みの確認を出さない
        cb.setContents(StringSelection("x\ty\r\n1\t2\r\n"), null)
        val other = io()
        other.pasteRows("member")
        assertNull(other.pending)
        assertEquals(Strings["desktop_io_no_matching_columns"], other.notice)
    }

    @Test
    fun pasteRowsOfAnUnknownEntityDoesNothing() {
        cb.setContents(StringSelection("名前\t学年\r\n田中\t2\r\n"), null)
        val io = io()
        io.pasteRows("no-such-entity")
        assertNull(io.pending)
        assertNull(io.notice)
    }
}
