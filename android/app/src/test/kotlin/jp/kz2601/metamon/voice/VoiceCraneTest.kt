package jp.kz2601.metamon.voice

import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.AppDocument
import jp.kz2601.metamon.dsl.decode
import jp.kz2601.metamon.ui.CranePose
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/** #1132: 音声画面のツル(iOS の `VoiceInputView.heading`)。 */
class VoiceCraneTest {
    private val voiceOnly = AppDSL.decode(
        """{"schemaVersion":1,"app":{"name":"t","voiceEntry":"fullscreen"},"initialScreen":"v","screens":[{"id":"v","components":[{"type":"voiceInput"}]}]}""",
    )
    private val built = AppDSL.decode(
        """{"schemaVersion":1,"app":{"name":"割り勘"},"initialScreen":"home","screens":[{"id":"home","components":[{"id":"a","type":"text","text":"a","area":"A1:F2"}]}]}""",
    )
    private val initial = VoiceUiState(document = AppDocument("app1", 0, voiceOnly))
    private val onApp = VoiceUiState(document = AppDocument("app1", 1, built), phase = VoicePhase.DONE)

    @Test
    fun `話しかける前の画面は状態によらず happy`() {
        for (phase in VoicePhase.entries) {
            assertEquals(phase.name, CranePose.HAPPY, VoiceCrane.pose(initial.copy(phase = phase), compactHeight = false))
        }
    }

    @Test
    fun `縦が狭い時は出さない`() {
        assertNull(VoiceCrane.pose(initial, compactHeight = true))
    }

    @Test
    fun `吹き出しの画面では出さない`() {
        val chatting = initial.copy(chat = listOf(ChatBubble(fromUser = true, text = "家計簿が欲しい")))
        assertNull(VoiceCrane.pose(chatting, compactHeight = false))
    }

    @Test
    fun `作ったアプリの上のシートでは出さない`() {
        assertNull(VoiceCrane.pose(onApp, compactHeight = false))
    }

    @Test
    fun `縦 480dp 未満を狭いとみなす`() {
        assertTrue(VoiceCrane.isCompactHeight(411))
        assertFalse(VoiceCrane.isCompactHeight(480))
        assertFalse(VoiceCrane.isCompactHeight(800))
    }
}
