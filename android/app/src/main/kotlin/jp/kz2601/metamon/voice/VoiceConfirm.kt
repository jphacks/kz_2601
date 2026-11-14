package jp.kz2601.metamon.voice

/**
 * 認識器が無音で自分から止まった時の扱い(#1014)。iOS はタップで止めるまで聞くので、Android は途中で切れた依頼をそのまま送らない:
 * - タップで止めた → これまでの分とつないで送る(iOS と同じ)
 * - 自分で止まった → 送らずに「送る / 続けて話す / 言い直す」を出す([VoiceUiState.pendingVoice])
 * - 「続けて話す」の後に聞き取れなかった → 前の分を確認に戻す(話した分を捨てない)
 */
object VoiceConfirm {
    sealed interface Outcome {
        data class Send(val text: String) : Outcome
        data class Confirm(val text: String) : Outcome
    }

    /** 文字になった後。[carried] は「続けて話す」の前に聞き取った分。 */
    fun afterTranscribe(autoStopped: Boolean, carried: String?, text: String): Outcome {
        val joined = join(carried, text)
        return if (autoStopped) Outcome.Confirm(joined) else Outcome.Send(joined)
    }

    /** 文字にできなかった後。前の分があれば確認に戻す(null = いつものエラー)。 */
    fun afterFailure(carried: String?): Outcome? = carried?.trim()?.takeIf { it.isNotEmpty() }?.let { Outcome.Confirm(it) }

    /** 前の分とつなぐ。英数字どうしの境目だけ空白を入れる(日本語は詰める)。 */
    fun join(carried: String?, text: String): String {
        val a = carried?.trim().orEmpty()
        val b = text.trim()
        if (a.isEmpty()) return b
        if (b.isEmpty()) return a
        val spaced = a.last().isAsciiLetterOrDigit() && b.first().isAsciiLetterOrDigit() ||
            a.last() in ".,!?" && b.first().isAsciiLetterOrDigit()
        return if (spaced) "$a $b" else a + b
    }

    private fun Char.isAsciiLetterOrDigit(): Boolean = this in 'a'..'z' || this in 'A'..'Z' || this in '0'..'9'
}
