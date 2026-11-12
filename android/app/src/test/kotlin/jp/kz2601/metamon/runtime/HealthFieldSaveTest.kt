package jp.kz2601.metamon.runtime

import jp.kz2601.metamon.dsl.Action
import jp.kz2601.metamon.dsl.ActionType
import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.decode
import jp.kz2601.metamon.renderer.ActionContext
import jp.kz2601.metamon.renderer.components.health.HealthAvailability
import jp.kz2601.metamon.renderer.components.health.HealthDailyValue
import jp.kz2601.metamon.renderer.components.health.HealthDataException
import jp.kz2601.metamon.renderer.components.health.HealthFailure
import jp.kz2601.metamon.renderer.components.health.HealthSource
import jp.kz2601.metamon.renderer.form.FormDraft
import jp.kz2601.metamon.renderer.form.ScreenForm
import jp.kz2601.metamon.renderer.form.setNumberText
import jp.kz2601.metamon.renderer.navigation.Route
import jp.kz2601.metamon.runtime.RuntimeFixture.runtime
import kotlinx.coroutines.test.TestScope
import kotlinx.coroutines.test.UnconfinedTestDispatcher
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * `Field.health`(#870・iOS の AppRuntime+Health): 新しく保存した体重だけをヘルスコネクトにも書く。
 * 書き込みが失敗しても記録は残り、理由だけトーストに出る。編集の保存では書かない。
 */
class HealthFieldSaveTest {
    // ヘルスへの書き込みは保存とは別の coroutine(失敗しても保存は続く)なので、その場で走る dispatcher で回す
    private val dsl = AppDSL.decode(
        """
        {"schemaVersion":1,"app":{"name":"からだ記録","voiceEntry":"none"},"initialScreen":"form",
         "entities":[{"id":"body","name":"体重","titleField":"weight","fields":[
           {"id":"weight","name":"体重","type":"number","required":true,"unit":"kg","health":"weight"},
           {"id":"fat","name":"体脂肪","type":"number"}]}],
         "screens":[{"id":"form","title":"体重を記録","entity":"body","components":[
           {"type":"numberField","field":"weight"},{"type":"numberField","field":"fat"},
           {"type":"button","label":"保存","action":{"type":"saveRecord","entity":"body"}}]}]}
        """.trimIndent(),
    )

    private val route = Route("form", stack = Route.SHEET)

    private class FakeHealth(private val failing: Boolean = false) : HealthSource {
        val written = mutableListOf<Double>()
        override fun availability() = HealthAvailability.AVAILABLE
        override fun readPermissions(weight: Boolean): Set<String> = emptySet()
        override fun writePermissions(): Set<String> = emptySet()
        override suspend fun granted(): Set<String> = emptySet()
        override suspend fun steps(days: Int): List<HealthDailyValue> = emptyList()
        override suspend fun weights(days: Int): List<HealthDailyValue> = emptyList()
        override suspend fun saveWeight(kg: Double) {
            if (failing) throw HealthDataException(HealthFailure.DENIED)
            written += kg
        }
    }

    private class HealthHost(override val health: HealthSource?) : RuntimeHost

    private fun TestScope.rt(host: RuntimeHost) = runtime(dsl, appId = "test-health", host = host)

    private fun AppRuntime.draft(recordId: String? = null): FormDraft {
        val entity = entityModel("body")!!
        return ScreenForm.makeDraft(entity, ScreenForm.formRecord(entity, recordId, records("body")), this)
    }

    private suspend fun save(rt: AppRuntime, form: FormDraft) =
        rt.run(Action(type = ActionType.SAVE_RECORD, entity = "body"), ActionContext(route, form = form))

    @Test
    fun newWeightIsAlsoWrittenToHealth() = runTest(UnconfinedTestDispatcher()) {
        val health = FakeHealth()
        val rt = rt(HealthHost(health))
        val form = rt.draft()
        form.setNumberText("weight", "61.8")
        form.setNumberText("fat", "18.2")

        save(rt, form)

        assertEquals("health: \"weight\" の項目だけ書く", listOf(61.8), health.written)
        assertEquals(1, rt.records("body").size)
    }

    @Test
    fun editingAnExistingRecordDoesNotWriteAgain() = runTest(UnconfinedTestDispatcher()) {
        val health = FakeHealth()
        val rt = rt(HealthHost(health))
        val first = rt.draft()
        first.setNumberText("weight", "61.8")
        save(rt, first)
        val id = rt.records("body").single().id

        val edit = rt.draft(recordId = id)
        edit.setNumberText("weight", "62.4")
        save(rt, edit)

        assertEquals("同じ計測が二重に入らない", listOf(61.8), health.written)
    }

    @Test
    fun aFailedHealthWriteStillKeepsTheRecord() = runTest(UnconfinedTestDispatcher()) {
        val rt = rt(HealthHost(FakeHealth(failing = true)))
        val form = rt.draft()
        form.setNumberText("weight", "61.8")

        save(rt, form)

        assertEquals(1, rt.records("body").size)
        assertEquals(RuntimeCopy.HEALTH_SAVE_FAILED, rt.state.value.toast)
    }

    @Test
    fun withoutAHealthSourceNothingIsWritten() = runTest(UnconfinedTestDispatcher()) {
        val rt = rt(HealthHost(null))
        val form = rt.draft()
        form.setNumberText("weight", "61.8")

        save(rt, form)

        assertEquals(1, rt.records("body").size)
        assertTrue("ヘルスの口が無い時はトーストも出さない", rt.state.value.toast == null)
    }
}
