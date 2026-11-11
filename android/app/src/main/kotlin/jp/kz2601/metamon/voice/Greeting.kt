package jp.kz2601.metamon.voice

import androidx.annotation.StringRes
import jp.kz2601.metamon.R

/** 音声の画面の問いかけと例文(iOS の `Greeting`)。初回 / 再訪などの出し分けは android-057 の残り。 */
object Greeting {
    data class ExampleCategory(val id: String, @StringRes val title: Int, val examples: List<Int>)

    /** 記録・習慣・お金・家族/友達と・ゲーム(#274) */
    val createCategories: List<ExampleCategory> = listOf(
        ExampleCategory("record", R.string.voice_cat_record, listOf(R.string.voice_ex_workout, R.string.voice_ex_walk, R.string.voice_ex_books)),
        ExampleCategory("habit", R.string.voice_cat_habit, listOf(R.string.voice_ex_water, R.string.voice_ex_medicine, R.string.voice_ex_early)),
        ExampleCategory("money", R.string.voice_cat_money, listOf(R.string.voice_ex_budget, R.string.voice_ex_allowance, R.string.voice_ex_subs)),
        ExampleCategory("together", R.string.voice_cat_together, listOf(R.string.voice_ex_split, R.string.voice_ex_attendance, R.string.voice_ex_board)),
        ExampleCategory("game", R.string.voice_cat_game, listOf(R.string.voice_ex_boardgame, R.string.voice_ex_bowling)),
    )

    val editExamples: List<Int> = listOf(R.string.voice_ex_dark, R.string.voice_ex_graph, R.string.voice_ex_memo)

    /** 5〜10 時 おはよう / 11〜17 時 こんにちは / それ以外 こんばんは(iOS と同じ) */
    @StringRes
    fun timeOfDay(hour: Int): Int = when (hour) {
        in 5..10 -> R.string.voice_hello_morning
        in 11..17 -> R.string.voice_hello_day
        else -> R.string.voice_hello_evening
    }
}
