package jp.kz2601.metamon.feedback

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

/** #1106: トーストを画面とシートのどちらに出すか(iOS の `ToastHost(onModal:)`)。 */
class ToastPlacementTest {
    @Test
    fun `作ったアプリの上で音声のシートを開いている間はシートに出す`() {
        assertTrue(ToastPlacement.voiceSheetShown(isInitial = false, voiceOpen = true))
        assertEquals(ToastPlacement.VOICE_SHEET, ToastPlacement.of(isInitial = false, voiceOpen = true))
    }

    @Test
    fun `シートを閉じている時は画面に出す`() {
        assertEquals(ToastPlacement.SCREEN, ToastPlacement.of(isInitial = false, voiceOpen = false))
    }

    @Test
    fun `話しかける前の画面はシートを使わないので画面に出す`() {
        assertFalse(ToastPlacement.voiceSheetShown(isInitial = true, voiceOpen = true))
        assertEquals(ToastPlacement.SCREEN, ToastPlacement.of(isInitial = true, voiceOpen = true))
        assertEquals(ToastPlacement.SCREEN, ToastPlacement.of(isInitial = true, voiceOpen = false))
    }
}
