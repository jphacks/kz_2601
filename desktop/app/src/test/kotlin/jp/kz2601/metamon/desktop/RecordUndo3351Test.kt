package jp.kz2601.metamon.desktop

import androidx.compose.ui.input.key.Key
import jp.kz2601.metamon.desktop.i18n.Lang
import jp.kz2601.metamon.desktop.i18n.StringTable
import jp.kz2601.metamon.dsl.Action
import jp.kz2601.metamon.dsl.ActionType
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/** 一覧・表の行を消した後の「削除しました・元に戻す」と Ctrl+Z(#3351)。 */
class RecordUndo3351Test {
    private fun store() = MockStore(
        mapOf("groceries" to listOf(Row("a", mapOf("name" to "牛乳")), Row("b", mapOf("name" to "卵")), Row("c", mapOf("name" to "パン")))),
    )

    private fun session(): DesktopSession = DesktopSession(Examples.load("shopping"), store())

    private fun deleteRow(s: DesktopSession, id: String) =
        s.dispatch(Action(type = ActionType.DELETE_RECORD, entity = "groceries"), ActionContext(s.top, null, id))

    private fun names(s: DesktopSession) = s.store.list("groceries").map { it.data["name"] }

    @Test
    fun restorePutsTheRowBackAtTheSamePlace() {
        val t = RecordUndo.ticket(store(), "groceries", "b", 1)!!
        assertEquals(1, t.index)
        val restored = RecordUndo.restore(store().delete("groceries", "b"), t)!!
        assertEquals(listOf("a", "b", "c"), restored.list("groceries").map { it.id })
        assertNull("無い記録には札を作らない", RecordUndo.ticket(store(), "groceries", "zzz", 2))
        assertNull("同じ id があれば戻さない(他のウィンドウで戻した)", RecordUndo.restore(store(), t))
        // 間で行が減っていても端に収める
        val shorter = MockStore(mapOf("groceries" to listOf(Row("a", emptyMap()))))
        assertEquals(listOf("a", "b"), RecordUndo.restore(shorter, t.copy(index = 5))!!.list("groceries").map { it.id })
    }

    @Test
    fun rowDeleteOffersUndoAndUndoRestores() {
        val s = session()
        deleteRow(s, "b")
        assertEquals(listOf("牛乳", "パン"), names(s))
        assertEquals("b", s.recordUndo?.row?.id)
        assertTrue(s.undoDelete())
        assertEquals(listOf("牛乳", "卵", "パン"), names(s))
        assertNull("戻したら帯は消える", s.recordUndo)
        assertFalse("2 回目は戻す物が無い", s.undoDelete())
    }

    @Test
    fun onlyTheLastDeleteIsUndoneAndStaleDismissKeepsTheNewBar() {
        val s = session()
        deleteRow(s, "a")
        val first = s.recordUndo
        deleteRow(s, "c")
        s.dismissRecordUndo(first)
        assertNotNull("前の帯の時間切れで後の帯を消さない", s.recordUndo)
        assertTrue(s.undoDelete())
        assertEquals(listOf("卵", "パン"), names(s))
        deleteRow(s, "b")
        s.dismissRecordUndo()
        assertFalse("閉じた後は戻せない", s.undoDelete())
    }

    @Test
    fun formDeleteButtonDoesNotOfferUndo() {
        val s = session()
        s.dispatch(Action(type = ActionType.NAVIGATE, screen = "item_form"), ActionContext(s.top, null, "a"))
        assertEquals("a", s.top.recordId)
        assertEquals("item_form", s.top.screen)
        s.dispatch(Action(type = ActionType.DELETE_RECORD, entity = "groceries"), ActionContext(s.top, s.form(s.top)))
        assertEquals(2, s.store.list("groceries").size)
        assertNull(s.recordUndo)
    }

    @Test
    fun ctrlZOnWindowsAndLinuxCommandZOnMacAndFallsThroughWithNothingToUndo() {
        assertEquals(Shortcut.UNDO, Keymap.match(KeyPress(Key.Z, ctrl = true), HostOs.WINDOWS))
        assertEquals(Shortcut.UNDO, Keymap.match(KeyPress(Key.Z, ctrl = true), HostOs.LINUX))
        assertEquals(Shortcut.UNDO, Keymap.match(KeyPress(Key.Z, meta = true), HostOs.MAC))
        assertNull("Ctrl+Shift+Z はやり直し(取らない)", Keymap.match(KeyPress(Key.Z, ctrl = true, shift = true), HostOs.WINDOWS))
        assertEquals("Ctrl+Z", Keymap.label(Shortcut.UNDO, HostOs.WINDOWS))
        assertEquals("⌘Z", Keymap.label(Shortcut.UNDO, HostOs.MAC))
        assertTrue("入力欄の文字の取り消しを先に通す", Shortcut.UNDO.bubble)

        val s = session()
        val k = DesktopKeyboard(s.dsl, s, HostOs.WINDOWS)
        assertFalse("戻す物が無ければ流す", k.run(Shortcut.UNDO))
        deleteRow(s, "a")
        assertFalse("入力欄より先には取らない", k.handle(KeyPress(Key.Z, ctrl = true), bubble = false))
        assertTrue(k.handle(KeyPress(Key.Z, ctrl = true), bubble = true))
        assertEquals(listOf("牛乳", "卵", "パン"), names(s))
    }

    @Test
    fun barTextUsesTheAndroidKeys() {
        val t = StringTable.fromClasspath()
        for (key in listOf("feedback_deleted", "feedback_undo", "feedback_close")) {
            assertTrue("$key(ja)", t.hasString(Lang.JA, key))
            assertTrue("$key(en)", t.hasString(Lang.EN, key))
        }
        assertEquals("削除しました", t.get(Lang.JA, "feedback_deleted"))
        assertEquals("Deleted", t.get(Lang.EN, "feedback_deleted"))
        assertEquals("元に戻す", t.get(Lang.JA, "feedback_undo"))
        assertEquals("Undo", t.get(Lang.EN, "feedback_undo"))
    }
}
