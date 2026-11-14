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
import jp.kz2601.metamon.renderer.screen.SaveBarFeedback
import jp.kz2601.metamon.renderer.screen.SaveBarPhase
import jp.kz2601.metamon.runtime.RuntimeFixture.runtime
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/** #1374: 保存バーが本物のランタイムの保存を待って結果を出す(テストのランタイムは圏外 = 送信待ち)。 */
class SaveBarRuntimeTest {
    private val appId = "test-save-bar"

    private val dsl = AppDSL.decode(
        """
        {"schemaVersion":1,"app":{"name":"メモ","voiceEntry":"none"},"initialScreen":"form",
         "entities":[{"id":"memo","name":"メモ","fields":[{"id":"body","name":"本文","type":"text","required":true}]}],
         "screens":[{"id":"form","title":"メモ","entity":"memo","toolbar":[{"label":"保存","action":{"type":"saveRecord"}}],"components":[]}]}
        """.trimIndent(),
    )

    private val route = Route("form", stack = Route.SHEET)
    private val save = Action(type = ActionType.SAVE_RECORD)

    @Test
    fun offlineSaveIsQueuedAndSaysSo() = runTest {
        val rt = runtime(dsl, appId = appId)
        val form = FormDraft(rt.entityModel("memo")!!).apply { values["body"] = JSONValue.Str("牛乳") }
        var shown: SaveBarPhase? = null
        lateinit var bar: SaveBarFeedback
        bar = SaveBarFeedback(hold = { shown = bar.phase })

        val result = bar.press(form) { rt.runAction(save, ActionContext(route, form)) }

        assertEquals(FormSaveResult.QUEUED_OFFLINE, result)
        assertEquals("「接続が戻ったら自動で送ります」", SaveBarPhase.Done(FormSaveResult.QUEUED_OFFLINE), shown)
        assertEquals(SaveBarPhase.Idle, bar.phase)
        assertTrue("送信待ちに積んだ", rt.recordSync.pendingCount.value > 0)
    }

    @Test
    fun missingRequiredShowsNothing() = runTest {
        val rt = runtime(dsl, appId = appId)
        val form = FormDraft(rt.entityModel("memo")!!)
        val bar = SaveBarFeedback(hold = { error("出さない") })

        assertNull(bar.press(form) { rt.runAction(save, ActionContext(route, form)) })
        assertEquals(SaveBarPhase.Idle, bar.phase)
        assertEquals(RuntimeCopy.missingRequired(listOf("本文")), rt.toast)
    }

    @Test
    fun unchangedEditShowsNothing() = runTest {
        val rt = runtime(dsl, appId = appId)
        val entity = rt.entityModel("memo")!!
        val first = FormDraft(entity).apply { values["body"] = JSONValue.Str("牛乳") }
        rt.runAction(save, ActionContext(route, first))

        val editing = FormDraft(entity, rt.records("memo").first())
        val bar = SaveBarFeedback(hold = { error("出さない") })
        assertNull(bar.press(editing) { rt.runAction(save, ActionContext(route, editing)) })
        assertEquals(SaveBarPhase.Idle, bar.phase)
    }
}
