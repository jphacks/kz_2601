package jp.kz2601.metamon.voice

import android.content.Context
import jp.kz2601.metamon.services.ChatMode
import jp.kz2601.metamon.services.ChatOption
import jp.kz2601.metamon.services.ChatPaidFeature
import jp.kz2601.metamon.services.ChatTurn

/** 複数選択(iOS の `MultiSelect`・#220): proposal の案 / question のチップをいくつでも選べる時。 */
object MultiSelect {
    /** 選んだ案の指示を 1 回の generate 用につなぐ。1 件ならそのまま、複数なら「・」の箇条書き(改行区切り)。 */
    fun joinedInstruction(instructions: List<String>): String {
        val items = instructions.map { it.trim() }.filter { it.isNotEmpty() }
        if (items.size <= 1) return items.firstOrNull().orEmpty()
        return items.joinToString("\n") { "・$it" }
    }

    /** 選んだチップを 1 つの答えにする(「、」区切り・押した順)。 */
    fun joinedAnswer(chips: List<String>): String = chips.map { it.trim() }.filter { it.isNotEmpty() }.joinToString("、")

    /** 選んだ案を 1 つにまとめる(並びは渡された順)。有料機能は和集合、無料版は各案の無料版(無ければ元の指示)。 */
    fun combined(options: List<ChatOption>): ChatOption? {
        val first = options.firstOrNull() ?: return null
        if (options.size == 1) return first
        val pro = options.any { it.requiresPro == true }
        val features = options.flatMap { it.paidFeatures.orEmpty() }.distinctBy(ChatPaidFeature::id)
        val free = options.map { if (it.requiresPro == true) it.freeInstruction.orEmpty() else it.instructionText }
        return ChatOption(
            id = options.joinToString("+") { it.optionId },
            title = options.joinToString("、") { it.title },
            instruction = joinedInstruction(options.map { it.instructionText }),
            requiresPro = if (pro) true else null,
            paidFeatures = if (pro) features else null,
            freeInstruction = if (pro) joinedInstruction(free) else null,
        )
    }
}

/** 新しく作る時の聞き返しは最大 3 回(iOS の `QuestionProgress`・M5)。最新の質問の下に「質問 n / 3」。 */
object QuestionProgress {
    /** backend `chatMaxTurns` と同じ */
    const val MAX_QUESTIONS = 3

    /** 今の質問が何問目か。create で、最新の吹き出しが待っている質問の時だけ(edit は上限が無いので出さない)。 */
    fun number(chat: List<ChatBubble>, mode: ChatMode, latest: ChatTurn.Kind?): Int? {
        val last = chat.lastOrNull() ?: return null
        if (mode != ChatMode.CREATE || latest != ChatTurn.Kind.QUESTION || last.fromUser || last.kind != ChatTurn.Kind.QUESTION) return null
        val n = chat.count { !it.fromUser && it.kind == ChatTurn.Kind.QUESTION }
        return n.takeIf { it in 1..MAX_QUESTIONS }
    }
}

/** 応答の下に出す副ボタン(iOS の `ClarifyFlow.Shortcut`)。 */
enum class ChatShortcut {
    NONE,

    /** create: 「このまま作る」 */
    BUILD_NOW,

    /** edit の聞き返し中: 「このまま反映」 */
    APPLY_NOW,
    ;

    companion object {
        fun of(turn: ChatTurn?, mode: ChatMode): ChatShortcut = when {
            turn?.kind == ChatTurn.Kind.READY -> NONE
            mode == ChatMode.CREATE -> BUILD_NOW
            // edit は聞き返し中だけ。答え・案の時に出すと「質問しただけなのに変わった」になる
            turn?.kind == ChatTurn.Kind.QUESTION -> APPLY_NOW
            else -> NONE
        }
    }
}

/**
 * 相談の設定(iOS の `ClarifySettings`・#75 / #186)。既定 ON。OFF は一発で反映(create も edit も)。
 * 自動で作る(#160)は声だけで新しく作っている時だけ。
 */
class ClarifySettings(context: Context) {
    private val prefs = context.applicationContext.getSharedPreferences(PREFS, Context.MODE_PRIVATE)

    var enabled: Boolean
        get() = prefs.getBoolean(KEY_ENABLED, true)
        set(value) = prefs.edit().putBoolean(KEY_ENABLED, value).apply()

    var autoBuild: Boolean
        get() = prefs.getBoolean(KEY_AUTO_BUILD, true)
        set(value) = prefs.edit().putBoolean(KEY_AUTO_BUILD, value).apply()

    /** 後片付け(#841)で既定(ON)に戻す。iOS は UserDefaults ごと消す。 */
    fun clear() = prefs.edit().clear().apply()

    private companion object {
        const val PREFS = "metamon.clarify"
        const val KEY_ENABLED = "enabled"
        const val KEY_AUTO_BUILD = "autoBuild"
    }
}
