package jp.kz2601.metamon.desktop

import jp.kz2601.metamon.dsl.Action
import jp.kz2601.metamon.dsl.ActionType
import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.Presentation
import jp.kz2601.metamon.dsl.decode
import jp.kz2601.metamon.renderer.form.FormDraft
import jp.kz2601.metamon.renderer.navigation.Route
import org.junit.Assert.assertEquals
import org.junit.Test

/** ボタンを出すか(isButtonVisible)と、下書きを残す route の id(liveRouteIds)。 */
class SessionTest {
    // MARK: - isButtonVisible

    /** groceries を共有(作った人だけが変えられる)にした shopping。 */
    private fun sharedShopping(): AppDSL {
        val text = javaClass.classLoader.getResourceAsStream("examples/shopping.json")!!.use { it.readBytes().decodeToString() }
        return AppDSL.decode(text.replaceFirst("\"titleField\": \"name\"", "\"scope\": \"shared\", \"titleField\": \"name\""))
    }

    private fun formFor(recordId: String?): FormDraft {
        val store = MockStore(
            mapOf(
                "groceries" to listOf(
                    Row("mine", mapOf("name" to "牛乳"), author = "me"),
                    Row("theirs", mapOf("name" to "卵"), author = "someone"),
                ),
            ),
        )
        val s = DesktopSession(sharedShopping(), store, uid = "me")
        return s.form(Route("item_form", recordId, Route.SHEET))!!
    }

    private val save = Action(type = ActionType.SAVE_RECORD, entity = "groceries")
    private val delete = Action(type = ActionType.DELETE_RECORD, entity = "groceries")
    private val navigate = Action(type = ActionType.NAVIGATE, screen = "home")

    @Test
    fun buttonVisibilityByFormStateAndActionType() {
        data class Case(val name: String, val form: FormDraft?, val save: Boolean, val delete: Boolean)
        val cases = listOf(
            Case("フォームの無い画面", null, save = true, delete = false),
            Case("新規", formFor(null), save = true, delete = false),
            Case("自分の記録の編集", formFor("mine"), save = true, delete = true),
            Case("作者以外の記録(読み取りだけ)", formFor("theirs"), save = false, delete = false),
        )
        for (c in cases) {
            assertEquals("${c.name}: 保存", c.save, isButtonVisible(save, c.form))
            assertEquals("${c.name}: 削除", c.delete, isButtonVisible(delete, c.form))
            assertEquals("${c.name}: 他のアクションはいつも出す", true, isButtonVisible(navigate, c.form))
            assertEquals("${c.name}: アクション無し", true, isButtonVisible(null, c.form))
        }
    }

    @Test
    fun deleteFollowsCanDeleteEvenWhenEditing() {
        val form = formFor("mine")
        form.canDelete = false
        assertEquals(false, isButtonVisible(delete, form))
        form.isReadOnly = true
        assertEquals("読み取りだけなら保存は隠す", false, isButtonVisible(save, form))
        form.canDelete = true
        assertEquals("削除は canDelete だけで決まる", true, isButtonVisible(delete, form))
    }

    // MARK: - liveRouteIds

    /** タブ a / other・initialScreen a・画面 b / c。 */
    private fun navigation() = DesktopSession(Examples.load("navigation"))

    private fun go(s: DesktopSession, to: String, presentation: Presentation = Presentation.PUSH) =
        s.dispatch(Action(type = ActionType.NAVIGATE, screen = to, presentation = presentation), ActionContext(s.top, null))

    private val roots = setOf("tab:a/a/", "tab:other/other/", "main/a/")

    @Test
    fun initialHasTabRootsAndInitialScreen() {
        assertEquals(roots, navigation().liveRouteIds())
    }

    @Test
    fun pushedRoutesAreLive() {
        val s = navigation()
        go(s, "b")
        go(s, "c")
        assertEquals(roots + setOf("tab:a/b/", "tab:a/c/"), s.liveRouteIds())
        s.back()
        assertEquals(roots + "tab:a/b/", s.liveRouteIds())
    }

    @Test
    fun sheetAndFullScreenOnTopOfAPush() {
        val s = navigation()
        go(s, "b")
        go(s, "c", Presentation.SHEET)
        assertEquals(roots + setOf("tab:a/b/", "sheet/c/"), s.liveRouteIds())
        go(s, "b", Presentation.FULL_SCREEN)
        assertEquals(roots + setOf("tab:a/b/", "sheet/c/", "full/b/"), s.liveRouteIds())
        s.back()
        assertEquals("全画面を閉じたら外れる", roots + setOf("tab:a/b/", "sheet/c/"), s.liveRouteIds())
        s.back()
        assertEquals("sheet を閉じたら外れる", roots + "tab:a/b/", s.liveRouteIds())
    }

    @Test
    fun otherTabsStackStaysLive() {
        val s = navigation()
        go(s, "b")
        s.selectTab("other")
        go(s, "b")
        assertEquals("選んでいないタブに積んだ物も残す", roots + setOf("tab:a/b/", "tab:other/b/"), s.liveRouteIds())
    }
}
