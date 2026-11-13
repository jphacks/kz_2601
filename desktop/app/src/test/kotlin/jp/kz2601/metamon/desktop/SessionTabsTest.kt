package jp.kz2601.metamon.desktop

import jp.kz2601.metamon.dsl.Action
import jp.kz2601.metamon.dsl.ActionType
import jp.kz2601.metamon.dsl.Presentation
import jp.kz2601.metamon.dsl.records.JSONValue
import jp.kz2601.metamon.renderer.navigation.Route
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

/** タブの切り替え(selectTab): タブごとの積んだ画面と下書きを残す。navigate でタブの根へ行くと積んだ画面を戻す。 */
class SessionTabsTest {
    private fun kakeibo() = DesktopSession(Examples.load("kakeibo"), MockStore())

    private fun DesktopSession.run(a: Action) = dispatch(a, ActionContext(top, form(top)))

    private val pushForm = Action(type = ActionType.NAVIGATE, screen = "expense_form", presentation = Presentation.PUSH)
    private val home = Route.tabStack("home")

    /** home のタブに入力画面を積み、メモを打ちかけにする。 */
    private fun DesktopSession.pushDraft(): Route {
        run(pushForm)
        val route = top
        form(route)!!.values["memo"] = JSONValue.Str("書きかけ")
        return route
    }

    @Test
    fun selectTabKeepsEachTabStackAndDraft() {
        val s = kakeibo()
        assertEquals("home", s.nav.selectedTab)
        val form = s.pushDraft()
        assertEquals("expense_form", form.screen)
        assertEquals(home, form.stack)

        s.selectTab("stats")
        assertEquals(Route("stats", stack = Route.tabStack("stats")), s.top)
        // 前のタブの積んだ画面と下書きは残る
        assertEquals(listOf(form), s.nav.path(home))
        assertTrue(form.id in s.draftRouteIds)

        s.selectTab("home")
        assertEquals(form, s.top)
        assertEquals("書きかけ", s.form(s.top)!!.values["memo"]?.stringValue)
    }

    @Test
    fun selectingTheSameTabDoesNotPop() {
        val s = kakeibo()
        val form = s.pushDraft()
        s.selectTab("home")
        assertEquals(form, s.top)
        assertTrue(form.id in s.draftRouteIds)
    }

    @Test
    fun navigateToTabRootResetsStackAndDropsDraft() {
        val s = kakeibo()
        val form = s.pushDraft()
        s.run(Action(type = ActionType.NAVIGATE, screen = "home"))
        assertEquals(Route("home", stack = home), s.top)
        assertTrue(s.nav.path(home).isEmpty())
        assertFalse("戻した画面の下書きは捨てる", form.id in s.draftRouteIds)

        // 別のタブから navigate で戻っても、そのタブは根から
        s.pushDraft()
        s.selectTab("stats")
        s.run(Action(type = ActionType.NAVIGATE, screen = "home"))
        assertEquals("home", s.nav.selectedTab)
        assertEquals(Route("home", stack = home), s.top)
        assertTrue(s.draftRouteIds.none { it.startsWith("$home/expense_form") })
    }

    @Test
    fun selectTabUnderASheetKeepsTheSheetOnTop() {
        val s = kakeibo()
        s.run(Action(type = ActionType.NAVIGATE, screen = "expense_form", presentation = Presentation.SHEET))
        val sheet = s.top
        assertEquals(Route.SHEET, sheet.stack)
        s.form(sheet)!!.values["memo"] = JSONValue.Str("シート")
        s.selectTab("stats")
        // 選んだタブは変わるが、シートは閉じない(下書きも残る)
        assertEquals("stats", s.nav.selectedTab)
        assertEquals(sheet, s.top)
        assertEquals("シート", s.form(s.top)!!.values["memo"]?.stringValue)
        s.back()
        assertEquals(Route("stats", stack = Route.tabStack("stats")), s.top)
    }
}
