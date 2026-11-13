@file:OptIn(androidx.compose.ui.test.ExperimentalTestApi::class)

package jp.kz2601.metamon.desktop.ui

import androidx.compose.ui.input.key.Key
import androidx.compose.ui.test.hasSetTextAction
import androidx.compose.ui.test.hasText
import androidx.compose.ui.test.performKeyInput
import androidx.compose.ui.test.performTextInput
import androidx.compose.ui.test.pressKey
import androidx.compose.ui.test.runComposeUiTest
import jp.kz2601.metamon.desktop.CommandPalette
import jp.kz2601.metamon.desktop.HostOs
import jp.kz2601.metamon.desktop.PaletteCommand
import org.junit.Assert.assertEquals
import org.junit.Test

/** コマンドパレットを開く → 文字で絞る → Enter で選ぶ(#2415)。 */
class CommandPaletteUiTest2415 {
    @Test
    fun filterAndEnterRunsTheMatch() = runComposeUiTest {
        val ran = mutableListOf<String>()
        val commands = listOf(
            PaletteCommand("a", "Alpha", PaletteCommand.Kind.ACTION) { ran += "a" },
            PaletteCommand("b", "Banana", PaletteCommand.Kind.ACTION) { ran += "b" },
        )
        var dismissed = 0
        setContent { CommandPalette(commands, HostOs.LINUX) { dismissed++ } }

        onNode(hasText("Alpha")).assertExists()
        onNode(hasText("Banana")).assertExists()

        onNode(hasSetTextAction()).performTextInput("ban")
        onNode(hasText("Alpha")).assertDoesNotExist()
        onNode(hasText("Banana")).assertExists()

        onNode(hasSetTextAction()).performKeyInput { pressKey(Key.Enter) }
        assertEquals(listOf("b"), ran)
        assertEquals(1, dismissed)
    }
}
