package jp.kz2601.metamon.runtime

import jp.kz2601.metamon.dsl.Action
import jp.kz2601.metamon.dsl.ActionType
import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.Notification
import jp.kz2601.metamon.dsl.Presentation
import jp.kz2601.metamon.dsl.decode
import jp.kz2601.metamon.dsl.records.JSONValue
import jp.kz2601.metamon.renderer.ActionContext
import jp.kz2601.metamon.renderer.form.FormDraft
import jp.kz2601.metamon.renderer.navigation.Route
import jp.kz2601.metamon.runtime.RuntimeFixture.runtime
import jp.kz2601.metamon.services.local.SyncEvent
import kotlinx.coroutines.test.TestScope
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/** アクション 16 種の振り分け(iOS の AppRuntime.perform)。端末の機能が要るものはホストへ渡すだけ。 */
class ActionDispatchTest {
    private val appId = "test-dispatch"

    private val dsl = AppDSL.decode(
        """
        {"schemaVersion":1,"app":{"name":"メモ","voiceEntry":"floating"},"initialScreen":"list",
         "entities":[{"id":"memo","name":"メモ","fields":[{"id":"body","name":"本文","type":"text"}]}],
         "screens":[
           {"id":"list","title":"一覧","components":[{"type":"list","entity":"memo"}]},
           {"id":"form","title":"メモ","entity":"memo","components":[{"type":"textField","field":"body"}]},
           {"id":"done","title":"完了","components":[]}
         ]}
        """.trimIndent(),
    )

    private val listRoute = Route("list")
    private val sheetForm = Route("form", stack = Route.SHEET)

    private fun TestScope.rt(host: RuntimeHost = UnavailableHost) = runtime(dsl, appId = appId, host = host)

    private fun TestScope.openForm(rt: AppRuntime): FormDraft {
        rt.setNavigation(rt.navigation.value.copy(sheet = sheetForm))
        return FormDraft(rt.entityModel("memo")!!)
    }

    private suspend fun AppRuntime.saveMemo(draft: FormDraft, body: String, screen: String? = null) {
        draft.values["body"] = JSONValue.Str(body)
        run(Action(type = ActionType.SAVE_RECORD, entity = "memo", screen = screen), ActionContext(sheetForm, form = draft))
    }

    @Test
    fun unknownActionShowsToast() = runTest {
        val rt = rt()
        rt.run(Action(type = ActionType.UNKNOWN), ActionContext(listRoute))
        assertTrue(rt.toast!!.startsWith("未対応のアクション: "))
    }

    @Test
    fun navigateAndBack() = runTest {
        val rt = rt()
        rt.run(Action(type = ActionType.NAVIGATE, screen = "done"), ActionContext(listRoute))
        assertEquals(listOf("done"), rt.navigation.value.path(Route.MAIN).map { it.screen })
        rt.run(Action(type = ActionType.BACK), ActionContext(Route("done")))
        assertTrue(rt.navigation.value.path(Route.MAIN).isEmpty())
    }

    @Test
    fun saveClosesTheFormOrGoesToScreen() = runTest {
        val rt = rt()
        rt.saveMemo(openForm(rt), "牛乳")
        assertNull("保存の後は閉じる", rt.navigation.value.sheet)

        rt.saveMemo(openForm(rt), "卵", screen = "list")
        assertNull("下の画面(根)へ飛ぶならシートを閉じる", rt.navigation.value.sheet)

        rt.saveMemo(openForm(rt), "パン", screen = "done")
        assertEquals("下に無い画面はシートの中に積む", listOf("done"), rt.navigation.value.path(Route.SHEET).map { it.screen })
        assertEquals(3, rt.records("memo").size)
    }

    @Test
    fun saveWithoutFormShowsToast() = runTest {
        val rt = rt()
        rt.run(Action(type = ActionType.SAVE_RECORD, entity = "memo"), ActionContext(listRoute))
        assertEquals("保存するフォームがありません", rt.toast)
    }

