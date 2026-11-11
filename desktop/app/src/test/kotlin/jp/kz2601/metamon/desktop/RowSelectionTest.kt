package jp.kz2601.metamon.desktop

import androidx.compose.ui.input.key.Key
import jp.kz2601.metamon.desktop.io.RecordIo
import jp.kz2601.metamon.dsl.records.FieldCoding
import jp.kz2601.metamon.dsl.records.entityModels
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import java.time.Instant
import java.time.ZoneId
import java.util.Locale
import java.util.TimeZone

/** list・table の行を選ぶ(#1766): 範囲・足し引き・全部・行が変わったら外す・選んだ行の TSV。 */
class RowSelectionTest {
    private val order = listOf("a", "b", "c", "d", "e")
    private val plain = RowClick()
    private val shift = RowClick(shift = true)
    private val toggle = RowClick(toggle = true)

    private fun start() = RowSelection(ids = order.toSet())

    @Test
    fun clickSelectsOneAndShiftClickSelectsTheRange() {
        var s = RowSelect.click(start(), "b", order, plain)
        assertEquals(setOf("b"), s.selected)
        s = RowSelect.click(s, "d", order, shift)
        assertEquals(setOf("b", "c", "d"), s.selected)
        // 起点は動かないので、上へ Shift+クリックし直すと置き換わる
        s = RowSelect.click(s, "a", order, shift)
        assertEquals(setOf("a", "b"), s.selected)
        assertEquals(listOf("a", "b"), s.inOrder(order))
    }

    @Test
    fun shiftClickWithoutAnAnchorSelectsJustThatRow() {
        val s = RowSelect.click(start(), "c", order, shift)
        assertEquals(setOf("c"), s.selected)
        assertEquals("c", s.anchor)
    }

    @Test
    fun ctrlClickTogglesAndMovesTheAnchor() {
        var s = RowSelect.click(start(), "a", order, plain)
        s = RowSelect.click(s, "c", order, toggle)
        assertEquals(setOf("a", "c"), s.selected)
        s = RowSelect.click(s, "a", order, toggle)
        assertEquals(setOf("c"), s.selected)
        // Ctrl+Shift は今の選びに範囲を足す(起点は最後に Ctrl+クリックした a)
        s = RowSelect.click(s, "e", order, RowClick(shift = true, toggle = true))
        assertEquals(setOf("a", "b", "c", "d", "e"), s.selected)
    }

    @Test
    fun selectAllTakesEveryVisibleRow() {
        val s = RowSelect.selectAll(start(), order)
        assertEquals(order.toSet(), s.selected)
        assertEquals(order, s.inOrder(order.reversed()).reversed())
    }

    @Test
    fun changingTheRowsClearsTheSelectionButSortingKeepsIt() {
        val s = RowSelect.click(RowSelect.click(start(), "a", order, plain), "c", order, shift)
        // 見出しで並べ替えただけ(集まりは同じ)なら残る
        assertEquals(setOf("a", "b", "c"), RowSelect.sync(s, order.reversed()).selected)
        // 行が足された・消えた・検索で絞った
        assertTrue(RowSelect.sync(s, order + "f").isEmpty)
        assertTrue(RowSelect.sync(s, order - "e").isEmpty)
        assertNull(RowSelect.sync(s, listOf("a", "b")).anchor)
        // 押した時に行が変わっていたら、前の選びを持ち越さない
        assertEquals(setOf("b"), RowSelect.click(s, "b", listOf("a", "b"), toggle).selected)
    }

    @Test
    fun tappingARowThatOpensDetailClearsTheSelectionButKeepsTheAnchor() {
        var s = RowSelect.selectAll(start(), order)
        s = RowSelect.tapped(s, "b", order)
        assertTrue(s.isEmpty)
        assertEquals(setOf("b", "c", "d"), RowSelect.click(s, "d", order, shift).selected)
    }

