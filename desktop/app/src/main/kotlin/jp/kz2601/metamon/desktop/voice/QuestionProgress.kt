package jp.kz2601.metamon.desktop.voice

import jp.kz2601.metamon.desktop.i18n.Strings

/**
 * 新しく作る時の聞き返しの「質問 n / 3」(iOS の `QuestionProgress`・Android の `QuestionProgress`・#3015)。
 * 最新の質問の吹き出しの上に小さく出し、あと何問かを見せる。数え方と文字だけで、画面は会話の画面ができてから繋ぐ。
 */
object QuestionProgress {
    /** backend `chatMaxTurns`(create の質問の上限)と同じ */
    const val MAX_QUESTIONS = 3

    /**
     * 今の質問が何問目か。create で、最新の吹き出しが待っている質問の時だけ(edit は上限が無いので出さない)。
     * [bubbleIsQuestion] は会話の吹き出しを古い順に並べた「AI の質問か」。
     * [latestIsQuestion] は直近の応答の種類が質問か。
     */
    fun number(bubbleIsQuestion: List<Boolean>, isCreate: Boolean, latestIsQuestion: Boolean): Int? {
        if (!isCreate || !latestIsQuestion || bubbleIsQuestion.lastOrNull() != true) return null
        return bubbleIsQuestion.count { it }.takeIf { it in 1..MAX_QUESTIONS }
    }

    /** 「質問 2 / 3」「Question 2 of 3」(Android と同じキー) */
    fun label(number: Int): String = Strings.format("voice_question_progress", number, MAX_QUESTIONS)

    /** 読み上げ。「質問 2、全部で 3」 */
    fun spoken(number: Int): String = Strings.format("desktop_question_progress_spoken", number, MAX_QUESTIONS)
}
