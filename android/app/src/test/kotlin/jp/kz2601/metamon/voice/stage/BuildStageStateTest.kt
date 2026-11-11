package jp.kz2601.metamon.voice.stage

import jp.kz2601.metamon.R
import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.AppDocument
import jp.kz2601.metamon.dsl.decode
import jp.kz2601.metamon.services.StreamedComponent
import jp.kz2601.metamon.voice.BuildProgress
import jp.kz2601.metamon.voice.VoicePhase
import jp.kz2601.metamon.voice.VoiceUiState
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/** 生成中 → 完成の見せ方の移り変わり(iOS の BuildChoreographyTests の runtime の分・PostBuildNoticeTests・PositionHint)。 */
class BuildStageStateTest {
    private val voiceOnly = AppDSL.decode(
        """{"schemaVersion":1,"app":{"name":"t","voiceEntry":"fullscreen"},"initialScreen":"v","screens":[{"id":"v","components":[{"type":"voiceInput"}]}]}""",
    )

    private fun app(vararg components: String) = AppDSL.decode(
        """{"schemaVersion":1,"app":{"name":"t"},"initialScreen":"home","screens":[{"id":"home","components":[${components.joinToString(",")}]}]}""",
    )

    private val a = """{"id":"a","type":"text","text":"a","area":"A1:F2"}"""
    private val b = """{"id":"b","type":"button","label":"b","area":"A3:F4"}"""
    private val c = """{"id":"c","type":"list","area":"A5:F10"}"""

    private class Hints(var shown: Boolean = false) {
        val store = EditHintStore(read = { shown }, write = { shown = true })
    }

    private fun initial() = VoiceUiState(document = AppDocument("app1", 0, voiceOnly))

    @Test
    fun createShowsFullScreenThenRevealsWithEditHintOnce() {
        val hints = Hints()
        val s = BuildStageState(hints.store)
        val start = initial()
        s.update(start.copy(phase = VoicePhase.SENDING, transcript = "家計簿"), 1_000, reduceMotion = false)
        assertTrue(s.showsFullScreen)
        assertFalse(s.showsEditing)
        assertEquals("家計簿", s.job?.transcript)

        val progress = BuildProgress(components = listOf(StreamedComponent(1, "home", 0, id = "a", type = "text", area = "A1:F2")))
        s.update(start.copy(phase = VoicePhase.GENERATING, transcript = "家計簿", progress = progress), 2_000, reduceMotion = false)
        assertEquals(listOf("a"), s.live?.parts?.map { it.id })

        val done = start.copy(phase = VoicePhase.DONE, document = AppDocument("app1", 1, app(a, b)), summary = "右下のボタンから追加できます")
        s.update(done, 3_000, reduceMotion = false)
        assertNull(s.job)
        assertNotNull(s.reveal)
        val reveal = s.reveal!!
        assertTrue("組み上げの間は全画面のまま", s.showsFullScreen)
        assertEquals("仮置きで見せた部品は最初から置く", 0.0, reveal.plan.steps.first { it.part.id == "a" }.delay, 0.0)
        assertEquals(PositionHint.anchor("右下"), s.anchor)
        assertTrue(s.editHint)
        assertTrue(hints.shown)

        s.finishReveal(reveal.serial + 1)
        assertNotNull("別の組み上げの終わりでは消さない", s.reveal)
        s.finishReveal()
        assertFalse(s.showsFullScreen)

        // 2 つ目のアプリでは出さない
        val s2 = BuildStageState(hints.store)
        s2.update(start.copy(phase = VoicePhase.SENDING, transcript = "x"), 0, reduceMotion = false)
        s2.update(done, 1, reduceMotion = false)
        assertFalse(s2.editHint)
    }

