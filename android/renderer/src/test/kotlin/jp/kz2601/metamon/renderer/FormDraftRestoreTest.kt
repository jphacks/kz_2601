package jp.kz2601.metamon.renderer

import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.decode
import jp.kz2601.metamon.dsl.records.Entity
import jp.kz2601.metamon.dsl.records.JSONValue
import jp.kz2601.metamon.dsl.records.Record
import jp.kz2601.metamon.dsl.records.entityModels
import jp.kz2601.metamon.renderer.form.DiscardGuard
import jp.kz2601.metamon.renderer.form.FormDraft
import jp.kz2601.metamon.renderer.form.FormDraftRestore
import jp.kz2601.metamon.renderer.form.FormDraftStorage
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/** #1185: プロセスが落とされた後にフォームの書きかけを戻す。 */
class FormDraftRestoreTest {
    private fun dsl(extraField: String = "") = AppDSL.decode(
        """
        {"schemaVersion":1,"app":{"name":"日記","voiceEntry":"none"},"initialScreen":"form",
         "entities":[{"id":"diary","name":"日記","fields":[
           {"id":"title","name":"題","type":"string"},
           {"id":"pin","name":"暗証","type":"string"},
           {"id":"photo","name":"写真","type":"photo"}$extraField]}],
         "screens":[{"id":"form","title":"日記","entity":"diary","components":[
           {"type":"textField","field":"title"},
           {"type":"vstack","children":[{"type":"secureField","field":"pin"}]},
           {"type":"photoField","field":"photo"}]}]}
        """.trimIndent(),
    )

    private val app = dsl()
    private val entity = app.entityModels().first()
    private val screen = app.screens.first()
    private val routeId = "sheet/form/"
    private val appId = "app-1"

    private fun dirtyForm(e: Entity = entity, record: Record? = null): FormDraft = FormDraft(e, record).apply {
        values["title"] = JSONValue.Str("書きかけ")
        values["pin"] = JSONValue.Str("1234")
    }

    private fun persisted(storage: FormDraftStorage, form: FormDraft = dirtyForm()): FormDraftStorage {
        val drafts = FormDraftRestore(storage)
        drafts.attach(routeId, form, FormDraftRestore.excludedFields(screen, form.entity))
        drafts.persist(appId)
        return storage
    }

    /** 次のプロセス: 画面の位置を戻せた後に作ったフォーム。 */
    private fun relaunch(storage: FormDraftStorage, e: Entity = entity, record: Record? = null, arm: String? = appId): Pair<FormDraft, Boolean> {
        val drafts = FormDraftRestore(storage)
        arm?.let(drafts::arm)
        val form = FormDraft(e, record)
        return form to drafts.restore(routeId, form)
    }

    @Test
    fun persistAndRestoreRoundTrip() {
        val storage = persisted(FormDraftStorage.InMemory())
        val (form, restored) = relaunch(storage)
        assertTrue(restored)
        assertEquals(JSONValue.Str("書きかけ"), form.values["title"])
        assertTrue("戻した入力は未保存のまま(離れる時に破棄の確認が出る)", form.hasUnsavedChanges)
    }

    @Test
    fun secureFieldIsNeverPersisted() {
        val storage = persisted(FormDraftStorage.InMemory()) as FormDraftStorage.InMemory
        assertFalse(storage.text!!.contains("1234"))
        val (form, _) = relaunch(storage)
        assertEquals("伏せ字の欄は初めの値", FormDraft(entity).values["pin"], form.values["pin"])
        assertEquals(setOf("pin"), FormDraftRestore.excludedFields(screen, entity))
    }

    @Test
    fun largeBinaryValuesAreSkippedButReferencesAreKept() {
        val big = dirtyForm().apply { values["photo"] = JSONValue.Str("data:image/jpeg;base64," + "A".repeat(10_000)) }
        val storage = persisted(FormDraftStorage.InMemory(), big) as FormDraftStorage.InMemory
        assertFalse(storage.text!!.contains("data:image"))
        assertTrue(storage.text!!.contains("書きかけ"))

        val ref = dirtyForm().apply { values["photo"] = JSONValue.Str("local-photo://abc.jpg") }
        val refStorage = persisted(FormDraftStorage.InMemory(), ref)
        assertEquals(JSONValue.Str("local-photo://abc.jpg"), relaunch(refStorage).first.values["photo"])
    }

    @Test
    fun untouchedFormIsNotPersisted() {
        val storage = persisted(FormDraftStorage.InMemory(), FormDraft(entity)) as FormDraftStorage.InMemory
        assertNull(storage.text)
    }

    @Test
    fun notRestoredWithoutArmingOrForAnotherApp() {
        val storage = persisted(FormDraftStorage.InMemory()) as FormDraftStorage.InMemory
        assertFalse("画面の位置を戻していない", relaunch(storage, arm = null).second)
        assertFalse("別のアプリ", relaunch(storage, arm = "app-2").second)
        assertNull("別のアプリに戻ったら捨てる", storage.text)
    }

    @Test
    fun anotherRecordDoesNotReceiveTheDraft() {
        val storage = persisted(FormDraftStorage.InMemory())
        val other = Record(id = "r9", data = emptyMap())
        assertFalse(relaunch(storage, record = other).second)
    }

    @Test
    fun changedDslFieldsDiscardTheDraft() {
        val storage = persisted(FormDraftStorage.InMemory()) as FormDraftStorage.InMemory
        val changed = dsl(""",{"id":"mood","name":"気分","type":"enum","options":["良","悪"]}""").entityModels().first()
        assertFalse(relaunch(storage, e = changed).second)
        assertNull(storage.text)
    }

    @Test
    fun saveOrDiscardForgetsTheDraft() {
        val storage = FormDraftStorage.InMemory()
        val drafts = FormDraftRestore(storage)
        val form = dirtyForm()
        drafts.attach(routeId, form, emptySet())
        drafts.persist(appId)
        assertNotNull(storage.text)
        drafts.forget(form)
        assertNull(storage.text)

        // 「破棄」: DiscardGuard が捨てたフォームを知らせる
        drafts.persist(appId)
        val guard = DiscardGuard(onDiscard = drafts::forget)
        guard.track("sheet", form)
        var left = false
        guard.request("sheet") { left = true }
        guard.discard()
        assertTrue(left)
        assertNull(storage.text)
    }

    @Test
    fun keepEditingDoesNotForget() {
        val storage = FormDraftStorage.InMemory()
        val drafts = FormDraftRestore(storage)
        val form = dirtyForm()
        drafts.attach(routeId, form, emptySet())
        drafts.persist(appId)
        val guard = DiscardGuard(onDiscard = drafts::forget)
        guard.track("sheet", form)
        guard.request("sheet") {}
        guard.keepEditing()
        assertNotNull(storage.text)
    }

    @Test
    fun wipeAndAppDeleteClear() {
        val storage = persisted(FormDraftStorage.InMemory()) as FormDraftStorage.InMemory
        FormDraftRestore(storage).forgetApp("app-2")
        assertNotNull("別のアプリの削除では消さない", storage.text)
        FormDraftRestore(storage).forgetApp(appId)
        assertNull(storage.text)

        persisted(storage)
        FormDraftRestore(storage).clear()
        assertNull(storage.text)
    }

    @Test
    fun brokenStorageIsIgnored() {
        val storage = FormDraftStorage.InMemory().apply { text = "{not json" }
        assertFalse(relaunch(storage).second)
    }
}
