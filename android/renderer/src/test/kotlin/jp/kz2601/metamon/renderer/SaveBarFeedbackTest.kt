package jp.kz2601.metamon.renderer

import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.decode
import jp.kz2601.metamon.dsl.records.JSONValue
import jp.kz2601.metamon.dsl.records.entityModels
import jp.kz2601.metamon.renderer.form.FormDraft
import jp.kz2601.metamon.renderer.form.FormSaveResult
import jp.kz2601.metamon.renderer.screen.SaveBarFeedback
import jp.kz2601.metamon.renderer.screen.SaveBarPhase
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.async
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.yield
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

/** #1374: 保存バーを押した後の表示(iOS の `FormSaveBar.run`)。 */
class SaveBarFeedbackTest {
    private val entity = AppDSL.decode(
        """
        {"schemaVersion":1,"app":{"name":"メモ","voiceEntry":"none"},"initialScreen":"form",
         "entities":[{"id":"memo","name":"メモ","fields":[{"id":"body","name":"本文","type":"text","required":true}]}],
         "screens":[{"id":"form","title":"メモ","entity":"memo","components":[]}]}
        """.trimIndent(),
    ).entityModels().first()

    private fun filled() = FormDraft(entity).apply { values["body"] = JSONValue.Str("牛乳") }

    @Test
    fun savingThenSavedThenBackToIdle() = runBlocking {
        val form = filled()
        val phases = mutableListOf<SaveBarPhase>()
        lateinit var bar: SaveBarFeedback
        bar = SaveBarFeedback(hold = { phases += bar.phase })
        val result = bar.press(form) {
            phases += bar.phase
            form.markSaved(queuedOffline = false)
        }
        assertEquals(FormSaveResult.SAVED, result)
        assertEquals(listOf(SaveBarPhase.Saving, SaveBarPhase.Done(FormSaveResult.SAVED)), phases)
        assertEquals(SaveBarPhase.Idle, bar.phase)
    }

    @Test
    fun offlineSaveSaysItWillSendLater() = runBlocking {
        val form = filled()
        var shown: SaveBarPhase? = null
        lateinit var bar: SaveBarFeedback
        bar = SaveBarFeedback(hold = { shown = bar.phase })
        assertEquals(FormSaveResult.QUEUED_OFFLINE, bar.press(form) { form.markSaved(queuedOffline = true) })
        assertEquals(SaveBarPhase.Done(FormSaveResult.QUEUED_OFFLINE), shown)
    }

    @Test
    fun noWriteShowsNothing() = runBlocking {
        val form = FormDraft(entity) // 必須が空: ホストは書かない
        var held = false
        val bar = SaveBarFeedback(hold = { held = true })
        assertNull(bar.press(form) { form.flagMissingRequired() })
        assertEquals(SaveBarPhase.Idle, bar.phase)
        assertEquals("「保存しました」を出していない", false, held)
    }

    @Test
    fun secondPressWhileSavingIsIgnored() = runBlocking {
        val form = filled()
        val gate = CompletableDeferred<Unit>()
        var writes = 0
        val bar = SaveBarFeedback(hold = {})
        val first = async { bar.press(form) { gate.await(); writes++; form.markSaved(queuedOffline = false) } }
        yield()
        assertEquals(SaveBarPhase.Saving, bar.phase)
        assertNull(bar.press(form) { writes++ })
        gate.complete(Unit)
        assertEquals(FormSaveResult.SAVED, first.await())
        assertEquals(1, writes)
    }

    @Test
    fun serverRefusalWithdrawsSaved() = runBlocking {
        val form = filled()
        val gate = CompletableDeferred<Unit>()
        val bar = SaveBarFeedback(hold = { gate.await() })
        val pressing = async { bar.press(form) { form.markSaved(queuedOffline = false) } }
        yield()
        assertEquals(SaveBarPhase.Done(FormSaveResult.SAVED), bar.phase)

        form.markSaveRefused() // 送った後にサーバーが断った
        bar.onSaveResultChanged(form.lastSaveResult)
        assertEquals("表示中でも消す", SaveBarPhase.Idle, bar.phase)
        gate.complete(Unit)
        pressing.await()
        assertEquals(SaveBarPhase.Idle, bar.phase)
    }

    @Test
    fun refusalDuringHoldDoesNotClearTheNextRound() = runBlocking {
        val form = filled()
        val firstHold = CompletableDeferred<Unit>()
        val secondHold = CompletableDeferred<Unit>()
        var round = 0
        val bar = SaveBarFeedback(hold = { if (++round == 1) firstHold.await() else secondHold.await() })
        val first = async { bar.press(form) { form.markSaved(queuedOffline = false) } }
        yield()
        form.markSaveRefused()
        bar.onSaveResultChanged(form.lastSaveResult)
        form.values["body"] = JSONValue.Str("卵")
        val second = async { bar.press(form) { form.markSaved(queuedOffline = false) } }
        yield()
        assertEquals(SaveBarPhase.Done(FormSaveResult.SAVED), bar.phase)
        firstHold.complete(Unit)
        first.await()
        assertEquals("前の回の時間切れでは消えない", SaveBarPhase.Done(FormSaveResult.SAVED), bar.phase)
        secondHold.complete(Unit)
        second.await()
        assertEquals(SaveBarPhase.Idle, bar.phase)
    }
}
