package jp.kz2601.metamon.renderer.screen

import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import jp.kz2601.metamon.renderer.form.FormDraft
import jp.kz2601.metamon.renderer.form.FormSaveResult
import jp.kz2601.metamon.renderer.theme.RenderTheme
import kotlinx.coroutines.delay

/** 保存バーの表示(iOS の `FormSaveBar.Phase`)。 */
sealed interface SaveBarPhase {
    data object Idle : SaveBarPhase

    /** 押した後・書き終わるまで(スピナー・2 回目は押せない) */
    data object Saving : SaveBarPhase

    /** 書けた: 「保存しました」/ オフラインなら「接続が戻ったら自動で送ります」 */
    data class Done(val result: FormSaveResult) : SaveBarPhase
}

/**
 * 保存バーを押した後の移り変わり(iOS の `FormSaveBar.run`)。画面を描かずに決めるので JVM で確かめられる。
 * 押す → [SaveBarPhase.Saving] → 書けたら [SaveBarPhase.Done] を [RenderTheme.Motion.CHECK_MILLIS] だけ → [SaveBarPhase.Idle]。
 * 書かなかった(必須が空・変更なし・確認待ち・連打)なら何も出さずに戻る。
 */
class SaveBarFeedback(private val hold: suspend () -> Unit = { delay(RenderTheme.Motion.CHECK_MILLIS.toLong()) }) {
    var phase: SaveBarPhase by mutableStateOf(SaveBarPhase.Idle)
        private set

    /** 押した回ごとの番号。断られて先に戻った後の次の保存の表示を、前の回の時間切れで消さない。 */
    private var round = 0

    /** [save] は保存のアクションを最後まで待つ。出した結果を返す(何も出さなければ null)。 */
    suspend fun press(form: FormDraft?, save: suspend () -> Unit): FormSaveResult? {
        if (phase != SaveBarPhase.Idle) return null
        val mine = ++round
        phase = SaveBarPhase.Saving
        val before = form?.saveCount
        val result = try {
            save()
            // 書いた直後にサーバーが断っていたら(lastSaveResult が null)出さない
            FormDraft.saveFeedback(before, form)
        } finally {
            if (round == mine && phase == SaveBarPhase.Saving) phase = SaveBarPhase.Idle
        }
        if (result == null || round != mine) return null
        phase = SaveBarPhase.Done(result)
        hold()
        if (round == mine && phase is SaveBarPhase.Done) phase = SaveBarPhase.Idle
        return result
    }

    /** 下書きの [FormDraft.lastSaveResult] が変わった時。null = 送った後にサーバーが断った: 「保存しました」を取り消す。 */
    fun onSaveResultChanged(result: FormSaveResult?) {
        if (result == null && phase is SaveBarPhase.Done) {
            round++
            phase = SaveBarPhase.Idle
        }
    }
}
