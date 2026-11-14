package jp.kz2601.metamon.renderer.components.actions

import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import jp.kz2601.metamon.dsl.Action
import jp.kz2601.metamon.dsl.ActionType

/**
 * button の実行中(#1089・iOS の `ButtonLeaf.running` / `showsProgress` / `begin`)。
 * 通信を待つアクションの間はくるくるを出し、終わるまで次の押下を受けない。
 */
class ButtonBusy {
    var running by mutableStateOf(false)
        private set

    /** 押した時に同期で呼ぶ。実行中なら false(二度押し)。待つアクションは最初の suspend より前に実行中にする。 */
    fun begin(action: Action): Boolean {
        if (running) return false
        if (waits(action)) running = true
        return true
    }

    fun end() {
        running = false
    }

    companion object {
        /** 押してから終わるまで待つアクション(iOS の `showsProgress`)。 */
        fun waits(action: Action?): Boolean = action?.type == ActionType.LOOKUP_BOOK
    }
}
