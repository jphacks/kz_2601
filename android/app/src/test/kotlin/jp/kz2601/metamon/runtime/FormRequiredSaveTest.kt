package jp.kz2601.metamon.runtime

import jp.kz2601.metamon.dsl.Action
import jp.kz2601.metamon.dsl.ActionType
import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.decode
import jp.kz2601.metamon.dsl.records.JSONValue
import jp.kz2601.metamon.renderer.ActionContext
import jp.kz2601.metamon.renderer.form.FormDraft
import jp.kz2601.metamon.renderer.form.FormSaveResult
import jp.kz2601.metamon.renderer.navigation.Route
import jp.kz2601.metamon.runtime.RuntimeFixture.runtime
import kotlinx.coroutines.test.TestScope
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/** iOS の FormRequiredSaveTests(M10): 必須が空のまま保存すると、トーストに加えて欄に印が付き、記録は作らない。 */
class FormRequiredSaveTest {
    private val appId = "test-required"

    private val dsl = AppDSL.decode(
        """
        {"schemaVersion":1,"app":{"name":"メモ","voiceEntry":"floating"},"initialScreen":"form",
         "entities":[{"id":"memo","name":"メモ","fields":[{"id":"body","name":"本文","type":"text","required":true}]}],
         "screens":[{"id":"form","title":"メモ","entity":"memo","components":[{"type":"textField","field":"body"}]}]}
        """.trimIndent(),
    )

    private val ctxRoute = Route("form", stack = Route.SHEET)

    private fun TestScope.rt() = runtime(dsl, appId = appId)

    @Test
    fun emptyRequiredIsFlaggedAndNotSaved() = runTest {
        val rt = rt()
        val draft = FormDraft(rt.entityModel("memo")!!)

        rt.run(Action(type = ActionType.SAVE_RECORD, entity = "memo"), ActionContext(ctxRoute, form = draft))

        assertEquals(setOf("body"), draft.flaggedFields)
        assertEquals(1, draft.flagCount)
        assertEquals("本文 を入力してください", rt.toast)
        assertTrue(rt.recordSync.cached(appId, "memo").isEmpty())
    }

    @Test
    fun filledFormSavesWithoutRejection() = runTest {
        val rt = rt()
        val draft = FormDraft(rt.entityModel("memo")!!)
        draft.flagMissingRequired()
        draft.values["body"] = JSONValue.Str("牛乳")

        rt.run(Action(type = ActionType.SAVE_RECORD, entity = "memo"), ActionContext(ctxRoute, form = draft))

        assertEquals("保存できた時は数えない", 1, draft.flagCount)
        assertEquals("保存バーが「保存しました」を出す", 1, draft.saveCount)
        assertTrue(draft.flaggedFields.isEmpty())
        assertEquals(1, rt.recordSync.cached(appId, "memo").size)
        assertEquals("画面の一覧にもすぐ出る", 1, rt.records("memo").size)
    }

    private suspend fun save(rt: AppRuntime, draft: FormDraft): FormSaveResult? {
        val before = draft.saveCount
        rt.run(Action(type = ActionType.SAVE_RECORD, entity = "memo"), ActionContext(ctxRoute, form = draft))
        return FormDraft.saveFeedback(before, draft)
    }

    @Test
    fun rejectedSaveShowsNoSavedFeedback() = runTest {
        val rt = rt()
        assertNull(save(rt, FormDraft(rt.entityModel("memo")!!)))
    }

    @Test
    fun unchangedEditIsNotWrittenNorReportedSaved() = runTest {
        val rt = rt()
        val entity = rt.entityModel("memo")!!
        val draft = FormDraft(entity)
        draft.values["body"] = JSONValue.Str("牛乳")
        val first = save(rt, draft)
        assertEquals(if (rt.recordSync.isOnline) FormSaveResult.SAVED else FormSaveResult.QUEUED_OFFLINE, first)

        val editing = FormDraft(entity, rt.records("memo").first())
        assertNull("何も変えずに保存したら「保存しました」を出さない", save(rt, editing))
        assertEquals(1, rt.recordSync.cached(appId, "memo").size)
    }
}
