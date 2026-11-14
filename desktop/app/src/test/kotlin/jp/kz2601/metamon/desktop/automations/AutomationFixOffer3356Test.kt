package jp.kz2601.metamon.desktop.automations

import jp.kz2601.metamon.desktop.i18n.Lang
import jp.kz2601.metamon.desktop.i18n.StringTable
import jp.kz2601.metamon.desktop.i18n.Strings
import jp.kz2601.metamon.desktop.store.JsonKeyValueStore
import jp.kz2601.metamon.services.automations.AppAutomations
import jp.kz2601.metamon.services.automations.AutomationRule
import jp.kz2601.metamon.services.automations.AutomationStop
import jp.kz2601.metamon.services.automations.AutomationsClient
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.IOException

/** 直す入力を開いた時の「直しますか?」: 選び方・押すと依頼文が入る・閉じたら同じ失敗では出さない(#3356)。 */
class AutomationFixOffer3356Test {
    private fun rule(id: String, failures: Int, description: String = id, lastOkAt: String? = null, enabled: Boolean? = true, stopped: AutomationStop? = null) =
        AutomationRule(id = id, description = description, consecutiveFailures = failures, lastOkAt = lastOkAt, enabled = enabled, stopped = stopped)

    private fun apps(vararg rules: AutomationRule, stopped: AutomationStop? = null) = AppAutomations(appId = "app1", rules = rules.toList(), stopped = stopped)

    private class FakeClient(var value: AppAutomations?, var fail: Boolean = false) : AutomationsClient {
        override suspend fun automations(appId: String): AppAutomations = if (fail) throw IOException("403") else value!!
        override suspend fun setRule(ruleId: String, enabled: Boolean, appId: String) = Unit
    }

    @Test
    fun picksTheMostFailingRuleAndSkipsStoppedOffAndHandled() {
        val a = rule("a", 3)
        val b = rule("b", 5)
        val c = rule("c", 5)
        assertEquals("回数が多い方・同じなら一覧の順", "b", AutomationFixOffer.pick(apps(a, b, c), emptySet())?.id)
        assertNull("3 回未満は出さない", AutomationFixOffer.pick(apps(rule("x", 2)), emptySet()))
        assertNull("運営がアプリごと止めている", AutomationFixOffer.pick(apps(b, stopped = AutomationStop()), emptySet()))
        assertEquals("運営が止めたルール・作成者がオフのルールは飛ばす", "a",
            AutomationFixOffer.pick(apps(a, rule("s", 9, stopped = AutomationStop()), rule("o", 9, enabled = false)), emptySet())?.id)
        assertEquals("enabled を返さない古いサーバーは出す", "n", AutomationFixOffer.pick(apps(rule("n", 4, enabled = null)), emptySet())?.id)
        val handled = setOf(AutomationFixOffer.key("app1", b))
        assertEquals("閉じた連続失敗は飛ばす", "c", AutomationFixOffer.pick(apps(a, b, c), handled)?.id)
    }

    @Test
    fun tappingFixFillsTheRequestAndDoesNotSend() = runTest {
        val kv = JsonKeyValueStore(null)
        val model = AutomationFixOfferModel("app1", FakeClient(apps(rule("r1", 4, description = "毎朝まとめを送る"))), AutomationFixOfferStore(kv))
        model.load()
        assertEquals("r1", model.rule?.id)
        var input = ""
        // 行の「直す」と同じ: fix() の文を入力欄に入れる
        model.fix()?.let { input = it }
        assertEquals("自動の動き「毎朝まとめを送る」が続けてうまくいっていません。うまく動くように直して", input)
        assertNull("押したら行は消える", model.rule)
        assertNull("2 回目は何も返さない", model.fix())
    }

    @Test
    fun dismissedOrFixedFailureIsNotOfferedAgainButANewStreakIs() = runTest {
        val kv = JsonKeyValueStore(null)
        val client = FakeClient(apps(rule("r1", 4)))
        val first = AutomationFixOfferModel("app1", client, AutomationFixOfferStore(kv))
        first.load()
        first.dismiss()
        val again = AutomationFixOfferModel("app1", client, AutomationFixOfferStore(kv))
        again.load()
        assertNull("同じ連続失敗では出さない", again.rule)
        // 一度成功して lastOkAt が進み、また続けて失敗した
        client.value = apps(rule("r1", 3, lastOkAt = "2026-11-12T00:00:00Z"))
        again.load()
        assertEquals("r1", again.rule?.id)
        assertEquals("同じアプリでも別のアプリの印は別", "r1",
            AutomationFixOfferModel("app2", FakeClient(apps(rule("r1", 4)).copy(appId = "app2")), AutomationFixOfferStore(kv)).also { it.load() }.rule?.id)
    }

    @Test
    fun unreadableAutomationsShowNothing() = runTest {
        val model = AutomationFixOfferModel("app1", FakeClient(null, fail = true), AutomationFixOfferStore(JsonKeyValueStore(null)))
        model.load()
        assertNull(model.rule)
    }

    @Test
    fun handledMarksAreCapped() {
        val store = AutomationFixOfferStore(JsonKeyValueStore(null))
        repeat(AutomationFixOfferStore.LIMIT + 5) { store.markHandled("k$it") }
        val h = store.handled()
        assertEquals(AutomationFixOfferStore.LIMIT, h.size)
        assertTrue("古い印から捨てる", "k0" !in h && "k${AutomationFixOfferStore.LIMIT + 4}" in h)
    }

    @Test
    fun textsInBothLanguages() {
        val t = StringTable.fromClasspath()
        for (key in listOf("desktop_automation_fix_offer", "desktop_automation_fix_offer_fix", "automations_fix_request", "desktop_close")) {
            assertTrue("$key(ja)", t.hasString(Lang.JA, key))
            assertTrue("$key(en)", t.hasString(Lang.EN, key))
        }
        val r = rule("r", 3, description = "Daily digest")
        assertEquals("The automation “Daily digest” keeps failing. Fix it?", Strings.withLang(Lang.EN) { AutomationFixOffer.message(r) })
        assertEquals("自動の動き「Daily digest」が続けて失敗しています。直しますか?", Strings.withLang(Lang.JA) { AutomationFixOffer.message(r) })
    }
}
