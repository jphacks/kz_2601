package jp.kz2601.metamon.desktop.io

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test
import java.awt.datatransfer.Clipboard
import java.awt.datatransfer.DataFlavor
import java.awt.datatransfer.StringSelection

/** 書き出しのクリップボード: 自分の入れた表の時だけ消す(作り物の Clipboard で試す)。 */
class ExportClipboardTest {
    private fun text(cb: Clipboard) = cb.getData(DataFlavor.stringFlavor) as String

    @Before
    fun forgetPrevious() {
        // 前のテストの覚えを捨てる(空のクリップボードで呼ぶと覚えだけ消える)
        ExportClipboard.clearIfOurs(Clipboard("reset"))
    }

    @Test
    fun clearsOurOwnTable() {
        val cb = Clipboard("test")
        assertTrue(ExportClipboard.put("名前\t学年\r\n山田\t1\r\n", cb))
        assertEquals("名前\t学年\r\n山田\t1\r\n", text(cb))
        assertTrue(ExportClipboard.clearIfOurs(cb))
        assertEquals("", text(cb))
    }

    @Test
    fun keepsWhatUserCopiedAfterwards() {
        val cb = Clipboard("test")
        ExportClipboard.put("表", cb)
        cb.setContents(StringSelection("自分のメモ"), null)
        assertFalse(ExportClipboard.clearIfOurs(cb))
        assertEquals("自分のメモ", text(cb))
    }

    @Test
    fun forgetsMarkAfterOneCheck() {
        val cb = Clipboard("test")
        ExportClipboard.put("表", cb)
        cb.setContents(StringSelection("別"), null)
        assertFalse(ExportClipboard.clearIfOurs(cb))
        // 利用者が同じ文字を入れ直しても、もう自分の物とはみなさない
        cb.setContents(StringSelection("表"), null)
        assertFalse(ExportClipboard.clearIfOurs(cb))
        assertEquals("表", text(cb))
    }

    @Test
    fun onlyLatestPutCounts() {
        val cb = Clipboard("test")
        ExportClipboard.put("一つ目", cb)
        ExportClipboard.put("二つ目", cb)
        cb.setContents(StringSelection("一つ目"), null)
        assertFalse(ExportClipboard.clearIfOurs(cb))
        assertEquals("一つ目", text(cb))
    }

    @Test
    fun nothingPutOrNoClipboard() {
        val cb = Clipboard("test").apply { setContents(StringSelection("x"), null) }
        assertFalse(ExportClipboard.clearIfOurs(cb))
        assertEquals("x", text(cb))
        assertFalse(ExportClipboard.put("表", null))
        assertFalse(ExportClipboard.clearIfOurs(null))
    }

    @Test
    fun emptyClipboardIsNotOurs() {
        val cb = Clipboard("test")
        ExportClipboard.put("表", cb)
        // 何も入っていないクリップボード(読めない)は触らない
        assertFalse(ExportClipboard.clearIfOurs(Clipboard("other")))
        assertEquals("表", text(cb))
    }
}
