package jp.kz2601.metamon.runtime

import jp.kz2601.metamon.dsl.Action
import jp.kz2601.metamon.dsl.ActionType
import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.decode
import jp.kz2601.metamon.dsl.expr.DSLClock
import jp.kz2601.metamon.dsl.records.FieldCoding
import jp.kz2601.metamon.dsl.records.JSONValue
import jp.kz2601.metamon.renderer.ActionContext
import jp.kz2601.metamon.renderer.form.FormDraft
import jp.kz2601.metamon.renderer.form.ScreenForm
import jp.kz2601.metamon.renderer.form.setBool
import jp.kz2601.metamon.renderer.form.setDate
import jp.kz2601.metamon.renderer.form.setDouble
import jp.kz2601.metamon.renderer.form.setNumberText
import jp.kz2601.metamon.renderer.form.setText
import jp.kz2601.metamon.renderer.form.setTime
import jp.kz2601.metamon.renderer.navigation.Route
import jp.kz2601.metamon.runtime.RuntimeFixture.runtime
import kotlinx.coroutines.test.TestScope
import kotlinx.coroutines.test.runTest
import kotlinx.serialization.json.JsonPrimitive
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test
import java.time.Instant
import java.time.ZoneId

/**
 * 画面の下書き → 入力部品の書き込み → 保存 までの流れ(#858)。#998 では描画器が下書きを作らず「保存するフォームがありません」になっていた。
 * 描画器と同じ [ScreenForm.makeDraft] で下書きを作り、入力部品と同じ書き込み口で値を入れて保存する。
 */
class FormInputSaveTest {
    private val appId = "test-inputs"

    private val dsl = AppDSL.decode(
        """
        {"schemaVersion":1,"app":{"name":"家計簿","voiceEntry":"none"},"initialScreen":"form",
         "entities":[{"id":"expense","name":"支出","titleField":"title","fields":[
           {"id":"title","name":"内容","type":"string","required":true},
           {"id":"amount","name":"金額","type":"currency","required":true},
           {"id":"date","name":"日付","type":"date"},
           {"id":"time","name":"時刻","type":"string"},
           {"id":"paid","name":"支払済み","type":"boolean"},
           {"id":"score","name":"満足度","type":"integer"}]}],
         "screens":[
           {"id":"form","title":"支出","entity":"expense","components":[
             {"type":"textField","field":"title"},{"type":"numberField","field":"amount"},{"type":"datePicker","field":"date"},
             {"type":"timePicker","field":"time"},{"type":"toggle","field":"paid"},{"type":"rating","field":"score"},
             {"type":"button","label":"保存","action":{"type":"saveRecord","entity":"expense"}}]},
           {"id":"home","title":"ホーム","components":[{"type":"text","text":"x"}]}]}
        """.trimIndent(),
    )

    private val route = Route("form", stack = Route.SHEET)
    private val savedZone = FieldCoding.zone

    @Before
    fun setUp() {
        DSLClock.fixed = Instant.parse("2026-11-12T03:00:00Z")
        FieldCoding.zone = ZoneId.of("Asia/Tokyo")
    }

    @After
    fun tearDown() {
        DSLClock.fixed = null
        FieldCoding.zone = savedZone
    }

    private fun TestScope.rt() = runtime(dsl, appId = appId)

    private fun AppRuntime.screenDraft(recordId: String? = null): FormDraft {
        val entity = entityModel("expense")!!
        return ScreenForm.makeDraft(entity, ScreenForm.formRecord(entity, recordId, records("expense")), this)
    }

    private suspend fun AppRuntime.save(form: FormDraft?) =
        run(Action(type = ActionType.SAVE_RECORD, entity = "expense"), ActionContext(route, form = form))

    @Test
    fun inputsAreSavedInTheSameShapeAsIos() = runTest {
        val rt = rt()
        val form = rt.screenDraft()
        form.setText("title", "ランチ")
        form.setNumberText("amount", "1,200")
        form.setDate("date", Instant.parse("2026-11-10T01:00:00Z"))
        form.setTime("time", 12, 30)
        form.setBool("paid", true)
        form.setDouble("score", 4.0)

        rt.save(form)

        val saved = rt.recordSync.cached(appId, "expense").single()
        assertEquals(JsonPrimitive("ランチ"), saved.data["title"])
        assertEquals(1200.0, (saved.data["amount"] as JsonPrimitive).content.toDouble(), 0.0)
        assertEquals(JsonPrimitive("2026-11-10"), saved.data["date"])
        assertEquals(JsonPrimitive("12:30"), saved.data["time"])
        assertEquals(JsonPrimitive(true), saved.data["paid"])
        assertEquals(4.0, (saved.data["score"] as JsonPrimitive).content.toDouble(), 0.0)
        assertEquals(1, form.saveCount)
        assertEquals("描画器が読む記録にもすぐ出る", 1, rt.recordsFlow.value["expense"]?.size)
    }

    @Test
    fun missingRequiredIsNotSaved() = runTest {
        val rt = rt()
        val form = rt.screenDraft()
        form.setText("title", "ランチ")
        form.setNumberText("amount", "abc")

        rt.save(form)

        assertEquals(setOf("amount"), form.flaggedFields)
        assertTrue(rt.recordSync.cached(appId, "expense").isEmpty())
    }

    @Test
    fun editOpensTheSavedRecordAndUpdatesIt() = runTest {
        val rt = rt()
        val first = rt.screenDraft()
        first.setText("title", "ランチ")
        first.setNumberText("amount", "800")
        rt.save(first)
        val id = rt.records("expense").single().id

        val edit = rt.screenDraft(recordId = id)
        assertEquals(id, edit.recordId)
        assertEquals(JSONValue.Num(800.0), edit.values["amount"])
        edit.setNumberText("amount", "900")
        rt.save(edit)

        val saved = rt.recordSync.cached(appId, "expense").single()
        assertEquals(900.0, (saved.data["amount"] as JsonPrimitive).content.toDouble(), 0.0)
    }

    @Test
    fun screenWithoutEntityHasNoForm() = runTest {
        val rt = rt()
        assertNotNull(rt.entityModel("expense"))
        rt.save(null)
        assertEquals(RuntimeCopy.NO_FORM, rt.toast)
    }
}
