package jp.kz2601.metamon.runtime

import jp.kz2601.metamon.R
import jp.kz2601.metamon.dsl.Action
import jp.kz2601.metamon.dsl.ActionType
import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.decode
import jp.kz2601.metamon.dsl.records.JSONValue
import jp.kz2601.metamon.renderer.ActionContext
import jp.kz2601.metamon.renderer.navigation.Route
import jp.kz2601.metamon.runtime.RuntimeFixture.runtime
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/** 確認ダイアログ(#895): 主ボタンで先に進み、キャンセルでは何もしない。削除は「元に戻す」で戻せる。 */
class ConfirmFlowTest {
    private val dsl = AppDSL.decode(
        """
        {"schemaVersion":1,"app":{"name":"メモ","voiceEntry":"floating"},"initialScreen":"list",
         "entities":[{"id":"memo","name":"メモ","fields":[{"id":"body","name":"本文","type":"text"}]}],
         "screens":[
           {"id":"list","title":"一覧","components":[{"type":"list","entity":"memo"}]},
           {"id":"done","title":"完了","components":[]}
         ]}
        """.trimIndent(),
    )
    private val listRoute = Route("list")

    @Test
    fun confirmedNavigateMovesOnlyAfterAccept() = runTest {
        val rt = runtime(dsl)
        rt.run(Action(type = ActionType.NAVIGATE, screen = "done", confirmMessage = "移りますか?"), ActionContext(listRoute))
        val confirm = rt.state.value.confirm
        assertNotNull(confirm)
        assertEquals(R.string.runtime_confirm_open, confirm!!.primaryLabel)
        assertFalse(confirm.isDestructive)
        assertTrue("確認の前は進まない", rt.navigation.value.path(Route.MAIN).isEmpty())

        rt.confirmAccepted()
        assertNull("押したら閉じる", rt.state.value.confirm)
        assertEquals(listOf("done"), rt.navigation.value.path(Route.MAIN).map { it.screen })
    }

    @Test
    fun cancelDoesNothing() = runTest {
        val rt = runtime(dsl)
        val id = rt.createRecord("memo", mapOf("body" to JSONValue.Str("牛乳")))
        assertNotNull(id)
        rt.run(Action(type = ActionType.DELETE_RECORD, entity = "memo", confirmMessage = "消しますか?"), ActionContext(listRoute, recordId = id))
        assertNotNull(rt.state.value.confirm)

        rt.confirmDismissed()
        assertNull(rt.state.value.confirm)
        assertEquals(listOf(id), rt.records("memo").map { it.id })
        assertNull("消していないので元に戻すも出さない", rt.state.value.recordUndo)
        // 閉じた後に主ボタンの処理が残っていない
        rt.confirmAccepted()
        assertEquals(1, rt.records("memo").size)
    }

    @Test
    fun deletedRecordUndoClosesBannerAndRestores() = runTest {
        val rt = runtime(dsl)
        val id = rt.createRecord("memo", mapOf("body" to JSONValue.Str("牛乳")))!!
        rt.run(Action(type = ActionType.DELETE_RECORD, entity = "memo"), ActionContext(listRoute, recordId = id))
        assertTrue(rt.records("memo").isEmpty())
        assertNotNull(rt.state.value.recordUndo)

        rt.undoRecordDelete()
        assertNull("押したらバナーを閉じる", rt.state.value.recordUndo)
        assertEquals(listOf(id), rt.records("memo").map { it.id })
    }

    @Test
    fun dismissedRecordUndoKeepsDeletion() = runTest {
        val rt = runtime(dsl)
        val id = rt.createRecord("memo", mapOf("body" to JSONValue.Str("牛乳")))!!
        rt.run(Action(type = ActionType.DELETE_RECORD, entity = "memo"), ActionContext(listRoute, recordId = id))
        rt.dismissRecordUndo()
        assertNull(rt.state.value.recordUndo)
        rt.undoRecordDelete()
        assertTrue("閉じた後は戻さない", rt.records("memo").isEmpty())
    }
}
