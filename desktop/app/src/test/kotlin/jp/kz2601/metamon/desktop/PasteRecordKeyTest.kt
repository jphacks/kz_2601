package jp.kz2601.metamon.desktop

import androidx.compose.ui.input.key.Key
import jp.kz2601.metamon.desktop.i18n.Lang
import jp.kz2601.metamon.desktop.i18n.Strings
import jp.kz2601.metamon.desktop.io.RecordClipboard
import jp.kz2601.metamon.dsl.records.JSONValue
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/** Ctrl+V(macOS は ⌘V)で、項目つきでコピーした記録をフォームに貼る(#2987)。 */
class PasteRecordKeyTest {
    private fun clip(vararg fields: Triple<String, String, JSONValue>): RecordClipboard.Clip? {
        val p = RecordClipboard.Payload("other", "他", "x", "x", fields.map { (name, type, v) -> RecordClipboard.FieldValue(name, name, type, v) })
        return RecordClipboard.clip(p)
    }

    private fun openForm(os: HostOs = HostOs.WINDOWS): Pair<DesktopSession, DesktopKeyboard> {
        val s = DesktopSession(Examples.load("shopping"), MockStore())
        val k = DesktopKeyboard(s.dsl, s, os)
        assertTrue(k.run(Shortcut.NEW))
        k.rootFocused = true
        return s to k
    }

    @Test
    fun ctrlVFillsMatchingFieldsAndReportsCounts() = Strings.withLang(Lang.JA) {
        val (s, k) = openForm()
        k.clipboard = { clip(Triple("品名", "string", JSONValue.Str("牛乳")), Triple("個数", "integer", JSONValue.Num(2.0)), Triple("無い項目", "string", JSONValue.Str("x"))) }
        assertTrue(k.handle(KeyPress(Key.V, ctrl = true), bubble = false))
        val v = s.form(s.top)!!.values
        assertEquals(JSONValue.Str("牛乳"), v["name"])
        assertEquals(JSONValue.Num(2.0), v["qty"])
        assertEquals("2 件の項目を貼り付けました(入らなかった項目 1 件)", s.message)
    }

    @Test
    fun commandVOnMacCtrlVNot() {
        val (s, k) = openForm(HostOs.MAC)
        k.clipboard = { clip(Triple("品名", "string", JSONValue.Str("牛乳"))) }
        assertFalse(k.handle(KeyPress(Key.V, ctrl = true), bubble = false))
        assertTrue(k.handle(KeyPress(Key.V, meta = true), bubble = false))
        assertEquals(JSONValue.Str("牛乳"), s.form(s.top)!!.values["name"])
    }

    @Test
    fun whileAnInputIsFocusedItIsPlainPaste() {
        val (s, k) = openForm()
        k.rootFocused = false
        k.clipboard = { clip(Triple("品名", "string", JSONValue.Str("牛乳"))) }
        assertFalse(k.handle(KeyPress(Key.V, ctrl = true), bubble = false))
        assertNull(s.form(s.top)!!.values["name"]?.stringValue)
    }

    @Test
    fun nothingPastedWhenClipboardHasNoRecord() {
        val (s, k) = openForm()
        k.clipboard = { null }
        assertFalse(k.handle(KeyPress(Key.V, ctrl = true), bubble = false))
        assertNull(s.message)
    }

    @Test
    fun saysSoWhenNothingFits() = Strings.withLang(Lang.EN) {
        val (s, k) = openForm()
        k.clipboard = { clip(Triple("無い", "string", JSONValue.Str("x"))) }
        assertTrue(k.handle(KeyPress(Key.V, ctrl = true), bubble = false))
        assertEquals("No fields to paste", s.message)
    }

    @Test
    fun screenWithoutFormIsLeftAlone() {
        val s = DesktopSession(Examples.load("shopping"), MockStore())
        val k = DesktopKeyboard(s.dsl, s, HostOs.WINDOWS)
        k.rootFocused = true
        k.clipboard = { clip(Triple("品名", "string", JSONValue.Str("牛乳"))) }
        assertFalse("一覧の画面にはフォームが無い", k.handle(KeyPress(Key.V, ctrl = true), bubble = false))
    }
}