    @Test
    fun editMarksChangedPartsWithoutReveal() {
        val s = BuildStageState()
        val before = VoiceUiState(document = AppDocument("app1", 1, app(a, b, c)), phase = VoicePhase.DONE)
        s.update(before, 0, reduceMotion = false)
        s.update(before.copy(phase = VoicePhase.SENDING, transcript = "ボタンを変えて"), 10, reduceMotion = false)
        assertTrue(s.showsEditing)
        assertFalse(s.showsFullScreen)
        val changed = b.replace("\"label\":\"b\"", "\"label\":\"保存\"")
        s.update(before.copy(document = AppDocument("app1", 2, app(a, changed, c)), summary = "右下のボタンの名前を変えました"), 20, reduceMotion = false)
        assertNull(s.reveal)
        assertEquals(setOf("home/b"), s.highlight.components)
        assertNull("印を出す時は位置のハイライトを出さない", s.anchor)
        val h = s.highlight
        s.clearHighlight(h)
        assertTrue(s.highlight.isEmpty)
    }

    @Test
    fun cancelAndFailureLeaveNothing() {
        val s = BuildStageState(Hints().store)
        s.update(initial().copy(phase = VoicePhase.SENDING, transcript = "x"), 0, reduceMotion = false)
        s.update(initial().copy(phase = VoicePhase.IDLE), 1, reduceMotion = false)
        assertNull(s.job)
        assertNull(s.reveal)
        assertFalse(s.showsFullScreen)
        assertFalse(s.editHint)
        s.update(initial().copy(phase = VoicePhase.SENDING, transcript = "x"), 2, reduceMotion = false)
        s.update(initial().copy(phase = VoicePhase.ERROR), 3, reduceMotion = false)
        assertFalse(s.showsFullScreen)
    }

    @Test
    fun chatIsNotGenerating() {
        val s = BuildStageState()
        s.update(initial().copy(phase = VoicePhase.SENDING, chatPending = true), 0, reduceMotion = false)
        assertNull("相談の往復では生成中の表示を出さない", s.job)
    }

    @Test
    fun noticesComeOneAtATime() {
        assertEquals(PostBuildNotice.SUMMARY, PostBuildNotice.current(setOf(PostBuildNotice.EDIT_HINT, PostBuildNotice.SUMMARY)))
        assertEquals(PostBuildNotice.PAYWALL, PostBuildNotice.current(PostBuildNotice.entries.toSet()))
        assertEquals(PostBuildNotice.EDIT_HINT, PostBuildNotice.current(setOf(PostBuildNotice.ACCOUNT_LINK, PostBuildNotice.EDIT_HINT)))
        assertNull(PostBuildNotice.current(emptySet()))

        val hints = Hints()
        val s = BuildStageState(hints.store)
        s.update(initial().copy(phase = VoicePhase.SENDING, transcript = "x"), 0, reduceMotion = false)
        val done = initial().copy(phase = VoicePhase.DONE, document = AppDocument("app1", 1, app(a)), summary = "できました")
        s.update(done, 1, reduceMotion = false)
        assertEquals("要約が先", PostBuildNotice.SUMMARY, s.currentNotice(done))
        assertEquals("要約が閉じたら直し方の例", PostBuildNotice.EDIT_HINT, s.currentNotice(done.copy(summary = null)))
        s.dismissEditHint()
        assertNull(s.currentNotice(done.copy(summary = null)))
    }

    @Test
    fun retentionWaitsForSummaryAndEditHintWaitsForRetention() {
        // iOS の順番: 要約 → 保存期間の確認 → 直し方の例(#1271)
        assertEquals(
            setOf(PostBuildNotice.SUMMARY, PostBuildNotice.RETENTION, PostBuildNotice.EDIT_HINT),
            PostBuildNotice.pending(summary = true, retention = true, editHint = true),
        )
        assertEquals(emptySet<PostBuildNotice>(), PostBuildNotice.pending(summary = false))

        val hints = Hints()
        val s = BuildStageState(hints.store)
        s.update(initial().copy(phase = VoicePhase.SENDING, transcript = "x"), 0, reduceMotion = false)
        val done = initial().copy(phase = VoicePhase.DONE, document = AppDocument("app1", 1, app(a)), summary = "できました")
        s.update(done, 1, reduceMotion = false)
        val closed = done.copy(summary = null)

        assertEquals("要約が先", PostBuildNotice.SUMMARY, s.currentNotice(done, retentionPending = true))
        assertFalse("要約の間は保存期間を出さない", BuildStageState.isRetentionTurn(done, retentionPending = true))
        assertEquals("要約が閉じたら保存期間", PostBuildNotice.RETENTION, s.currentNotice(closed, retentionPending = true))
        assertTrue(BuildStageState.isRetentionTurn(closed, retentionPending = true))
        assertFalse("確認が無ければ番は来ない", BuildStageState.isRetentionTurn(closed, retentionPending = false))
        assertEquals("確認が閉じたら直し方の例", PostBuildNotice.EDIT_HINT, s.currentNotice(closed, retentionPending = false))
    }

