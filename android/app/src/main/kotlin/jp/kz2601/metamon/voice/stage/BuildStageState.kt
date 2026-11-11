package jp.kz2601.metamon.voice.stage

import androidx.compose.runtime.Stable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import jp.kz2601.metamon.dsl.AppDocument
import jp.kz2601.metamon.dsl.isVoiceOnly
import jp.kz2601.metamon.dsl.logic.ChangeHighlight
import jp.kz2601.metamon.voice.BuildProgress
import jp.kz2601.metamon.voice.VoicePhase
import jp.kz2601.metamon.voice.VoiceUiState

/**
 * 生成中から完成の直後までの見せ方(#894・iOS の `AppRuntime+BuildStage`)。[VoiceUiState] の移り変わりを見て決める。
 * - create(初期画面から): 全画面の組み立て台 → 結果の組み上げ([reveal])→ 本物の画面
 * - edit(2 回目以降): 上部の小さなカプセル → 変わった部品の印([highlight])
 * - 完成の後: 要約の位置語のハイライト([anchor])と、最初のアプリの直後の「こう話すと直せます」([editHint])
 * ViewModel を太らせないよう、画面の側(Compose)で持つ。
 */
@Stable
class BuildStageState(private val editHints: EditHintStore? = null) {
    /** 生成の 1 回分。[before] は送った時の版 */
    data class Job(val transcript: String, val startedAtMillis: Long, val before: AppDocument, val create: Boolean)

    /** create の結果の組み上げ。演出の間は全画面の生成中の表示を続ける */
    data class Reveal(val job: Job, val plan: BuildChoreography.AssemblyPlan, val serial: Int)

    var job: Job? by mutableStateOf(null)
        private set

    /** 生成中に届いた途中経過(完成時に仮置きで見せた部品を組み上げに引き継ぐ) */
    var live: BuildChoreography.LiveDraft? by mutableStateOf(null)
        private set

    var reveal: Reveal? by mutableStateOf(null)
        private set

    var highlight: ChangeHighlight by mutableStateOf(ChangeHighlight.NONE)
        private set

    /** 要約の位置語の場所(変わった部品の印を出す時は出さない・iOS と同じ) */
    var anchor: PositionHint.Anchor? by mutableStateOf(null)
        private set

    /** 「こう話すと直せます」を待っている */
    var editHint: Boolean by mutableStateOf(false)
        private set

    private var lastProgress: BuildProgress? = null
    private var serial = 0

    /** 全画面の生成中(待っている間・組み上げ中) */
    val showsFullScreen: Boolean get() = job?.create == true || reveal != null

    /** 編集の待ち(上部のカプセル) */
    val showsEditing: Boolean get() = job?.create == false

    /** 状態が変わるたびに呼ぶ。 */
    fun update(state: VoiceUiState, nowMillis: Long, reduceMotion: Boolean) {
        if (state.isGenerating) {
            if (job == null) {
                job = Job(state.transcript, nowMillis, state.document, create = state.isInitial)
                reveal = null
                highlight = ChangeHighlight.NONE
                anchor = null
                lastProgress = null
                live = null
            }
            state.progress?.let {
                if (it != lastProgress) {
                    lastProgress = it
                    live = BuildChoreography.LiveDraft(it)
                }
            }
            return
        }
        if (state.summary == null) anchor = null
        val j = job ?: return
        job = null
        val shown = live?.parts.orEmpty()
        live = null
        lastProgress = null
        // 取り消し・失敗・変わらなかった時は何も出さない
        if (state.phase != VoicePhase.DONE || state.document == j.before || state.isInitial) return
        if (j.create) {
            reveal = Reveal(j, BuildChoreography.AssemblyPlan.make(state.document.dsl, reduceMotion, shown), ++serial)
            anchor = state.summary?.let(PositionHint::anchor)
            if (editHints?.shouldShow(createdFromInitial = true) == true) {
                editHints.markShown()
                editHint = true
            }
            return
        }
        if (j.before.appId == state.document.appId && !j.before.dsl.isVoiceOnly) {
            highlight = ChangeHighlight.between(j.before.dsl, state.document.dsl)
        }
        anchor = if (highlight.isEmpty) state.summary?.let(PositionHint::anchor) else null
    }

    /** 組み上げを終える(時間切れ・タップで飛ばす)。[serial] が違えば(次の組み上げが来た後)何もしない。 */
    fun finishReveal(serial: Int? = null) {
        val r = reveal ?: return
        if (serial == null || r.serial == serial) reveal = null
    }

    /** 印は出現が済んだ頃に消す(後で画面を開き直した時に光らせ直さない) */
    fun clearHighlight(h: ChangeHighlight) {
        if (highlight == h) highlight = ChangeHighlight.NONE
    }

    fun dismissEditHint() {
        editHint = false
    }

    /**
     * いま出すお知らせ(課金画面 → 要約 → 公開の記録 → 保存期間の確認 → 直し方の例 → 引き継ぎ。前のが閉じたら次)。
     * @param retentionPending 今のアプリの保存期間の確認が待っている(#1271)
     * @param waiting 課金画面・公開の記録・引き継ぎ(#1198)
     */
    fun currentNotice(
        state: VoiceUiState,
        retentionPending: Boolean = false,
        waiting: PostBuildWaiting = PostBuildWaiting.NONE,
    ): PostBuildNotice? = PostBuildNotice.current(
        PostBuildNotice.pending(summary = summaryPending(state), retention = retentionPending, editHint = editHint, waiting = waiting),
    )

    companion object {
        /** 要約バナー(「元に戻す」を含む)が出ている */
        fun summaryPending(state: VoiceUiState): Boolean = state.summary != null || state.undone != null

        /**
         * 保存期間の確認の番か(iOS の `isNoticeTurn(.retention)`)。前にあるのは課金画面・要約・公開の記録。
         * 直し方の例・引き継ぎは後ろなので見ない(確認が閉じるまで後ろの方が待つ)。
         */
        fun isRetentionTurn(state: VoiceUiState, retentionPending: Boolean, waiting: PostBuildWaiting = PostBuildWaiting.NONE): Boolean =
            PostBuildNotice.current(PostBuildNotice.pending(summary = summaryPending(state), retention = retentionPending, waiting = waiting)) ==
                PostBuildNotice.RETENTION
    }
}
