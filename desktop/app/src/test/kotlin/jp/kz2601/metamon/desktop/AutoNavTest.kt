package jp.kz2601.metamon.desktop

import jp.kz2601.metamon.desktop.net.DesktopApiConfig
import jp.kz2601.metamon.dsl.ActionType
import jp.kz2601.metamon.dsl.Presentation
import kotlinx.coroutines.runBlocking
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/** ヘッドレスで画面を順に移る(iOS の METAMON_AUTO_NAV と同じ書き方)。 */
class AutoNavTest {
    @Test
    fun parsesScreensPresentationsAndInterval() {
        val plan = AutoNavPlan.parse("b,c,a,c:sheet,other:fullScreen", "0.5")!!
        assertEquals(
            listOf(
                AutoNavStep("b", null), AutoNavStep("c", null), AutoNavStep("a", null),
                AutoNavStep("c", "sheet"), AutoNavStep("other", "fullScreen"),
            ),
            plan.steps,
        )
        assertEquals(0.5, plan.intervalSeconds, 0.0)
        assertEquals("c:sheet", plan.steps[3].toString())
    }

    @Test
    fun intervalDefaultsToTwoSeconds() {
        assertEquals(2.0, AutoNavPlan.parse("a")!!.intervalSeconds, 0.0)
        assertEquals(2.0, AutoNavPlan.parse("a", "abc")!!.intervalSeconds, 0.0)
        assertEquals(2.0, AutoNavPlan.parse("a", "-1")!!.intervalSeconds, 0.0)
        assertEquals(2.0, AutoNavPlan.parse("a", "NaN")!!.intervalSeconds, 0.0)
        assertEquals(0.0, AutoNavPlan.parse("a", "0")!!.intervalSeconds, 0.0)
    }

    @Test
    fun emptyOrMissingSpecDoesNothing() {
        assertNull(AutoNavPlan.parse(null))
        assertNull(AutoNavPlan.parse(""))
        assertNull(AutoNavPlan.parse(" , ,:sheet"))
        // 空の手は飛ばす
        assertEquals(listOf(AutoNavStep("a", null), AutoNavStep("b", "sheet")), AutoNavPlan.parse("a,,b: sheet,")!!.steps)
    }

    @Test
    fun stepBecomesANavigateAction() {
        val sheet = AutoNavStep("item_form", "sheet").toAction()
        assertEquals(ActionType.NAVIGATE, sheet.type)
        assertEquals("item_form", sheet.screen)
        assertEquals(Presentation.SHEET, sheet.presentation)
        assertEquals(Presentation.FULL_SCREEN, AutoNavStep("x", "fullScreen").toAction().presentation)
        // 知らない presentation は指定なし(push)
        assertNull(AutoNavStep("x", "popover").toAction().presentation)
        assertNull(AutoNavStep("x", null).toAction().presentation)
    }

    @Test
    fun readsTheEnvironmentOnlyForAllowedTargets() {
        val env = mapOf("METAMON_AUTO_NAV" to "a,b:sheet", "METAMON_AUTO_NAV_INTERVAL" to "1")
        assertEquals(2, AutoNavPlan.fromEnvironment(env)!!.steps.size)
        assertEquals(1.0, AutoNavPlan.fromEnvironment(env)!!.intervalSeconds, 0.0)
        assertEquals(2, AutoNavPlan.fromEnvironment(env + ("TAILORIT_API" to "stg"))!!.steps.size)
        assertNull(AutoNavPlan.fromEnvironment(emptyMap()))
        assertTrue(AutoNavPlan.allowed(DesktopApiConfig(DesktopApiConfig.Mode.STG)))
    }

    @Test
    fun runsTheStepsInOrderAndLogsEachOne() = runBlocking {
        val session = DesktopSession(Examples.load("shopping"))
        assertEquals("home", session.top.screen)
        val lines = mutableListOf<String>()

        runAutoNav(AutoNavPlan.parse("item_form:sheet,home", "0")!!, session, startDelayMillis = 0, log = lines::add)

        assertEquals(2, lines.size)
        assertTrue(lines[0], lines[0].startsWith("auto nav 1/2: → item_form:sheet top=item_form sheet=item_form"))
        // home はタブの根でもシートの下の画面でもないので、シートを閉じて戻る(navigate の解決は NavigationResolver)
        assertTrue(lines[1], lines[1].startsWith("auto nav 2/2: → home top=home"))
        assertEquals("home", session.top.screen)
    }
}
