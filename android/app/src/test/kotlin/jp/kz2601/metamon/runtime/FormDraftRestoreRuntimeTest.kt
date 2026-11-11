package jp.kz2601.metamon.runtime

import jp.kz2601.metamon.account.LocalAppForget
import jp.kz2601.metamon.account.LocalDataWipe
import jp.kz2601.metamon.dsl.Action
import jp.kz2601.metamon.dsl.ActionType
import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.decode
import jp.kz2601.metamon.dsl.records.JSONValue
import jp.kz2601.metamon.renderer.ActionContext
import jp.kz2601.metamon.renderer.form.FormDraft
import jp.kz2601.metamon.renderer.form.FormDraftStorage
import jp.kz2601.metamon.renderer.navigation.NavigationState
import jp.kz2601.metamon.renderer.navigation.Route
import jp.kz2601.metamon.runtime.RuntimeFixture.runtime
import jp.kz2601.metamon.services.local.AppListCache
import jp.kz2601.metamon.services.local.DocumentCache
import jp.kz2601.metamon.switcher.AppThumbnailStore
import jp.kz2601.metamon.voice.ChatHistoryStore
import kotlinx.coroutines.test.TestScope
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.rules.TemporaryFolder
import kotlin.coroutines.EmptyCoroutineContext

/** #1185: 画面の位置を戻せた時だけフォームの書きかけを戻し、保存・後片付けで消す。 */
class FormDraftRestoreRuntimeTest {
    @get:Rule
    val tmp = TemporaryFolder()

    private val appId = "test-drafts"
    private val dsl = AppDSL.decode(
        """
        {"schemaVersion":1,"app":{"name":"メモ","voiceEntry":"floating"},"initialScreen":"list",
         "entities":[{"id":"memo","name":"メモ","fields":[{"id":"body","name":"本文","type":"text"}]}],
         "screens":[{"id":"list","title":"一覧","components":[{"type":"text","text":"一覧"}]},
                    {"id":"form","title":"メモ","entity":"memo","components":[{"type":"textField","field":"body"}]}]}
        """.trimIndent(),
    )
    private val sheet = Route("form", stack = Route.SHEET)
    private val navigation = NavigationState(sheet = sheet)

    private fun TestScope.rt(storage: FormDraftStorage) = runtime(dsl, appId = appId, formDrafts = storage)

    /** 前のプロセス: シートのフォームに書きかけ → 背面で保存状態を書く。 */
    private fun TestScope.leaveInBackground(storage: FormDraftStorage): Pair<AppRuntime, FormDraft> {
        val rt = rt(storage)
        val form = FormDraft(rt.entityModel("memo")!!).apply { values["body"] = JSONValue.Str("牛乳") }
        rt.formDrafts.attach(sheet.id, form, emptySet())
        rt.persistFormDrafts()
        return rt to form
    }

    @Test
    fun restoredNavigationBringsTheDraftBack() = runTest {
        val storage = FormDraftStorage.InMemory()
        val (old, _) = leaveInBackground(storage)
        old.setNavigation(navigation)
        val saved = old.navigationSnapshot()

        val rt = rt(storage)
        rt.offerNavigationRestore(saved)
        rt.bootstrap()
        assertEquals(navigation, rt.navigation.value)
        val form = FormDraft(rt.entityModel("memo")!!)
        assertTrue(rt.formDrafts.restore(sheet.id, form))
        assertEquals(JSONValue.Str("牛乳"), form.values["body"])
    }

    @Test
    fun navigationThatCannotBeRestoredDropsTheDraft() = runTest {
        val storage = FormDraftStorage.InMemory()
        leaveInBackground(storage)
        val rt = rt(storage)
        rt.offerNavigationRestore(NavigationSnapshot("another-app", navigation))
        rt.bootstrap()
        assertFalse(rt.formDrafts.restore(sheet.id, FormDraft(rt.entityModel("memo")!!)))
        assertNull(storage.text)
    }

    @Test
    fun savingClearsThePersistedDraft() = runTest {
        val storage = FormDraftStorage.InMemory()
        val (rt, form) = leaveInBackground(storage)
        assertNotNull(storage.text)
        rt.run(Action(type = ActionType.SAVE_RECORD, entity = "memo"), ActionContext(sheet, form = form))
        assertEquals(1, form.saveCount)
        assertNull(storage.text)
    }

    @Test
    fun localDataWipeClearsAllDrafts() = runTest {
        val storage = FormDraftStorage.InMemory()
        val (rt, _) = leaveInBackground(storage)
        LocalDataWipe(
            rt.recordSync, DocumentCache(null, EmptyCoroutineContext), AppListCache(null),
            resetRuntime = {}, forgetDeviceState = {},
            forgetFormDrafts = rt.formDrafts::clear,
        )()
        assertNull(storage.text)
    }

    @Test
    fun deletingTheAppClearsItsDrafts() = runTest {
        val storage = FormDraftStorage.InMemory()
        val (rt, _) = leaveInBackground(storage)
        LocalAppForget(
            rt.recordSync, DocumentCache(null, EmptyCoroutineContext), AppThumbnailStore(null), ChatHistoryStore(tmp.newFolder("chat")),
            forgetFormDrafts = rt.formDrafts::forgetApp,
        )(appId)
        assertNull(storage.text)
    }
}
