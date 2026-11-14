package jp.kz2601.metamon.desktop

import jp.kz2601.metamon.desktop.voice.PromptTipsText
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

/** 頼み方のコツ: パレットから開く・英語の名前でも探せる(#2973)。 */
class PromptTipsPaletteTest {
    @Test
    fun paletteOpensTheTipsDialog() {
        val s = DesktopSession(Examples.load("shopping"), MockStore())
        val k = DesktopKeyboard(s.dsl, s, HostOs.WINDOWS)
        val command = k.commands().single { it.id == "help:prompt_tips" }
        assertEquals(PromptTipsText.TITLE, command.title)
        assertEquals(command.id, CommandRanking.rank(k.commands(), "tips").first().id)
        assertFalse(k.tipsOpen)
        command.run()
        assertTrue(k.tipsOpen)
    }
}
