package jp.kz2601.metamon.desktop.onboarding

import androidx.compose.ui.input.key.Key
import jp.kz2601.metamon.desktop.DesktopKeyboard
import jp.kz2601.metamon.desktop.DesktopSession
import jp.kz2601.metamon.desktop.Examples
import jp.kz2601.metamon.desktop.HostOs
import jp.kz2601.metamon.desktop.KeyPress
import jp.kz2601.metamon.desktop.MockStore
import jp.kz2601.metamon.desktop.i18n.Lang
import jp.kz2601.metamon.desktop.i18n.Strings
import jp.kz2601.metamon.desktop.store.JsonKeyValueStore
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.rules.TemporaryFolder

/** 初回の流れ(#3201): 3 枚の順・文言(日本語・英語)・枚数の読み上げ・見たかの保存・やり直し・F1 / パレットからの入り口。 */
class OnboardingDialog3201Test {
    @get:Rule
    val tmp = TemporaryFolder()

    private val saved = Strings.lang

    @After
    fun restore() {
        Strings.lang = saved
    }

    @Test
    fun threeCardsInOrderWithCopyInBothLanguages() {
        assertEquals(listOf("create", "keys", "windows"), OnboardingCards.all.map { it.id })
        for (lang in Lang.entries) {
            Strings.lang = lang
            for (card in OnboardingCards.all) {
                assertTrue("$lang ${card.titleKey}", Strings.table.hasString(lang, card.titleKey))
                val body = card.body(HostOs.WINDOWS)
                assertFalse("$lang ${card.id} の本文が引けている", body.startsWith("desktop_") || body.startsWith("onboarding_"))
            }
            for (key in listOf("desktop_onboarding_title", "desktop_onboarding_later", "desktop_onboarding_later_hint", "onboarding_next", "onboarding_start", "onboarding_replay")) {
                assertTrue("$lang $key", Strings.table.hasString(lang, key))
            }
        }
    }

    @Test
    fun keysCardNamesTheOsKeys() {
        Strings.lang = Lang.JA
        val keys = OnboardingCards.all.first { it.id == "keys" }
        assertTrue(keys.body(HostOs.WINDOWS).contains("Ctrl+K"))
        assertTrue(keys.body(HostOs.WINDOWS).contains("F1"))
        assertTrue(keys.body(HostOs.MAC).contains("⌘K"))
    }

    @Test
    fun pageLabelForScreenReaders() {
        assertEquals("1 / 3", OnboardingCards.pageLabel(0))
        assertEquals("3 / 3", OnboardingCards.pageLabel(2))
    }

    @Test
    fun pagerWalksToTheEnd() {
        val p = OnboardingPager()
        assertTrue(p.isFirst)
        assertTrue(p.next())
        p.back()
        assertEquals(0, p.index)
        p.back()
        assertEquals(0, p.index)
        assertTrue(p.next())
        assertTrue(p.next())
        assertTrue(p.isLast)
        assertFalse("最後の 1 枚の「はじめる」で終わり", p.next())
        assertEquals(2, p.index)
    }

    @Test
    fun onboardingShownOnceAndResettable() {
        val file = tmp.root.resolve("settings.json")
        val store = OnboardingStore(JsonKeyValueStore(file))
        assertTrue(store.shouldAutoPresent())
        store.markSeen()
        assertFalse("再起動しても出さない", OnboardingStore(JsonKeyValueStore(file)).shouldAutoPresent())
        assertFalse(store.applyLaunchOverride(null))
        assertFalse(store.applyLaunchOverride("0"))
        assertFalse(store.shouldAutoPresent())
        assertTrue(store.applyLaunchOverride("1"))
        assertTrue("TAILORIT_RESET_ONBOARDING=1 でもう一度出す", store.shouldAutoPresent())
        assertEquals("iOS と同じ鍵", "metamon.tutorial.seen", OnboardingStore.SEEN_KEY)
    }

    @Test
    fun paletteOpensOnboardingAndKeysWaitWhileOpen() {
        val s = DesktopSession(Examples.load("shopping"), MockStore())
        val k = DesktopKeyboard(s.dsl, s, HostOs.WINDOWS)
        val cmd = k.commands().single { it.id == "help:onboarding" }
        assertEquals(Strings["onboarding_replay"], cmd.title)
        cmd.run()
        assertTrue(k.onboardingOpen)
        assertFalse("開いている間は F1 などをダイアログに任せる", k.handle(KeyPress(Key.F1), bubble = true))
        assertFalse(k.helpOpen)
    }
}
