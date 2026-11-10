package jp.kz2601.metamon.renderer

import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.renderer.navigation.NavigationResolver
import jp.kz2601.metamon.renderer.navigation.NavigationState
import jp.kz2601.metamon.renderer.navigation.Presentation
import jp.kz2601.metamon.renderer.navigation.Route
import jp.kz2601.metamon.renderer.navigation.top
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

class NavigationResolverTest {
    private fun dsl(tabs: String = "") = AppDSL.decode(
        """
        {"schemaVersion":1,"app":{"name":"t"$tabs},"initialScreen":"home",
         "screens":[{"id":"home","title":"H","components":[]},{"id":"list","title":"L","components":[]},
                    {"id":"detail","title":"D","components":[]},{"id":"settings","title":"S","components":[]}]}
        """.trimIndent(),
    )

    private fun go(s: NavigationState, to: String, dsl: AppDSL, p: Presentation = Presentation.PUSH) =
        NavigationResolver.navigate(s, to, p, null, s.top(dsl), dsl)

    @Test
    fun pushesAndPopsBackToExistingScreen() {
        val d = dsl()
        var s = NavigationState.initial(d)
        s = go(s, "list", d)
        s = go(s, "detail", d)
        assertEquals(listOf("list", "detail"), s.path(Route.MAIN).map { it.screen })
        // 既にスタックにある画面へは積み直さず、そこまで戻る
        s = go(s, "list", d)
        assertEquals(listOf("list"), s.path(Route.MAIN).map { it.screen })
        // 根へ
        s = go(s, "home", d)
        assertEquals(Route("home"), s.top(d))
        assertEquals(emptyList<Route>(), s.path(Route.MAIN))
    }

    @Test
    fun backPopsAndDoesNotCloseRoot() {
        val d = dsl()
        var s = go(NavigationState.initial(d), "list", d)
        s = NavigationResolver.close(s, s.top(d))
        assertEquals(Route("home"), s.top(d))
        assertEquals(s, NavigationResolver.close(s, s.top(d)))
    }

    @Test
    fun switchesToTabRoot() {
        val d = dsl(""","tabs":[{"screen":"home","title":"ホーム"},{"screen":"settings","title":"設定"}]""")
        var s = NavigationState.initial(d)
        assertEquals("home", s.selectedTab)
        s = go(s, "list", d)
        assertEquals(Route("list", stack = "tab:home"), s.top(d))
        s = go(s, "settings", d)
        assertEquals("settings", s.selectedTab)
        assertEquals(Route("settings", stack = "tab:settings"), s.top(d))
    }

    @Test
    fun sheetOpensAndClosesAndPushInsideSheetFallsBackToUnderlyingStack() {
        val d = dsl()
        var s = go(NavigationState.initial(d), "list", d)
        s = go(s, "detail", d, Presentation.SHEET)
        assertEquals(Route("detail", stack = Route.SHEET), s.top(d))
        // シートの中から下の画面にある list へ → シートを閉じて list に戻る
        s = go(s, "list", d)
        assertNull(s.sheet)
        assertEquals(Route("list"), s.top(d))

        s = go(s, "settings", d, Presentation.FULL_SCREEN)
        s = NavigationResolver.close(s, s.top(d))
        assertNull(s.fullScreen)
    }
}
