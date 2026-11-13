package jp.kz2601.metamon.desktop.automations

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotEquals
import org.junit.Assert.assertNull
import org.junit.Test

class AutomationFixOfferTest {
    private fun rule(
        id: String,
        failures: Int,
        enabled: Boolean? = true,
        stopped: Boolean = false,
        lastOkAt: String? = null,
    ) = AutomationRule(
        id = id, description = "説明$id", lastOkAt = lastOkAt,
        stopped = if (stopped) AutomationStop() else null, enabled = enabled, consecutiveFailures = failures,
    )

    private fun apps(rules: List<AutomationRule>, stopped: Boolean = false) =
        AppAutomations(appId = "a", stopped = if (stopped) AutomationStop() else null, rules = rules)

    @Test
    fun belowThresholdShowsNothing() {
        assertNull(AutomationFixOffer.pick(apps(listOf(rule("r", 2))), emptySet()))
    }

    @Test
    fun atThresholdShows() {
        assertEquals("r", AutomationFixOffer.pick(apps(listOf(rule("r", 3))), emptySet())?.id)
    }

    @Test
    fun picksMostFailuresThenListOrder() {
        val a = apps(listOf(rule("x", 3), rule("y", 5), rule("z", 5)))
        assertEquals("y", AutomationFixOffer.pick(a, emptySet())?.id)
    }

    @Test
    fun skipsOperatorStoppedAndOff() {
        assertNull(AutomationFixOffer.pick(apps(listOf(rule("r", 4, stopped = true))), emptySet()))
        assertNull(AutomationFixOffer.pick(apps(listOf(rule("r", 4, enabled = false))), emptySet()))
        assertNull(AutomationFixOffer.pick(apps(listOf(rule("r", 4)), stopped = true), emptySet()))
        // 古いサーバー(enabled を返さない)は出す
        assertEquals("r", AutomationFixOffer.pick(apps(listOf(rule("r", 4, enabled = null))), emptySet())?.id)
    }

    @Test
    fun handledStreakIsSkippedButNextStreakShows() {
        val old = rule("r", 3, lastOkAt = "1970-01-01T00:01:40Z")
        val handled = setOf(AutomationFixOffer.key("a", old))
        assertNull(AutomationFixOffer.pick(apps(listOf(old)), handled))
        // 同じ連続で回数が増えても出さない
        assertNull(AutomationFixOffer.pick(apps(listOf(rule("r", 6, lastOkAt = "1970-01-01T00:01:40Z"))), handled))
        // 一度うまくいってから、また続けて失敗した = 次の連続
        val next = rule("r", 3, lastOkAt = "1970-01-01T00:08:20Z")
        assertEquals("r", AutomationFixOffer.pick(apps(listOf(next)), handled)?.id)
        // 閉じたルールの代わりに、ほかのルールを出す
        assertEquals("s", AutomationFixOffer.pick(apps(listOf(old, rule("s", 3))), handled)?.id)
    }

    @Test
    fun keyIsPerAppAndUsesEpochSecondsOrNever() {
        val r = rule("r", 3)
        assertNotEquals(AutomationFixOffer.key("a", r), AutomationFixOffer.key("b", r))
        assertEquals("a|r|never", AutomationFixOffer.key("a", r))
        assertEquals("a|r|100", AutomationFixOffer.key("a", rule("r", 3, lastOkAt = "1970-01-01T00:01:40Z")))
        assertEquals("a|r|odd", AutomationFixOffer.key("a", rule("r", 3, lastOkAt = "odd")))
    }

    @Test
    fun emptyListShowsNothing() {
        assertNull(AutomationFixOffer.pick(apps(emptyList()), emptySet()))
    }
}
