package jp.kz2601.metamon.renderer

import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.decode
import jp.kz2601.metamon.dsl.records.JSONValue
import jp.kz2601.metamon.dsl.records.Record
import jp.kz2601.metamon.dsl.records.entityModels
import jp.kz2601.metamon.renderer.form.DiscardGuard
import jp.kz2601.metamon.renderer.form.FormDraft
import jp.kz2601.metamon.renderer.form.RequiredFieldFocus
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/** #1111: 保存していない入力の破棄の確認と、保存で弾かれた時に移る最初の空の必須欄(iOS の M10)。 */
class FormDiscardGuardTest {
    private val dsl = AppDSL.decode(
        """
        {"schemaVersion":1,"app":{"name":"本","voiceEntry":"none"},"initialScreen":"form",
         "entities":[{"id":"book","name":"本","fields":[
           {"id":"title","name":"題名","type":"string","required":true},
           {"id":"author","name":"著者","type":"string","required":true},
           {"id":"memo","name":"メモ","type":"text"},
           {"id":"secret","name":"秘密","type":"string","required":true,"hidden":true}]}],
         "screens":[{"id":"form","title":"本","entity":"book","components":[
           {"type":"textField","field":"memo","area":"A3:F3"},
           {"type":"vstack","children":[{"type":"textField","field":"secret"}]},
           {"type":"textField","field":"author","area":"A2:F2"},
           {"type":"textField","field":"title","area":"A1:F1"}]}]}
        """.trimIndent(),
    )
    private val entity = dsl.entityModels().first()
    private val screen = dsl.screens.first()

    // ---- 破棄の確認 ----

    @Test
    fun untouchedFormLeavesWithoutConfirm() {
        val guard = DiscardGuard()
        guard.track("main", FormDraft(entity))
        var left = false
        guard.request("main") { left = true }
        assertTrue(left)
        assertNull(guard.pending)
    }

    @Test
    fun editedFormAsksBeforeLeaving() {
        val guard = DiscardGuard()
        val form = FormDraft(entity).apply { values["title"] = JSONValue.Str("坊っちゃん") }
        guard.track("sheet", form)
        var left = 0
        guard.request("sheet") { left++ }
        assertEquals(0, left)
        assertNotNull(guard.pending)
        guard.request("sheet") { left += 10 } // 確認中の 2 回目(払い直しなど)は受けない
        guard.discard()
        assertEquals(1, left)
        assertNull(guard.pending)
    }

    @Test
    fun keepEditingDropsTheAction() {
        val guard = DiscardGuard()
        guard.track("main", FormDraft(entity).apply { values["memo"] = JSONValue.Str("a") })
        var left = false
        guard.request("main") { left = true }
        guard.keepEditing()
        guard.discard()
        assertFalse(left)
    }

    @Test
    fun onlyTheStackBeingLeftCounts() {
        val guard = DiscardGuard()
        guard.track("sheet", FormDraft(entity).apply { values["memo"] = JSONValue.Str("a") })
        assertTrue(guard.needsConfirm("sheet"))
        assertFalse(guard.needsConfirm("main"))
    }

    @Test
    fun savedOrPrefilledInputIsNotUnsaved() {
        val saved = FormDraft(entity).apply { values["title"] = JSONValue.Str("x") }
        saved.markSaved(queuedOffline = false)
        assertFalse(DiscardGuard.needsConfirm(saved))
        val prefilled = FormDraft(entity).apply { prefill(mapOf("title" to JSONValue.Str("親から"))) }
        assertFalse(DiscardGuard.needsConfirm(prefilled))
        assertFalse(DiscardGuard.needsConfirm(null))
    }

    @Test
    fun readOnlyRecordNeverAsks() {
        val form = FormDraft(entity, Record("r1", mapOf("title" to JSONValue.Str("a"))))
        form.isReadOnly = true
        form.values["title"] = JSONValue.Str("b")
        assertFalse(DiscardGuard.needsConfirm(form))
    }

    @Test
    fun untrackOnlyRemovesTheSameForm() {
        val guard = DiscardGuard()
        val old = FormDraft(entity)
        val new = FormDraft(entity).apply { values["memo"] = JSONValue.Str("a") }
        guard.track("main", new)
        guard.untrack("main", old) // 前の画面の後片付けが後から来ても、今のフォームは残す
        assertTrue(guard.needsConfirm("main"))
        guard.untrack("main", new)
        assertFalse(guard.needsConfirm("main"))
    }

    // ---- 最初の空の必須欄 ----

    @Test
    fun firstMissingFollowsScreenOrderNotDslOrder() {
        val form = FormDraft(entity)
        form.flagMissingRequired()
        assertEquals("title", RequiredFieldFocus.firstFlagged(screen, form, currentUid = null))
    }

    @Test
    fun filledFieldsAreSkipped() {
        val form = FormDraft(entity).apply { values["title"] = JSONValue.Str("坊っちゃん") }
        form.flagMissingRequired()
        assertEquals("author", RequiredFieldFocus.firstFlagged(screen, form, currentUid = null))
    }

    @Test
    fun stackedChildrenComeAfterPlacedFields() {
        val form = FormDraft(entity).apply {
            values["title"] = JSONValue.Str("a")
            values["author"] = JSONValue.Str("b")
        }
        form.flagMissingRequired()
        assertEquals("secret", RequiredFieldFocus.firstFlagged(screen, form, currentUid = null))
    }

    @Test
    fun hiddenFieldOfSomeoneElsesRecordIsSkipped() {
        val form = FormDraft(entity, Record("r1", emptyMap(), author = "other"))
        form.values["title"] = JSONValue.Str("a")
        form.values["author"] = JSONValue.Str("b")
        form.flagMissingRequired()
        assertNull(RequiredFieldFocus.firstFlagged(screen, form, currentUid = "me"))
    }

    @Test
    fun nothingFlaggedBeforeSave() {
        assertNull(RequiredFieldFocus.firstFlagged(screen, FormDraft(entity), currentUid = null))
    }
}
