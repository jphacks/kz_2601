package jp.kz2601.metamon.renderer.form

import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue

/**
 * 保存していない入力を失わせる操作(戻る・シートを閉じる・タブの切り替え)の前の確認(M10・iOS の `ScreenView.guardsDiscard`)。
 * スタックごとに今出ているフォームを控え、入力が変わっていれば [pending] に操作を預けて確認を出す。変わっていなければすぐ実行。
 */
class DiscardGuard {
    private val forms = mutableMapOf<String, FormDraft>()

    /** 「破棄」を押したら実行する操作。null なら確認は出ていない。 */
    var pending: (() -> Unit)? by mutableStateOf(null)
        private set

    fun track(stack: String, form: FormDraft) {
        forms[stack] = form
    }

    fun untrack(stack: String, form: FormDraft) {
        if (forms[stack] === form) forms.remove(stack)
    }

    /** [stack] の今の画面を離れる前に確認するか。 */
    fun needsConfirm(stack: String): Boolean = needsConfirm(forms[stack])

    /** [stack] の画面を離れる操作。確認が要れば預け、要らなければすぐ実行する。確認が出ている間の 2 回目は受けない。 */
    fun request(stack: String, action: () -> Unit) {
        if (!needsConfirm(stack)) return action()
        if (pending == null) pending = action
    }

    /** 複数のスタックをまとめて離れる操作(画面マップ: シート・全画面・今のタブ)。どれかに入力があれば確認する。 */
    fun request(stacks: Collection<String>, action: () -> Unit) {
        val stack = stacks.firstOrNull(::needsConfirm) ?: return action()
        request(stack, action)
    }

    /** 「破棄」: 預けた操作を実行する。 */
    fun discard() {
        val action = pending ?: return
        pending = null
        action()
    }

    /** 「編集を続ける」・窓の外を押した。 */
    fun keepEditing() {
        pending = null
    }

    companion object {
        /** 保存していない入力があるか(他人の記録=見るだけは false)。 */
        fun needsConfirm(form: FormDraft?): Boolean = form?.hasUnsavedChanges == true
    }
}
