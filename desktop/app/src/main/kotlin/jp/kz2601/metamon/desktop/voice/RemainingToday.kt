package jp.kz2601.metamon.desktop.voice

import jp.kz2601.metamon.services.RemainingToday
import java.time.Instant

/**
 * 「今日の残り N」(#3008・Android の `RemainingTodayDisplay` と同じ規則)の出し分け。単位はサーバーの重みなので「回」と言わない。
 * 会話の画面が入力欄の横に小さく出す。
 */
sealed interface RemainingTodayDisplay {
    /** まだ分からない・日付が変わって古い */
    data object Hidden : RemainingTodayDisplay

    data class Normal(val remaining: Int, val pro: Boolean) : RemainingTodayDisplay

    /** 少ない([LOW_THRESHOLD] 以下)。目立たせる */
    data class Low(val remaining: Int, val pro: Boolean) : RemainingTodayDisplay

    /** 0。理由と戻る時刻は上限のカードが出す */
    data class Exhausted(val resetAt: Instant?) : RemainingTodayDisplay

    companion object {
        /** 新しく作るのがあと 2 回分を切ったら少ない扱い */
        const val LOW_THRESHOLD = RemainingToday.Weight.CREATE * 2

        fun of(value: RemainingToday?, now: Instant, isPro: Boolean): RemainingTodayDisplay = when {
            value == null || value.isStale(now) -> Hidden
            value.remaining <= 0 -> Exhausted(value.resetAt)
            value.remaining <= LOW_THRESHOLD -> Low(value.remaining, isPro)
            else -> Normal(value.remaining, isPro)
        }
    }
}

/** 文言の引き方(キー・書式の引数 → 文)。画面は `Strings.format` を渡す。 */
typealias RemainingText = (key: String, args: Array<Any?>) -> String

/**
 * 入力欄の横に出す小さな文。文言は Android のキー(`voice_remaining_*`)をそのまま引く。
 * 出さない時([RemainingTodayDisplay.Hidden]・0 の時は上限のカードが出す)は null。
 */
fun RemainingTodayDisplay.label(text: RemainingText): String? = when (this) {
    RemainingTodayDisplay.Hidden, is RemainingTodayDisplay.Exhausted -> null
    is RemainingTodayDisplay.Normal -> text("voice_remaining_today", arrayOf(remaining))
    is RemainingTodayDisplay.Low -> text("voice_remaining_today", arrayOf(remaining))
}

/** 読み上げ用。少ない時は「少なくなっています」、Pro の時は先に「Pro」を付ける。 */
fun RemainingTodayDisplay.a11y(text: RemainingText): String? {
    val base = label(text) ?: return null
    val pro = (this as? RemainingTodayDisplay.Normal)?.pro ?: (this as? RemainingTodayDisplay.Low)?.pro ?: false
    val low = if (this is RemainingTodayDisplay.Low) text("voice_remaining_a11y_low", arrayOf(base)) else base
    return if (pro) text("voice_remaining_a11y_pro", arrayOf(low)) else low
}