    @Test
    fun anchorClearsWhenSummaryCloses() {
        val s = BuildStageState()
        s.update(initial().copy(phase = VoicePhase.SENDING, transcript = "x"), 0, reduceMotion = false)
        val done = initial().copy(phase = VoicePhase.DONE, document = AppDocument("app1", 1, app(a)), summary = "上部に合計が出ます")
        s.update(done, 1, reduceMotion = false)
        assertNotNull(s.anchor)
        s.update(done.copy(summary = null), 2, reduceMotion = false)
        assertNull(s.anchor)
    }

    // MARK: 経過段階・位置語

    @Test
    fun stagesFollowElapsedTime() {
        assertEquals(GeneratingStage.NORMAL, GeneratingStage.of(0.0))
        assertEquals(GeneratingStage.SLOW, GeneratingStage.of(10.0))
        assertEquals(GeneratingStage.WARNING, GeneratingStage.of(25.0))
        assertEquals(GeneratingStage.FALLBACK, GeneratingStage.of(31.0))
        assertTrue(GeneratingStage.FALLBACK.cancelIsProminent)
        assertFalse(GeneratingStage.WARNING.cancelIsProminent)
        assertTrue(GeneratingStage.WARNING.warns)
        assertFalse(GeneratingStage.SLOW.warns)
        // 最初の段は 3 秒ごとに文言を回す・編集は「変更を反映しています」
        assertEquals(R.string.voice_gen_thinking, GeneratingStage.NORMAL.message(0.0, edit = false))
        assertEquals(R.string.voice_gen_deciding, GeneratingStage.NORMAL.message(3.5, edit = false))
        assertEquals(R.string.voice_gen_thinking, GeneratingStage.NORMAL.message(12.5, edit = false))
        assertEquals(R.string.voice_gen_editing, GeneratingStage.NORMAL.message(3.5, edit = true))
        assertEquals(R.string.voice_gen_usually, GeneratingStage.NORMAL.estimate(edit = false))
        assertNull(GeneratingStage.NORMAL.estimate(edit = true))
        assertNull(GeneratingStage.SLOW.estimate(edit = false))
    }

    @Test
    fun headingDropsNewAppKeyword() {
        assertEquals("家計簿", GeneratingStage.displayTranscript("別のアプリ。家計簿"))
        assertEquals("別のアプリ", GeneratingStage.displayTranscript("別のアプリ"))
        assertEquals("家計簿", GeneratingStage.displayTranscript("家計簿"))
    }

    @Test
    fun positionWordsMapToRegions() {
        assertEquals(PositionHint.Anchor(0.85f, 0.85f), PositionHint.anchor("右下のボタンから追加できます"))
        assertEquals("「右下」は「右」より先", PositionHint.Anchor(0.85f, 0.85f), PositionHint.anchor("右の方、右下にあります"))
        assertEquals(PositionHint.Anchor(0.5f, 0.88f), PositionHint.anchor("一番下に保存ボタン"))
        assertEquals(PositionHint.Anchor(0.15f, 0.15f), PositionHint.anchor("Tap the button at the Top Left"))
        assertNull(PositionHint.anchor("家計簿を作りました"))
    }
}
