package jp.kz2601.metamon.dsl.logic

import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.decode
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotEquals
import org.junit.Assert.assertTrue
import org.junit.Test

/** iOS の BuildChoreographyTests の editHighlight の分(#894)。 */
class ChangeHighlightTest {
    private fun dsl(vararg screens: String, tabs: String = "[]", initial: String = "home"): AppDSL = AppDSL.decode(
        """{"schemaVersion":1,"app":{"name":"t","tabs":$tabs},"initialScreen":"$initial","screens":[${screens.joinToString(",")}]}""",
    )

    private fun screen(id: String, vararg components: String) = """{"id":"$id","components":[${components.joinToString(",")}]}"""

    private val title = """{"type":"text","id":"title","text":"家計簿","area":"A1:F1"}"""
    private val amount = """{"type":"numberField","id":"amount","label":"金額","field":"amount","area":"A2:C3"}"""
    private val save = """{"type":"button","id":"save","label":"保存","area":"D2:F3"}"""
    private val list = """{"type":"list","id":"items","entity":"expense","area":"A4:F10"}"""

    @Test
    fun addedComponentIsMarked() {
        val old = dsl(screen("home", title, amount, save, list))
        val memo = """{"type":"textField","id":"memo","label":"メモ","field":"memo","area":"A11:F11"}"""
        val new = dsl(screen("home", title, amount, save, list, memo))
        val h = ChangeHighlight.between(old, new)
        assertEquals(setOf("home/memo"), h.components)
        assertEquals(setOf("home"), h.screens)
    }

    @Test
    fun changedContentAndMovedAreMarked() {
        val old = dsl(screen("home", title, amount, save, list))
        val renamed = save.replace("保存", "追加")
        val moved = amount.replace("A2:C3", "A2:B3")
        val h = ChangeHighlight.between(old, dsl(screen("home", title, moved, renamed, list)))
        assertEquals(setOf("home/amount", "home/save"), h.components)
    }

    @Test
    fun newIdsWithSameContentAreNotMarked() {
        // id は付け直されても中身が同じなら変わっていない
        val old = dsl(screen("home", title, amount, save, list))
        val new = dsl(screen("home", title.replace("\"title\"", "\"t2\""), amount, save, list))
        assertTrue(ChangeHighlight.between(old, new).isEmpty)
    }

    @Test
    fun childChangeMarksTheParent() {
        val card = { label: String -> """{"type":"card","id":"c","children":[{"type":"text","text":"$label"}]}""" }
        val old = dsl(screen("home", title, amount, save, card("a")))
        val h = ChangeHighlight.between(old, dsl(screen("home", title, amount, save, card("b"))))
        assertEquals(setOf("home/c"), h.components)
    }

    @Test
    fun newScreenAndMostlyChangedScreenAreNotMarked() {
        val old = dsl(screen("home", title, amount, save, list))
        // 新しい画面は光らせない
        val added = ChangeHighlight.between(old, dsl(screen("home", title, amount, save, list), screen("detail", title)))
        assertTrue(added.isEmpty)
        // 7 割を超えて変わった画面は光らせない(4 つ中 3 つ = 75%)
        val most = dsl(screen("home", title, amount.replace("金額", "値"), save.replace("保存", "OK"), list.replace("expense", "x")))
        assertTrue(ChangeHighlight.between(old, most).isEmpty)
        // 4 つ中 2 つ = 50% は光らせる
        val half = dsl(screen("home", title, amount.replace("金額", "値"), save.replace("保存", "OK"), list))
        assertEquals(2, ChangeHighlight.between(old, half).components.size)
    }

    @Test
    fun duplicatesAreCountedOnce() {
        // 同じ中身が 2 つに増えたら、増えた 1 つだけが変わった扱い
        val old = dsl(screen("home", title, amount, save, list))
        val new = dsl(screen("home", title, amount, save, list, save.replace("\"save\"", "\"save2\"")))
        assertEquals(setOf("home/save2"), ChangeHighlight.between(old, new).components)
    }

    @Test
    fun componentsWithoutIdUseTheirPosition() {
        val noId = """{"type":"text","text":"a"}"""
        val old = dsl(screen("home", noId, title, amount, save))
        val new = dsl(screen("home", noId, title, amount, save, """{"type":"text","text":"b"}"""))
        assertEquals(setOf("home/#4"), ChangeHighlight.between(old, new).components)
    }

    @Test
    fun signatureLooksAtVisibleKeys() {
        val a = dsl(screen("home", save)).screens[0].components[0]
        val b = dsl(screen("home", save.replace("\"id\":\"save\"", "\"id\":\"x\""))).screens[0].components[0]
        val c = dsl(screen("home", save.replace("D2:F3", "D2:F4"))).screens[0].components[0]
        assertEquals(ChangeHighlight.signature(a), ChangeHighlight.signature(b))
        assertNotEquals(ChangeHighlight.signature(a), ChangeHighlight.signature(c))
    }

    @Test
    fun initialTabFollowsInitialScreen() {
        val tabs = """[{"screen":"a","title":"A"},{"screen":"b","title":"B"}]"""
        assertEquals("b", dsl(screen("a"), screen("b"), tabs = tabs, initial = "b").initialTab)
        assertEquals("a", dsl(screen("a"), screen("b"), tabs = tabs, initial = "detail").initialTab)
        assertEquals("", dsl(screen("home")).initialTab)
    }
}
