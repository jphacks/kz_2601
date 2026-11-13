package jp.kz2601.metamon.desktop.gridedit

import jp.kz2601.metamon.desktop.AppHub
import jp.kz2601.metamon.desktop.GridArea
import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.decode
import jp.kz2601.metamon.dsl.screen
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertSame
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.rules.TemporaryFolder

/** 並びを直す編集の画面の状態・保存・再起動して当て直す(#3207)。 */
class GridEditLayer3207Test {
    @get:Rule
    val tmp = TemporaryFolder()

    private fun area(s: String): GridArea = GridArea.parse(s)!!

    private val dsl = AppDSL.decode(
        """
        {"schemaVersion":1,"app":{"name":"t"},"initialScreen":"home","entities":[{"id":"item","fields":[{"id":"name","type":"text"}]}],
         "screens":[
          {"id":"form","title":"入力","entity":"item","components":[{"type":"textField","field":"name","area":"A1:F1"}]},
          {"id":"home","title":"ホーム","components":[
            {"id":"summary","type":"text","text":"合計","area":"A1:F2"},
            {"id":"note","type":"text","text":"積む"},
            {"id":"recent","type":"text","text":"一覧","area":"A3:F10"},
            {"id":"add","type":"button","label":"追加","area":"D11:F12","action":{"type":"navigate","screen":"home"}}
          ]}
         ]}
        """.trimIndent(),
    )

    @Test
    fun onlyPlainGridScreensAreEditable() {
        assertNull("フォーム画面は編集しない", dsl.gridEditDraft("form"))
        assertNull(dsl.gridEditDraft("missing"))
        val d = dsl.gridEditDraft("home")!!
        assertEquals(1, d.screenIndex)
        assertEquals(listOf("summary", "recent", "add"), d.items.map { it.componentId })
    }

    @Test
    fun arrowKeysMoveSelectedOneCellAndRefuseOverlap() {
        var s = GridEditSession(dsl.gridEditDraft("home")!!)
        assertEquals("summary", s.selected)
        s = s.cycle(forward = true).cycle(forward = true)
        assertEquals("add", s.selected)
        // 上は一覧と重なる
        val (same, blocked) = s.nudge(0, -1)
        assertEquals(GridEditSession.Nudge.BLOCKED, blocked)
        assertEquals("D11:F12", same.selectedItem!!.area.notation)
        // 右端より右へは出ない
        assertEquals(GridEditSession.Nudge.EDGE, s.nudge(1, 0).second)
        val (left, moved) = s.nudge(-1, 0)
        assertEquals(GridEditSession.Nudge.MOVED, moved)
        assertEquals("C11:E12", left.selectedItem!!.area.notation)
        assertTrue(left.draft.hasChanges)
        // Shift+Tab で前へ・先頭から前は最後
        assertEquals("recent", left.cycle(forward = false).selected)
        assertEquals("add", GridEditSession(left.draft, "summary").cycle(forward = false).selected)
    }

    @Test
    fun dropOnOverlapKeepsOriginal() {
        val s = GridEditSession(dsl.gridEditDraft("home")!!)
        val (refused, ok) = s.drop("add", area("A10:C11"))
        assertFalse(ok)
        assertEquals("add", refused.selected)
        assertEquals("D11:F12", refused.draft.item("add")!!.area.notation)
        val (placed, ok2) = s.drop("add", area("A11:C12"))
        assertTrue(ok2)
        assertEquals("A11:C12", placed.draft.item("add")!!.area.notation)
    }

    @Test
    fun spokenPositionIsRowAndColumnLetter() {
        val item = dsl.gridEditDraft("home")!!.item("add")!!
        assertEquals(11 to 'D', item.positionLabel())
    }

    @Test
    fun patchAppliesLikeLocalApply() {
        val d = dsl.gridEditDraft("home")!!.place("add", area("A11:C12"))!!.remove("summary")!!
        val viaPatch = GridEditPatch.apply(dsl, d.patch)!!
        assertEquals(d.applied(dsl), viaPatch)
        assertEquals(listOf("note", "recent", "add"), viaPatch.screen("home")!!.components.map { it.id })
    }

    @Test
    fun patchWithStaleTestIsNotApplied() {
        val d = dsl.gridEditDraft("home")!!.place("add", area("A11:C12"))!!
        val moved = d.applied(dsl)
        // 同じ patch をもう一度: 元の area の test が合わない
        assertNull(GridEditPatch.apply(moved, d.patch))
    }

    @Test
    fun savedLayoutSurvivesRestart() {
        val dir = tmp.root.resolve("layouts")
        val first = LayoutEditStore(dir)
        val d1 = dsl.gridEditDraft("home")!!.place("add", area("A11:C12"))!!
        first.append("t", d1.patch)
        val after1 = d1.applied(dsl)
        // 2 回目は 1 回目を当てた DSL の上で直す
        val d2 = after1.gridEditDraft("home")!!.place("add", area("A13:C14"))!!
        first.append("t", d2.patch)
        first.append("t", emptyList())

        // 再起動: 新しい store で読み直し、同梱の DSL に当て直す
        val reopened = LayoutEditStore(dir)
        assertEquals(2, reopened.load("t").size)
        val out = reopened.applyTo("t", dsl)
        assertEquals("A13:C14", out.screen("home")!!.components.first { it.id == "add" }.area)
        // ほかのアプリには当てない
        assertSame(dsl, reopened.applyTo("other", dsl))

        reopened.removeAll()
        assertFalse(dir.exists())
        assertSame(dsl, LayoutEditStore(dir).applyTo("t", dsl))
    }

    @Test
    fun brokenFileIsIgnored() {
        val dir = tmp.newFolder("layouts")
        dir.resolve("t.json").writeText("{broken")
        assertSame(dsl, LayoutEditStore(dir).applyTo("t", dsl))
    }

    @Test
    fun hubReplacesDslForOpenWindowsAndTray() {
        val hub = AppHub(load = { dsl })
        val app = hub.app("t")
        val next = dsl.gridEditDraft("home")!!.place("add", area("A11:C12"))!!.applied(dsl)
        hub.replaceDsl("t", next)
        assertSame(next, app.dsl)
        assertSame(next, hub.dsl("t"))
    }
}
