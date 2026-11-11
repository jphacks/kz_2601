package jp.kz2601.metamon.onboarding

import jp.kz2601.metamon.account.LocalDataWipe
import jp.kz2601.metamon.runtime.RuntimeFixture
import jp.kz2601.metamon.services.consent.InMemoryKeyValueStore
import jp.kz2601.metamon.services.local.AppListCache
import jp.kz2601.metamon.services.local.BackoffPolicy
import jp.kz2601.metamon.services.local.DocumentCache
import jp.kz2601.metamon.services.local.RecordStore
import jp.kz2601.metamon.services.local.RecordSyncEngine
import jp.kz2601.metamon.services.local.SyncQueue
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test
import kotlin.coroutines.EmptyCoroutineContext

/** 初回のチュートリアル(#900・iOS の FirstRunTutorialTests): 出す時点・見た印・「こう話すと直せます」は 1 回だけ・後片付け。 */
class FirstRunTutorialTest {
    private val prefs = InMemoryKeyValueStore()
    private val store = FirstRunTutorialStore(prefs)
    private val prompt = TutorialPrompt(store)

    @Test
    fun oneCardLikeIos() {
        assertEquals(listOf("create"), FirstRunTutorial.cards.map { it.id })
    }

    @Test
    fun presentsOnlyAfterConsentWasExplained() {
        prompt.presentIfNeeded(consentPrompted = false, consentShowing = false)
        assertFalse("同意の説明より先には出さない", prompt.visible.value)

        prompt.presentIfNeeded(consentPrompted = true, consentShowing = true)
        assertFalse("同意の画面と重ねない", prompt.visible.value)

        prompt.presentIfNeeded(consentPrompted = true, consentShowing = false)
        assertTrue(prompt.visible.value)
    }

    @Test
    fun finishMarksSeenAndDoesNotAutoPresentAgain() {
        prompt.presentIfNeeded(consentPrompted = true, consentShowing = false)
        prompt.finish()
        assertFalse(prompt.visible.value)
        assertTrue(store.hasSeenTutorial.value)

        // 次の起動(同じ置き場)でも出さない
        val next = TutorialPrompt(FirstRunTutorialStore(prefs))
        next.presentIfNeeded(consentPrompted = true, consentShowing = false)
        assertFalse(next.visible.value)
    }

    @Test
    fun settingsReplayShowsEvenAfterSeen() {
        store.markTutorialSeen()
        prompt.replay()
        assertTrue(prompt.visible.value)
        prompt.finish()
        assertFalse(prompt.visible.value)
    }

    @Test
    fun editHintShowsOnlyOnceAndOnlyAfterCreatingFromInitial() {
        val hints = store.editHints
        assertFalse("初期画面からの作成でなければ出さない", hints.shouldShow(createdFromInitial = false))
        assertTrue(hints.shouldShow(createdFromInitial = true))
        hints.markShown()
        assertFalse(hints.shouldShow(createdFromInitial = true))
        assertFalse("次の起動でも出さない", FirstRunTutorialStore(prefs).editHints.shouldShow(createdFromInitial = true))
        assertFalse("チュートリアルの見た印とは別", store.hasSeenTutorial.value)
    }

    @Test
    fun keepsTheEditHintKeyFrom1157() {
        // #1157 の EditHintStore が書いていた置き場(metamon.tutorial / editHintShown)をそのまま読む
        prefs.putBoolean("editHintShown", true)
        assertFalse(FirstRunTutorialStore(prefs).editHints.shouldShow(createdFromInitial = true))
        assertEquals("metamon.tutorial", FirstRunTutorialStore.PREFS)
    }

    @Test
    fun mockStartsWithEverythingSeen() {
        val mock = FirstRunTutorialStore(InMemoryKeyValueStore()).apply { markAllSeen() }
        assertFalse(mock.shouldAutoPresent(consentPrompted = true))
        assertFalse(mock.editHints.shouldShow(createdFromInitial = true))
    }

    @Test
    fun localDataWipeForgetsBothMarksLikeIosLocalWipe() = runTest {
        store.markAllSeen()
        prompt.replay()
        val records = RecordStore(null, EmptyCoroutineContext)
        val client = RuntimeFixture.offlineClient
        val queue = SyncQueue(null, records, client, BackoffPolicy.immediate, sleep = {}, io = EmptyCoroutineContext)
        val engine = RecordSyncEngine(records, queue, client, RuntimeFixture.Offline(), backgroundScope)

        LocalDataWipe(
            engine, DocumentCache(null, EmptyCoroutineContext), AppListCache(null),
            resetRuntime = {},
            forgetDeviceState = {},
            forgetTutorial = {
                store.forget()
                prompt.reset()
            },
        )()

        assertFalse(prompt.visible.value)
        assertFalse(store.hasSeenTutorial.value)
        assertTrue("次の人には同意の後に出し直す", store.shouldAutoPresent(consentPrompted = true))
        assertTrue("次の人の最初のアプリの後に出し直す", store.editHints.shouldShow(createdFromInitial = true))
    }
}
