package jp.kz2601.metamon.voice

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

/** #1014: 認識器が自分で止まった時は送る前に確かめる。 */
class VoiceConfirmTest {
    @Test
    fun tapStopSendsAndSelfStopAsks() {
        assertEquals(VoiceConfirm.Outcome.Send("家計簿"), VoiceConfirm.afterTranscribe(autoStopped = false, carried = null, text = "家計簿"))
        assertEquals(VoiceConfirm.Outcome.Confirm("家計簿"), VoiceConfirm.afterTranscribe(autoStopped = true, carried = null, text = " 家計簿 "))
    }

    @Test
    fun continuedPartIsJoined() {
        assertEquals(
            VoiceConfirm.Outcome.Send("体重と睡眠を記録"),
            VoiceConfirm.afterTranscribe(autoStopped = false, carried = "体重と", text = "睡眠を記録"),
        )
        assertEquals(
            VoiceConfirm.Outcome.Confirm("track weight and sleep"),
            VoiceConfirm.afterTranscribe(autoStopped = true, carried = "track weight", text = "and sleep"),
        )
    }

    @Test
    fun joinSpacesOnlyBetweenLatinWords() {
        assertEquals("abc def", VoiceConfirm.join("abc", "def"))
        assertEquals("Hi. there", VoiceConfirm.join("Hi.", "there"))
        assertEquals("家計簿アプリ", VoiceConfirm.join("家計簿", "アプリ"))
        assertEquals("ToDoリスト", VoiceConfirm.join("ToDo", "リスト"))
        assertEquals("x", VoiceConfirm.join(null, "x"))
        assertEquals("x", VoiceConfirm.join("x", " "))
    }

    @Test
    fun failureAfterContinueKeepsTheFirstPart() {
        assertEquals(VoiceConfirm.Outcome.Confirm("家計簿"), VoiceConfirm.afterFailure("家計簿"))
        assertNull(VoiceConfirm.afterFailure(null))
        assertNull(VoiceConfirm.afterFailure("  "))
    }
}
