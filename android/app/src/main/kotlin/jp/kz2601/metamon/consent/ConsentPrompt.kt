package jp.kz2601.metamon.consent

import jp.kz2601.metamon.services.consent.AiConsentStore
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow

/**
 * 同意の画面を出す時点(iOS の `AppRuntime+Consent` / `ConsentHost`・#888)。
 * - 起動時: まだ説明を出していない人と、版が上がった人に 1 回だけ([showOnLaunchIfNeeded])
 * - AI に送る直前: 未同意なら出して送らない([require])。同意して閉じたら、止めた 1 件を続けて送る
 * - 設定から([show])
 */
class ConsentPrompt(private val store: AiConsentStore) {
    private val _visible = MutableStateFlow(false)
    val visible: StateFlow<Boolean> = _visible.asStateFlow()

    /** 同意の前に止めた 1 件(M2: 話し直さなくてよい)。「あとで」で閉じたら捨てる */
    private var awaiting: (() -> Unit)? = null

    val isGranted: Boolean get() = store.isGranted

    /** AI に送る直前に呼ぶ。同意済みなら true。未同意なら画面を出して false(送らない)。 */
    fun require(resume: (() -> Unit)? = null): Boolean {
        if (store.isGranted) return true
        resume?.let { awaiting = it }
        _visible.value = true
        return false
    }

    fun showOnLaunchIfNeeded() {
        if (store.shouldPromptOnLaunch) _visible.value = true
    }

    fun show() {
        _visible.value = true
    }

    /** 「同意して使う」 */
    fun agree() {
        store.grant()
        close()?.invoke()
    }

    /** 「あとで」・戻る。後で設定から同意した時に、古い発話を勝手に送らない */
    fun later() {
        store.markPrompted()
        close()
    }

    private fun close(): (() -> Unit)? {
        _visible.value = false
        return awaiting.also { awaiting = null }
    }
}