    @Test
    fun keysFollowThePlatform() {
        for (os in listOf(HostOs.WINDOWS, HostOs.LINUX)) {
            assertEquals(RowSelect.Command.SELECT_ALL, RowSelect.command(KeyPress(Key.A, ctrl = true), os))
            assertEquals(RowSelect.Command.COPY, RowSelect.command(KeyPress(Key.C, ctrl = true), os))
            assertNull(RowSelect.command(KeyPress(Key.C, meta = true), os))
            assertEquals(RowClick(shift = false, toggle = true), RowSelect.click(shift = false, ctrl = true, meta = false, os = os))
        }
        assertEquals(RowSelect.Command.COPY, RowSelect.command(KeyPress(Key.C, meta = true), HostOs.MAC))
        assertNull("macOS の Ctrl+A は行頭へ(emacs)", RowSelect.command(KeyPress(Key.A, ctrl = true), HostOs.MAC))
        assertEquals(RowClick(toggle = true), RowSelect.click(shift = false, ctrl = false, meta = true, os = HostOs.MAC))
        assertNull(RowSelect.command(KeyPress(Key.C), HostOs.WINDOWS))
        assertNull(RowSelect.command(KeyPress(Key.C, ctrl = true, shift = true), HostOs.WINDOWS))
        assertNull(RowSelect.command(KeyPress(Key.V, ctrl = true), HostOs.WINDOWS))
        // 一覧のキーはショートカットとぶつからない(パレット・印刷などに当たらない)
        for (os in HostOs.entries) {
            assertNull(Keymap.match(KeyPress(Key.A, ctrl = os != HostOs.MAC, meta = os == HostOs.MAC), os))
            assertNull(Keymap.match(KeyPress(Key.C, ctrl = os != HostOs.MAC, meta = os == HostOs.MAC), os))
        }
    }

    // MARK: - コピーした TSV

    @Test
    fun tableCopiesOnlySelectedRowsWithHeaderInViewOrder() {
        val headers = listOf("品名", "メモ", "金額")
        val rows = listOf(
            TableCells.Row("1", listOf("りんご", "赤い", "¥120")),
            TableCells.Row("2", listOf("みかん", "タブ\tあり", "¥80")),
            TableCells.Row("3", listOf("ぶどう", "\"甘い\"", "¥300")),
        )
        val sel = RowSelect.click(RowSelect.click(RowSelection(ids = setOf("1", "2", "3")), "3", listOf("1", "2", "3"), plain), "2", listOf("1", "2", "3"), toggle)
        val view = rows.reversed()
        assertEquals(
            "品名\tメモ\t金額\r\n" +
                "ぶどう\t\"\"\"甘い\"\"\"\t¥300\r\n" +
                "みかん\t\"タブ\tあり\"\t¥80\r\n",
            TableCopy.tsv(headers, view, sel),
        )
    }

    @Test
    fun listCopiesOnlySelectedRecordsInTheSameShapeAsCopyTable() {
        TimeZone.setDefault(TimeZone.getTimeZone("Asia/Tokyo"))
        FieldCoding.zone = ZoneId.of("Asia/Tokyo")
        val now = Instant.parse("2026-10-24T03:00:00Z")
        val entities = Examples.load("file_import").entityModels()
        val member = entities.first { it.id == "member" }
        val store = MockStore()
            .save("member", null, mapOf("name" to "山田 太郎", "grade" to "2", "part" to "前衛", "joined" to "2026-04-10"))
            .save("member", null, mapOf("name" to "鈴木 一郎", "grade" to "1", "part" to "後衛", "joined" to "2026-05-01"))
            .save("member", null, mapOf("name" to "佐藤 花子", "part" to "未定", "joined" to "2025-04-01"))
        val ids = store.list("member").map { it.id }
        val all = RecordIo.exportTable("部員", member, store, entities, null, locale = Locale.JAPAN, now = now)
        val picked = RecordIo.exportTable("部員", member, store, entities, null, locale = Locale.JAPAN, now = now, ids = setOf(ids[0], ids[2]))
        assertEquals(all.headers, picked.headers)
        assertEquals(3, all.rows.size)
        // 選んだ 2 件だけ・全部の時と同じ並び(古い順)と書式
        assertEquals(all.rows.filter { it[0] != "鈴木 一郎" }, picked.rows)
        assertEquals(
            "名前\t学年\tパート\t入部日\r\n" +
                "佐藤 花子\t\t未定\t2025/4/1(火)\r\n" +
                "山田 太郎\t2\t前衛\t4/10(金)\r\n",
            jp.kz2601.metamon.desktop.io.Tsv.string(picked),
        )
    }
}
