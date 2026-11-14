package jp.kz2601.metamon.desktop.onboarding

import jp.kz2601.metamon.desktop.store.DesktopStore
import kotlinx.coroutines.runBlocking
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Test

/** 初回の案内の順番と「もう見た」の規則(iOS の FirstRunTutorial・Mac の MacOnboarding)。 */
class OnboardingTest {
    private fun fresh(): Triple<DesktopStore, OnboardingSeenStore, Onboarding> {
        val store = DesktopStore(null)
        val seen = OnboardingSeenStore(store.settings)
        return Triple(store, seen, Onboarding(store.aiConsent, seen))
    }

    @Test
    fun firstRunOrder() {
        val (_, _, o) = fresh()
        val shown = mutableListOf<OnboardingStep>()
        // 最初のアプリを作る前: 同意 → ようこそ で止まる
        while (true) {
            val s = o.next() ?: break
            shown += s
            o.finish(s)
        }
        assertEquals(listOf(OnboardingStep.CONSENT, OnboardingStep.WELCOME), shown)
        // 最初のアプリの直後に 1 回だけ
        assertEquals(OnboardingStep.EDIT_HINT, o.next(firstAppMade = true))
        o.finish(OnboardingStep.EDIT_HINT)
        assertNull(o.next(firstAppMade = true))
    }

    @Test
    fun consentGrantedThenWelcome() {
        val (store, _, o) = fresh()
        store.aiConsent.grant()
        assertEquals(OnboardingStep.WELCOME, o.next())
    }

    @Test
    fun noEditHintBeforeWelcome() {
        val (store, _, o) = fresh()
        assertEquals("同意より先には何も出さない", OnboardingStep.CONSENT, o.next(firstAppMade = true))
        store.aiConsent.markPrompted()
        assertEquals(OnboardingStep.WELCOME, o.next(firstAppMade = true))
    }

    @Test
    fun seenIsCleared() {
        val (store, seen, o) = fresh()
        store.aiConsent.grant()
        o.finish(OnboardingStep.WELCOME)
        o.finish(OnboardingStep.EDIT_HINT)
        assertNull(o.next(firstAppMade = true))
        // サインアウト・「この端末のデータを消す」は DesktopStore.wipe(設定と同意を消す)
        runBlocking { store.wipe() }
        assertFalse(seen.hasSeenWelcome)
        assertFalse(seen.hasShownEditHint)
        assertEquals("次の人には最初から", OnboardingStep.CONSENT, o.next())
    }

    @Test
    fun forgetClearsOnlyTheMarks() {
        val (store, seen, o) = fresh()
        store.aiConsent.grant()
        o.finish(OnboardingStep.WELCOME)
        seen.forget()
        assertEquals(OnboardingStep.WELCOME, o.next())
    }
}
