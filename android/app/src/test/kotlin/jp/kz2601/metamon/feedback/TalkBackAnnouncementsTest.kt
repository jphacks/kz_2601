package jp.kz2601.metamon.feedback

import jp.kz2601.metamon.R
import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.AppDocument
import jp.kz2601.metamon.dsl.decode
import jp.kz2601.metamon.runtime.RuntimeState
import jp.kz2601.metamon.runtime.UiText
import jp.kz2601.metamon.services.ChatTurn
import jp.kz2601.metamon.services.TranscriberFailure
import jp.kz2601.metamon.services.http.FailureKind
import jp.kz2601.metamon.services.local.DeletionTicket
import jp.kz2601.metamon.voice.ChatBubble
import jp.kz2601.metamon.voice.ErrorCardCopy
import jp.kz2601.metamon.voice.UndoAction
import jp.kz2601.metamon.voice.VoiceError
import jp.kz2601.metamon.voice.VoicePhase
import jp.kz2601.metamon.voice.VoiceUiState
import jp.kz2601.metamon.voice.bodyText
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test
import java.util.UUID

/** H8: TalkBack に何を読ませるか(iOS の VoiceOverAnnouncementTests)。 */
class TalkBackAnnouncementsTest {
    private val voiceOnly = AppDSL.decode(
        """{"schemaVersion":1,"app":{"name":"t","voiceEntry":"fullscreen"},"initialScreen":"v","screens":[{"id":"v","components":[{"type":"voiceInput"}]}]}""",
    )
    private val built = AppDSL.decode(
        """{"schemaVersion":1,"app":{"name":"割り勘"},"initialScreen":"home","screens":[{"id":"home","components":[{"id":"a","type":"text","text":"a","area":"A1:F2"}]}]}""",
    )
    private val initial = VoiceUiState(document = AppDocument("app1", 0, voiceOnly))
    private val onApp = VoiceUiState(document = AppDocument("app1", 1, built), phase = VoicePhase.DONE)

    private fun voice(old: VoiceUiState?, new: VoiceUiState) = TalkBackAnnouncements.voice(old, new)

    @Test
    fun aiReplyIsAnnouncedButNotMyOwnWords() {
        val mine = initial.copy(chat = listOf(ChatBubble(fromUser = true, text = "家計簿が欲しい")))
        assertEquals("自分の発言は読まない", emptyList<UiText>(), voice(initial, mine))
        val replied = mine.copy(chat = mine.chat + ChatBubble(fromUser = false, text = "何を記録しますか?", kind = ChatTurn.Kind.QUESTION))
        assertEquals(listOf(UiText.Raw("何を記録しますか?")), voice(mine, replied))
        assertEquals("同じ状態の置き直しでは読まない", emptyList<UiText>(), voice(replied, replied))
    }

    @Test
    fun summaryIsAnnouncedWhenFinished() {
        val sending = onApp.copy(phase = VoicePhase.SENDING)
        val done = sending.copy(phase = VoicePhase.DONE, document = AppDocument("app1", 2, built), summary = "合計を足しました")
        assertEquals(listOf(UiText.Raw("合計を足しました")), voice(sending, done))
    }

    @Test
    fun completionIsAnnouncedWithoutSummary() {
        val sending = initial.copy(phase = VoicePhase.SENDING)
        val created = sending.copy(phase = VoicePhase.DONE, document = AppDocument("app1", 1, built))
        assertEquals(listOf(UiText.Res(R.string.a11y_completed_created, "割り勘")), voice(sending, created))

        val editing = onApp.copy(phase = VoicePhase.SENDING)
        val changed = editing.copy(phase = VoicePhase.DONE, document = AppDocument("app1", 2, built))
        assertEquals(listOf(UiText.Res(R.string.a11y_completed_changed)), voice(editing, changed))
    }

    @Test
    fun reopeningOrHotReloadIsNotAnnounced() {
        val reloaded = onApp.copy(document = AppDocument("app1", 5, built))
        assertEquals(emptyList<UiText>(), voice(onApp, reloaded))
    }

