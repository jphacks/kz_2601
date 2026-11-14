package jp.kz2601.metamon.desktop.stage

import jp.kz2601.metamon.desktop.i18n.Lang
import jp.kz2601.metamon.desktop.i18n.StringTable
import jp.kz2601.metamon.desktop.store.DesktopStore
import jp.kz2601.metamon.desktop.store.JsonKeyValueStore
import kotlinx.coroutines.runBlocking
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.rules.TemporaryFolder

/** 最初のアプリの後の「こう頼むと直せます」を 1 回だけ出す(#3360・Android の EditHintStore)。 */
class EditHint3360Test {
    @get:Rule
    val tmp = TemporaryFolder()

    @Test
    fun showsOnlyOnceAfterTheFirstApp() {
        val hints = EditHintStore(JsonKeyValueStore(null))
        assertTrue("初回は出す", hints.take(createdFromInitial = true))
        assertFalse("2 回目は出さない", hints.take(createdFromInitial = true))
        assertTrue(hints.hasShown)
    }

    @Test
    fun notFromTheInitialFlowDoesNotShowOrMark() {
        val hints = EditHintStore(JsonKeyValueStore(null))
        assertFalse(hints.take(createdFromInitial = false))
        assertFalse("出していないので覚えない", hints.hasShown)
        assertTrue(hints.shouldShow(createdFromInitial = true))
    }

    @Test
    fun rememberedInTheSettingsFileAcrossRestarts() {
        val root = tmp.newFolder()
        assertTrue(EditHintStore(DesktopStore(root).settings).take(createdFromInitial = true))
        assertFalse("起動し直しても出さない", EditHintStore(DesktopStore(root).settings).take(createdFromInitial = true))
        assertTrue(root.resolve(DesktopStore.SETTINGS_FILE).readText().contains(EditHintStore.KEY))
    }

    @Test
    fun wipingTheDeviceShowsItAgainForTheNextPerson() = runBlocking {
        val root = tmp.newFolder()
        val store = DesktopStore(root)
        EditHintStore(store.settings).markShown()
        store.wipe()
        assertTrue(EditHintStore(DesktopStore(root).settings).shouldShow(createdFromInitial = true))
    }

    @Test
    fun stringsResolveInBothLanguages() {
        val t = StringTable.fromClasspath()
        val keys = listOf(
            "desktop_edit_hint_body", "desktop_edit_hint_title", "desktop_edit_hint_try",
            "feedback_edit_hint_close", "feedback_edit_hint_examples",
        )
        for (lang in Lang.entries) for (k in keys) assertTrue("$k ($lang)", t.hasString(lang, k))
    }
}
