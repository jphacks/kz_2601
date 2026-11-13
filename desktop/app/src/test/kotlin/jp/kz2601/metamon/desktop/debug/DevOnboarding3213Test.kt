package jp.kz2601.metamon.desktop.debug

import jp.kz2601.metamon.desktop.net.DesktopApiConfig
import jp.kz2601.metamon.services.consent.InMemoryKeyValueStore
import jp.kz2601.metamon.services.consent.KeyValueStore
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

/** #3213: 開発用メニューの「初回の流れをやり直す」。 */
class DevOnboarding3213Test {
    private class Host : DevMenuHost {
        override val config = DesktopApiConfig.fromEnvironment(mapOf("TAILORIT_API" to "stg"))
        override val pendingCount: Int? = null
        override val canWipe = false
        val messages = mutableListOf<String>()
        override fun showInfo(title: String, lines: List<String>) = Unit
        override fun askWipe() = Unit
        override fun message(text: String) { messages += text }
    }

    /**
     * 初回の流れの規則(#3201・onboarding/Onboarding.kt)のうち「ようこそを出すか」の写し(同意を出し終えた後)。
     * 規則が epic に入ったら `Onboarding(consent, OnboardingSeenStore(settings)).next()` に替える。
     */
    private fun showsWelcome(settings: KeyValueStore) = settings.getBoolean("metamon.tutorial.seen") != true

    @After
    fun unhook() {
        DevOnboardingItems.hook = null
    }

    @Test
    fun afterReplayTheRuleShowsTheFlowAgain() {
        val settings = InMemoryKeyValueStore()
        settings.putBoolean("metamon.tutorial.seen", true)
        settings.putBoolean("metamon.tutorial.editHintShown", true)
        settings.putString("lastAppId", "shopping")
        assertFalse(showsWelcome(settings))

        DevOnboardingItems.hook = SeenFlagsOnboardingReplay(settings)
        val host = Host()
        DevMenu.commands(host, enabled = true).single { it.id == "dev:onboarding:replay" }.run()

        assertTrue(showsWelcome(settings))
        assertEquals(null, settings.getBoolean("metamon.tutorial.editHintShown"))
        assertEquals("shopping", settings.getString("lastAppId")) // 他の設定は残す
        assertEquals(1, host.messages.size)
    }

    @Test
    fun hiddenWithoutHookOrOutsideStg() {
        assertFalse(DevMenu.commands(Host(), enabled = true).any { it.id == "dev:onboarding:replay" })
        DevOnboardingItems.hook = OnboardingReplay { }
        assertTrue(DevMenu.commands(Host(), enabled = true).any { it.id == "dev:onboarding:replay" })
        assertTrue(DevMenu.commands(Host(), enabled = DevMenu.isEnabled(DesktopApiConfig(), packaged = false)).isEmpty())
    }
}
