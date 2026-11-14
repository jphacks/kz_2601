package jp.kz2601.metamon.runtime

import jp.kz2601.metamon.DebugKnobs.AutoNavStep
import jp.kz2601.metamon.renderer.navigation.Route
import jp.kz2601.metamon.runtime.RuntimeFixture.runtime
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/** #1024: `METAMON_AUTO_NAV` は initialScreen の差し替えでなく、本物の navigate で進む(タブ・push・sheet)。 */
class AutoNavTest {
    private val kakeibo = RuntimeFixture.dsl("kakeibo")

    @Test
    fun walksThroughRealNavigation() = runTest {
        val rt = runtime(kakeibo)
        val steps = listOf(AutoNavStep("stats"), AutoNavStep("expense_form"), AutoNavStep("home"), AutoNavStep("expense_form", "sheet"))
        val lines = steps.mapIndexed { i, s -> AutoNav.step(rt, s, i, steps.size) }

        val nav = rt.navigation.value
        // stats タブへ切り替え → その上に push → home タブへ → sheet で開く
        assertEquals("home", nav.selectedTab)
        assertEquals(Route("expense_form", stack = Route.SHEET), nav.sheet)
        // stats のタブには expense_form が積まれたまま(タブごとのスタック)
        assertEquals(listOf(Route("expense_form", stack = Route.tabStack("stats"))), nav.path(Route.tabStack("stats")))
        // 初期 DSL は書き換えない(以前は initialScreen を差し替えていた)
        assertEquals("home", rt.dsl.initialScreen)
        assertTrue(lines[0], lines[0].startsWith("auto nav 1/4: → stats ok tab=stats"))
        assertTrue(lines[3], lines[3].endsWith("sheet=expense_form full=-"))
    }

    @Test
    fun fullScreenPresentation() = runTest {
        val rt = runtime(kakeibo)
        AutoNav.step(rt, AutoNavStep("expense_form", "fullScreen"), 0, 1)
        assertEquals(Route("expense_form", stack = Route.FULL), rt.navigation.value.fullScreen)
    }

    @Test
    fun missingScreenIsSkipped() = runTest {
        val rt = runtime(kakeibo)
        val before = rt.navigation.value
        val line = AutoNav.step(rt, AutoNavStep("nope"), 0, 1)
        assertEquals(before, rt.navigation.value)
        assertTrue(line, line.contains("not found"))
        assertNull(rt.navigation.value.sheet)
    }
}
