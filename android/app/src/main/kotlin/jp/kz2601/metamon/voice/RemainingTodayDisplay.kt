package jp.kz2601.metamon.voice

import jp.kz2601.metamon.services.RemainingToday
import java.time.Instant

/**
 * 「今日の残り N」(iOS の `RemainingTodayDisplay`・#630)の出し分け。単位はサーバーの重みなので「回」と言わない。
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