    @Test
    fun undoAndRedoAreAnnouncedEachTime() {
        val undone = onApp.copy(document = AppDocument("app1", 2, built), undone = UndoAction.UNDO)
        assertEquals(listOf(UiText.Res(R.string.feedback_undone)), voice(onApp, undone))
        val again = undone.copy(document = AppDocument("app1", 3, built))
        assertEquals("続けて戻した時も読む", listOf(UiText.Res(R.string.feedback_undone)), voice(undone, again))
        val redone = again.copy(document = AppDocument("app1", 4, built), undone = UndoAction.REDO)
        assertEquals(listOf(UiText.Res(R.string.feedback_redone)), voice(again, redone))
    }

    @Test
    fun errorCardIsAnnouncedOnce() {
        val failed = onApp.copy(phase = VoicePhase.ERROR, error = VoiceError.Request(FailureKind.Timeout, VoiceError.Origin.GENERATE))
        val expected = UiText.Res(R.string.a11y_error_card, UiText.Res(R.string.voice_timeout_title), UiText.Res(R.string.voice_timeout_body))
        assertEquals(listOf(expected), voice(onApp, failed))
        assertEquals("同じカードの置き直しでは読み直さない", emptyList<UiText>(), voice(failed, failed.copy()))
        assertEquals("消した時は読まない", emptyList<UiText>(), voice(failed, failed.copy(error = null)))
    }

    @Test
    fun voiceFailureCardIsAnnounced() {
        val failed = initial.copy(phase = VoicePhase.ERROR, error = VoiceError.Voice(TranscriberFailure.NotHeard))
        val expected = UiText.Res(R.string.a11y_error_card, UiText.Res(R.string.voice_t_not_heard), UiText.Res(R.string.voice_b_not_heard))
        assertEquals(listOf(expected), voice(initial, failed))
    }

    @Test
    fun rateLimitCardSaysTomorrowWhenResetIsUnknown() {
        val copy = ErrorCardCopy(R.string.voice_rate_title, R.string.voice_rate_body, resetAt = null)
        assertEquals(UiText.Res(R.string.voice_rate_body, UiText.Res(R.string.voice_tomorrow)), copy.bodyText(english = false))
    }

    @Test
    fun nothingIsReadOnFirstLook() {
        val shown = onApp.copy(summary = "合計を足しました", error = VoiceError.Voice(TranscriberFailure.NotHeard))
        assertEquals("開いた時点で出ているものは読まない", emptyList<UiText>(), voice(null, shown))
    }

    @Test
    fun toastIsAnnouncedOnceAndNotWhenCleared() {
        val base = RuntimeState(AppDocument("app1", 1, built))
        val toast = UiText.Res(R.string.feedback_no_earlier)
        val shown = base.copy(toast = toast)
        assertEquals(listOf<UiText>(toast), TalkBackAnnouncements.runtime(base, shown))
        assertEquals(emptyList<UiText>(), TalkBackAnnouncements.runtime(shown, shown.copy()))
        assertEquals(emptyList<UiText>(), TalkBackAnnouncements.runtime(shown, base))
    }

    @Test
    fun recordDeletionIsAnnouncedPerTicket() {
        val base = RuntimeState(AppDocument("app1", 1, built))
        val first = base.copy(recordUndo = DeletionTicket(UUID.randomUUID(), "app1", "items", "r1"))
        assertEquals(listOf(UiText.Res(R.string.feedback_deleted)), TalkBackAnnouncements.runtime(base, first))
        val second = first.copy(recordUndo = DeletionTicket(UUID.randomUUID(), "app1", "items", "r2"))
        assertEquals("続けて消した時も読む", listOf(UiText.Res(R.string.feedback_deleted)), TalkBackAnnouncements.runtime(first, second))
        assertTrue(TalkBackAnnouncements.runtime(second, base).isEmpty())
    }
}
