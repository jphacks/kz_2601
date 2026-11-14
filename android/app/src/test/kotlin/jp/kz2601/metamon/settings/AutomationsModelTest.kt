package jp.kz2601.metamon.settings

import jp.kz2601.metamon.services.automations.AppAutomations
import jp.kz2601.metamon.services.automations.AutomationRule
import jp.kz2601.metamon.services.automations.AutomationsClient
import jp.kz2601.metamon.services.http.ApiError
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.async
import kotlinx.coroutines.test.runTest
import kotlinx.coroutines.yield
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/** 自動の動きのオン / オフ(楽観的・失敗で戻す・#905)。 */
class AutomationsModelTest {
    private class FakeClient(var rules: List<AutomationRule>) : AutomationsClient {
        var listError: Exception? = null
        var setError: Exception? = null
        var gate: CompletableDeferred<Unit>? = null
        val calls = mutableListOf<Pair<String, Boolean>>()

        override suspend fun automations(appId: String): AppAutomations {
            listError?.let { throw it }
            return AppAutomations(appId = appId, rules = rules)
        }

        override suspend fun setRule(ruleId: String, enabled: Boolean, appId: String) {
            calls += ruleId to enabled
            gate?.await()
            setError?.let { throw it }
        }
    }

    private fun enabledOf(m: AutomationsModel, id: String): Boolean? =
        (m.phase.value as AutomationsModel.Phase.Loaded).automations.rules.first { it.id == id }.enabled

    private val rules = listOf(AutomationRule(id = "r1", enabled = true), AutomationRule(id = "old"))

    @Test
    fun loadsAndMaps403ToNotCreator() = runTest {
        val c = FakeClient(rules)
        val m = AutomationsModel("a", c)
        assertEquals(AutomationsModel.Phase.Loading, m.phase.value)
        m.load()
        assertTrue(m.phase.value is AutomationsModel.Phase.Loaded)

        c.listError = ApiError.Http(403, "")
        val other = AutomationsModel("a", c)
        other.load()
        assertEquals(AutomationsModel.Phase.NotCreator, other.phase.value)
    }

    @Test
    fun failedReloadKeepsPreviousList() = runTest {
        val c = FakeClient(rules)
        val m = AutomationsModel("a", c).apply { load() }
        c.listError = RuntimeException("offline")
        m.load()
        assertTrue(m.phase.value is AutomationsModel.Phase.Loaded)
        val fresh = AutomationsModel("a", c).apply { load() }
        assertEquals(AutomationsModel.Phase.Failed, fresh.phase.value)
    }

    @Test
    fun toggleIsOptimisticWhileSending() = runTest {
        val c = FakeClient(rules).apply { gate = CompletableDeferred() }
        val m = AutomationsModel("a", c).apply { load() }
        val job = async { m.setEnabled(false, "r1") }
        yield()
        assertEquals(false, enabledOf(m, "r1"))
        assertEquals(setOf("r1"), m.pending.value)
        // 送っている最中は二重に送らない
        m.setEnabled(true, "r1")
        c.gate!!.complete(Unit)
        job.await()
        assertEquals(false, enabledOf(m, "r1"))
        assertEquals(listOf("r1" to false), c.calls)
        assertEquals(emptySet<String>(), m.pending.value)
        assertNull(m.toggleError.value)
    }

    @Test
    fun failureRollsBackAndReports() = runTest {
        val c = FakeClient(rules).apply { setError = ApiError.Http(500, "") }
        val m = AutomationsModel("a", c).apply { load() }
        m.setEnabled(false, "r1")
        assertEquals(true, enabledOf(m, "r1"))
        assertEquals(AutomationsModel.ToggleError.FAILED, m.toggleError.value)
        m.clearToggleError()
        assertNull(m.toggleError.value)
    }

    @Test
    fun conflictRollsBackAndReloads() = runTest {
        val c = FakeClient(rules).apply { setError = ApiError.Http(412, "") }
        val m = AutomationsModel("a", c).apply { load() }
        c.rules = listOf(AutomationRule(id = "r1", enabled = false))
        m.setEnabled(false, "r1")
        assertEquals(AutomationsModel.ToggleError.CONFLICT, m.toggleError.value)
        // 取り直した最新(他で既にオフ)を見せる
        assertEquals(false, enabledOf(m, "r1"))
    }

    @Test
    fun ruleWithoutEnabledIsNotToggled() = runTest {
        val c = FakeClient(rules)
        val m = AutomationsModel("a", c).apply { load() }
        m.setEnabled(false, "old")
        m.setEnabled(true, "r1") // もうその状態
        assertEquals(emptyList<Pair<String, Boolean>>(), c.calls)
    }
}
