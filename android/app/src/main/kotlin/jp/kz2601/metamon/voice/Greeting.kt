package jp.kz2601.metamon.voice

import android.content.Context
import androidx.annotation.StringRes
import jp.kz2601.metamon.R

/** 音声の画面の問いかけと例文(iOS の `Greeting`)。初回・再訪・新しいアプリ・既存アプリの改変で出し分ける。 */
object Greeting {
    /** iOS の `Greeting.Context` */
    enum class Context {
        /** 初回起動(まだ何も作っていない) */
        FIRST_VISIT,

        /** 2 回目以降の起動で初期画面 */
        RETURNING,

        /** 切り替えの「+ 新しいアプリ」 */
        NEW_APP,

        /** 最初の画面に戻った(アプリのリセット) */
        HOME,

        /** 既にアプリがある(FAB からの音声シート) */
        EDITING,
    }

    /** 初期画面に来た経路(iOS の `AppRuntime.HomeEntry`) */
    enum class HomeEntry { LAUNCH, NEW_APP, HOME }

    /**
     * 見出しとサブ文言。[subtitle] が [subtitleHasHello] の時は時間帯の挨拶([timeOfDay])を `%1$s` に入れる。
     */
    data class Copy(@StringRes val title: Int, @StringRes val subtitle: Int, val subtitleHasHello: Boolean)

    fun copy(context: Context): Copy = when (context) {
        Context.FIRST_VISIT -> Copy(R.string.voice_title_first, R.string.voice_subtitle_first, true)
        Context.RETURNING -> Copy(R.string.voice_title_returning, R.string.voice_subtitle_returning, true)
        Context.NEW_APP -> Copy(R.string.voice_title_new_app, R.string.voice_subtitle_new_app, false)
        Context.HOME -> Copy(R.string.voice_title_home, R.string.voice_subtitle_home, true)
        Context.EDITING -> Copy(R.string.voice_title_editing, R.string.voice_subtitle_editing, false)
    }

    /** 初期画面でどの挨拶を出すか(来た経路と起動回数から・iOS と同じ) */
    fun context(entry: HomeEntry, isInitial: Boolean, launchIsReturning: Boolean): Context {
        if (!isInitial) return Context.EDITING
        return when (entry) {
            HomeEntry.LAUNCH -> if (launchIsReturning) Context.RETURNING else Context.FIRST_VISIT
            HomeEntry.NEW_APP -> Context.NEW_APP
            HomeEntry.HOME -> Context.HOME
        }
    }

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

/**
 * この起動が再訪か(iOS の `Greeting.launchIsReturning`)。プロセスで最初に聞いた時に判定して訪問回数を 1 増やす。
 * 起動中は値が変わらない。
 */
object LaunchVisits {
    private const val PREFS = "metamon.greeting"
    private const val KEY = "metamon.visitCount"

    @Volatile
    private var cached: Boolean? = null

    fun isReturning(context: Context): Boolean = cached ?: synchronized(this) {
        cached ?: run {
            val prefs = context.applicationContext.getSharedPreferences(PREFS, Context.MODE_PRIVATE)
            val count = prefs.getInt(KEY, 0)
            prefs.edit().putInt(KEY, count + 1).apply()
            note(count).also { cached = it }
        }
    }

    /**
     * 訪問回数を消す(#841・iOS の `LocalWipe` も `metamon.visitCount` を消す)。
     * この起動の判定(cached)は iOS の `launchIsReturning` と同じく変えない。
     */
    fun forget(context: Context) {
        context.applicationContext.getSharedPreferences(PREFS, Context.MODE_PRIVATE).edit().clear().apply()
    }

    /** それまでの訪問回数から再訪かを決める */
    internal fun note(previousVisits: Int): Boolean = previousVisits > 0
}
