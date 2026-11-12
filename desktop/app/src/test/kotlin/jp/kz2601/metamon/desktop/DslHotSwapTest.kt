package jp.kz2601.metamon.desktop

import jp.kz2601.metamon.desktop.i18n.Lang
import jp.kz2601.metamon.desktop.i18n.Strings
import jp.kz2601.metamon.desktop.store.RecordChangeSubscriptions
import jp.kz2601.metamon.dsl.Action
import jp.kz2601.metamon.dsl.ActionType
import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.AppDocument
import jp.kz2601.metamon.dsl.Presentation
import jp.kz2601.metamon.dsl.records.JSONValue
import jp.kz2601.metamon.services.FetchedDocument
import jp.kz2601.metamon.services.sync.DocumentSync
import jp.kz2601.metamon.services.sync.SyncUpdate
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.flow.MutableSharedFlow
import kotlinx.coroutines.test.runCurrent
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotEquals
import org.junit.Assert.assertSame
import org.junit.Assert.assertTrue
import org.junit.Test

/** アプリの版が変わった通知(dsl)で DSL を差し替える。入力中は待つ(#2809)。 */
@OptIn(ExperimentalCoroutinesApi::class)
class DslHotSwapTest {
    private val v1: AppDSL = Examples.load("shopping")
    private val v2: AppDSL = v1.copy(app = v1.app.copy(name = "買い物 v2"))

    @Test
    fun swapsRightAwayWhenNotEditing() {
        val hub = AppHub()
        val app = hub.acquire("shopping")
        hub.trackEditing(app) { false }

        assertTrue(hub.offerDsl("shopping", v2))
        assertSame(v2, app.dsl)
        assertEquals(1, app.reloads)
        assertFalse(hub.isWaiting("shopping"))
    }

    @Test
    fun waitsWhileEditingAndSwapsWhenSettled() {
        val hub = AppHub()
        val app = hub.acquire("shopping")
        var editing = true
        hub.trackEditing(app) { editing }

        assertFalse(hub.offerDsl("shopping", v2))
        assertEquals(v1, app.dsl)
        assertEquals(0, app.reloads)
        assertTrue(hub.isWaiting("shopping"))

        // まだ入力中に知らせても当てない
        app.records.editsSettled()
        assertEquals(v1, app.dsl)

        editing = false
        app.records.editsSettled()
        assertSame(v2, app.dsl)
        assertEquals(1, app.reloads)
        assertFalse(hub.isWaiting("shopping"))
    }

    @Test
    fun closingTheEditingWindowSwaps() {
        val hub = AppHub()
        val app = hub.acquire("shopping")
        val untrack = app.records.trackEdits { true }
        assertFalse(hub.offerDsl("shopping", v2))

        untrack()
        assertSame(v2, app.dsl)
    }

    @Test
    fun notOpenedAppTakesNewDslNextTime() {
        val hub = AppHub()
        hub.dsl("shopping")
        assertFalse(hub.offerDsl("shopping", v2))
        assertSame(v2, hub.dsl("shopping"))
        assertSame(v2, hub.acquire("shopping").dsl)
    }

    @Test
    fun sessionReportsUnsavedForm() {
        val s = DesktopSession(v1)
        assertFalse(s.hasUnsavedForms)
        s.dispatch(Action(type = ActionType.NAVIGATE, screen = "item_form", presentation = Presentation.SHEET), ActionContext(s.top, null))
        s.form(s.top)!!.values["name"] = JSONValue.Str("牛乳")
        assertTrue(s.hasUnsavedForms)
        s.back()
        assertFalse(s.hasUnsavedForms)
    }

    @Test
    fun dslNotificationReachesHub() = runTest {
        val flow = MutableSharedFlow<SyncUpdate>()
        lateinit var hub: AppHub
        val live = RecordChangeSubscriptions(
            sync = DocumentSync { _, _ -> flow },
            scope = backgroundScope,
            onServer = { true },
            version = { 1 },
            visible = { emptySet() },
            refetch = { _, _ -> },
            onDocument = { id, doc -> hub.offerDsl(id, doc.document.dsl) },
            log = {},
        )
        hub = AppHub(live = live)
        val app = hub.acquire("shopping")
        runCurrent()

        flow.emit(SyncUpdate.Document(FetchedDocument(AppDocument("shopping", 2, v2), "{}")))
        runCurrent()
        assertSame(v2, app.dsl)
    }

    @Test
    fun noticeUsesAndroidKeyInBothLanguages() {
        val ja = Strings.withLang(Lang.JA) { Strings["runtime_hot_reloaded"] }
        val en = Strings.withLang(Lang.EN) { Strings["runtime_hot_reloaded"] }
        assertEquals("最新の内容に更新しました", ja)
        assertEquals("Updated to the latest version", en)
        assertNotEquals(ja, en)
    }

    private fun AppHub.trackEditing(app: AppHub.OpenedApp, probe: () -> Boolean) {
        app.records.trackEdits(probe)
    }
}
