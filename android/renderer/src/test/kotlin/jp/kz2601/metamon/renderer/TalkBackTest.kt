package jp.kz2601.metamon.renderer

import jp.kz2601.metamon.renderer.a11y.TalkBack
import jp.kz2601.metamon.renderer.components.ai.AiChatAnnounceQueue
import jp.kz2601.metamon.renderer.components.ai.AiChatAutoRead
import jp.kz2601.metamon.renderer.components.ai.DictationPhase
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

/** H8: TalkBack の時の出し方(iOS の VoiceOverAnnouncer.holdSeconds・RecordUndo.seconds・AiChatLeaf.autoReadMode)。 */
class TalkBackTest {
    @Test
    fun holdIsLongerWithTalkBack() {
        assertEquals(3_000L, TalkBack.holdMillis(3_000L, talkBack = false))
        assertEquals("短い知らせも 15 秒は出す", 15_000L, TalkBack.holdMillis(3_000L, talkBack = true))
        assertEquals("長い知らせは 3 倍", 18_000L, TalkBack.holdMillis(6_000L, talkBack = true))
    }

    @Test
    fun recordUndoIsTenSecondsWithTalkBack() {
        assertEquals(5_000L, TalkBack.recordUndoMillis(talkBack = false))
        assertEquals(10_000L, TalkBack.recordUndoMillis(talkBack = true))
    }

    @Test
    fun blankTextIsNotRead() {
        assertNull(TalkBack.text("  \n"))
        assertNull(TalkBack.text(null))
        assertEquals("保存しました", TalkBack.text(" 保存しました\n"))
    }

    @Test
    fun aiChatReplyGoesToTalkBackInsteadOfTheDeviceVoice() {
        assertEquals(AiChatAutoRead.SPEAK, AiChatAutoRead.mode(speak = true, visible = true, recording = false, talkBack = false))
        assertEquals(AiChatAutoRead.ANNOUNCE, AiChatAutoRead.mode(speak = true, visible = true, recording = false, talkBack = true))
        assertEquals("録音中は終わるまで待つ", AiChatAutoRead.QUEUE, AiChatAutoRead.mode(speak = true, visible = true, recording = true, talkBack = true))
        assertEquals(AiChatAutoRead.NONE, AiChatAutoRead.mode(speak = true, visible = true, recording = true, talkBack = false))
        assertEquals("画面を離れた後は読まない", AiChatAutoRead.NONE, AiChatAutoRead.mode(speak = true, visible = false, recording = false, talkBack = true))
        assertEquals(AiChatAutoRead.NONE, AiChatAutoRead.mode(speak = null, visible = true, recording = false, talkBack = true))
    }

    @Test
    fun queuedReplyIsReadAfterRecordingEnds() {
        val q = AiChatAnnounceQueue()
        q.received("一つ目", AiChatAutoRead.QUEUE)
        q.received("二つ目", AiChatAutoRead.QUEUE)
        assertNull("録音中は返さない", q.phaseChanged(DictationPhase.TRANSCRIBING, visible = true, talkBack = true))
        assertEquals("最新の 1 件だけ", "二つ目", q.phaseChanged(DictationPhase.IDLE, visible = true, talkBack = true))
        assertNull("読んだら空", q.phaseChanged(DictationPhase.IDLE, visible = true, talkBack = true))

        q.received("三つ目", AiChatAutoRead.QUEUE)
        assertNull("TalkBack を切った後は読まない", q.phaseChanged(DictationPhase.IDLE, visible = true, talkBack = false))
        q.received("四つ目", AiChatAutoRead.ANNOUNCE)
        assertNull("待たせる分だけ持つ", q.pending)
        q.received("五つ目", AiChatAutoRead.QUEUE)
        q.clear()
        assertNull(q.phaseChanged(DictationPhase.IDLE, visible = true, talkBack = true))
    }
}
