package jp.kz2601.metamon.desktop.voice

import jp.kz2601.metamon.desktop.i18n.Strings
import jp.kz2601.metamon.services.consent.KeyValueStore

/**
 * 起動時の挨拶(Android の `voice/Greeting.kt`・iOS の `Greeting` の純粋な部分・#3019)。
 * 文言は Android の strings のキー(`voice_title_*`・`voice_subtitle_*`・`voice_hello_*`)をそのまま引く。
 */
object Greeting {
    enum class Context {
        /** 初回起動(まだ何も作っていない) */
        FIRST_VISIT,

        /** 2 回目以降の起動で初期画面 */
        RETURNING,

        /** 切り替えの「+ 新しいアプリ」 */
        NEW_APP,

        /** 最初の画面に戻った(アプリのリセット) */
        HOME,

        /** 既にアプリがある */
        EDITING,
    }

    /** 初期画面に来た経路 */
    enum class HomeEntry { LAUNCH, NEW_APP, HOME }

    /** 見出しとサブ文言のキー。[subtitleHasHello] の時は時間帯の挨拶を `%1$s` に入れる。 */
    data class Copy(val titleKey: String, val subtitleKey: String, val subtitleHasHello: Boolean)

    fun copy(context: Context): Copy = when (context) {
        Context.FIRST_VISIT -> Copy("voice_title_first", "voice_subtitle_first", true)
        Context.RETURNING -> Copy("voice_title_returning", "voice_subtitle_returning", true)
        Context.NEW_APP -> Copy("voice_title_new_app", "voice_subtitle_new_app", false)
        Context.HOME -> Copy("voice_title_home", "voice_subtitle_home", true)
        Context.EDITING -> Copy("voice_title_editing", "voice_subtitle_editing", false)
    }

    /** 初期画面でどの挨拶を出すか(来た経路と起動回数から・Android と同じ) */
    fun context(entry: HomeEntry, isInitial: Boolean, launchIsReturning: Boolean): Context {
        if (!isInitial) return Context.EDITING
        return when (entry) {
            HomeEntry.LAUNCH -> if (launchIsReturning) Context.RETURNING else Context.FIRST_VISIT
            HomeEntry.NEW_APP -> Context.NEW_APP
            HomeEntry.HOME -> Context.HOME
        }
    }

    /** 5〜10 時 おはよう / 11〜17 時 こんにちは / それ以外 こんばんは */
    fun timeOfDayKey(hour: Int): String = when (hour) {
        in 5..10 -> "voice_hello_morning"
        in 11..17 -> "voice_hello_day"
        else -> "voice_hello_evening"
    }

    /** 見出し・サブ文言を今の言語で作る。 */
    fun text(context: Context, hour: Int): Pair<String, String> {
        val copy = copy(context)
        val subtitle = if (copy.subtitleHasHello) Strings.format(copy.subtitleKey, Strings[timeOfDayKey(hour)]) else Strings[copy.subtitleKey]
        return Strings[copy.titleKey] to subtitle
    }
}

/**
 * この起動が再訪か(Android の `LaunchVisits`)。プロセスで最初に聞いた時に判定して訪問回数を 1 増やす。起動中は値が変わらない。
 * 回数は [store](`metamon.greeting` の鍵は Android と同じ名前)に残す。
 */
class LaunchVisits(private val store: KeyValueStore) {
    @Volatile
    private var cached: Boolean? = null

    fun isReturning(): Boolean = cached ?: synchronized(this) {
        cached ?: run {
            val count = store.getInt(KEY) ?: 0
            store.putInt(KEY, count + 1)
            note(count).also { cached = it }
        }
    }

    /** 訪問回数を消す(この起動の判定は変えない)。 */
    fun forget() = store.remove(KEY)

    companion object {
        const val KEY = "metamon.visitCount"

        /** それまでの訪問回数から再訪かを決める */
        fun note(previousVisits: Int): Boolean = previousVisits > 0
    }
}