    @Test
    fun deleteAsksFirstThenHidesWithUndo() = runTest {
        val rt = rt()
        rt.saveMemo(openForm(rt), "牛乳")
        val id = rt.records("memo").single().id

        rt.run(Action(type = ActionType.DELETE_RECORD, entity = "memo", confirmMessage = "消しますか?"), ActionContext(listRoute, recordId = id))
        val confirm = rt.state.value.confirm
        assertNotNull(confirm)
        assertEquals("削除", confirm!!.primaryLabel)
        assertTrue(confirm.isDestructive)
        assertEquals("確認の前は消さない", 1, rt.records("memo").size)

        rt.confirmAccepted()
        assertTrue(rt.records("memo").isEmpty())
        assertNotNull("元に戻すを出す", rt.state.value.recordUndo)

        rt.undoRecordDelete()
        assertEquals(listOf(id), rt.records("memo").map { it.id })
    }

    @Test
    fun deleteFromFormClosesIt() = runTest {
        val rt = rt()
        rt.saveMemo(openForm(rt), "牛乳")
        val record = rt.records("memo").single()
        rt.setNavigation(rt.navigation.value.copy(sheet = sheetForm.copy(recordId = record.id)))
        val draft = FormDraft(rt.entityModel("memo")!!, record)

        rt.run(Action(type = ActionType.DELETE_RECORD), ActionContext(sheetForm.copy(recordId = record.id), form = draft))

        assertTrue(rt.records("memo").isEmpty())
        assertNull(rt.navigation.value.sheet)
    }

    @Test
    fun openVoiceAndResetForm() = runTest {
        val rt = rt()
        rt.run(Action(type = ActionType.OPEN_VOICE), ActionContext(listRoute))
        assertTrue(rt.state.value.showVoiceSheet)

        val draft = openForm(rt)
        draft.values["body"] = JSONValue.Str("書きかけ")
        rt.run(Action(type = ActionType.RESET_FORM), ActionContext(sheetForm, form = draft))
        assertFalse(draft.hasUnsavedChanges)
    }

    @Test
    fun deviceActionsWithoutHostShowFixedToast() = runTest {
        val actions = listOf(
            Action(type = ActionType.FOLLOW),
            Action(type = ActionType.UNFOLLOW),
            Action(type = ActionType.EXPORT, entity = "memo"),
            Action(type = ActionType.ADD_TO_CALENDAR),
            Action(type = ActionType.LOOKUP_BOOK),
            Action(type = ActionType.END_ACTIVITY),
            Action(type = ActionType.CANCEL_NOTIFICATION, notificationId = "n1"),
            Action(type = ActionType.SCHEDULE_NOTIFICATION, notification = Notification(id = "n1", title = "t", body = "b")),
        )
        for (a in actions) {
            val rt = rt()
            rt.run(a, ActionContext(listRoute))
            assertEquals(a.type.wire, "この機能は Android ではまだ使えません", rt.toast)
        }
    }

    @Test
    fun scheduleNotificationToastsWhen() = runTest {
        val host = object : RuntimeHost {
            override suspend fun scheduleNotification(spec: Notification, appId: String): String? = if (spec.id == "ok") "明日 9:00" else null
        }
        val rt = rt(host)
        rt.run(Action(type = ActionType.SCHEDULE_NOTIFICATION, notification = Notification(id = "ok", title = "t", body = "b")), ActionContext(listRoute))
        assertEquals("通知を予約しました(明日 9:00)", rt.toast)
        rt.run(Action(type = ActionType.SCHEDULE_NOTIFICATION, notification = Notification(id = "ng", title = "t", body = "b")), ActionContext(listRoute))
        assertEquals("通知を予約できませんでした", rt.toast)
    }

    @Test
    fun refusedSaveRevokesSavedFeedback() = runTest {
        val rt = rt()
        val draft = openForm(rt)
        rt.saveMemo(draft, "牛乳")
        val id = rt.records("memo").single().id
        assertNotNull(draft.lastSaveResult)

        rt.handle(SyncEvent.Rejected(appId, "memo", "field \"body\" is bad", field = "body", recordId = id))

        assertNull("「保存しました」を取り消す", draft.lastSaveResult)
        assertTrue(draft.hasUnsavedChanges)
        assertEquals("保存できませんでした。「本文」の入力を確認してください", rt.toast)
    }

    @Test
    fun presentationSheetOpensModal() = runTest {
        val rt = rt()
        rt.run(Action(type = ActionType.NAVIGATE, screen = "form", presentation = Presentation.SHEET), ActionContext(listRoute))
        assertEquals("form", rt.navigation.value.sheet?.screen)
    }
}
