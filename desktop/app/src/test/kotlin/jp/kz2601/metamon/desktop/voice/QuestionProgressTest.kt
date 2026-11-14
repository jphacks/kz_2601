package jp.kz2601.metamon.desktop.voice

import jp.kz2601.metamon.desktop.i18n.Lang
import jp.kz2601.metamon.desktop.i18n.Strings
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

/** 「質問 n / 3」の数え方と文字(日本語・英語)・#3015。 */
class QuestionProgressTest {
    // 古い順に「AI の質問か」。false はユーザーの答え・案など
    private val asked1 = listOf(false, true)
    private val asked2 = listOf(false, true, false, true)

    @Test
    fun countsAssistantQuestionsWhileWaiting() {
        assertEquals(1, QuestionProgress.number(asked1, isCreate = true, latestIsQuestion = true))
        assertEquals(2, QuestionProgress.number(asked2, isCreate = true, latestIsQuestion = true))
    }

    @Test
    fun hiddenOutsideWaitingQuestion() {
        assertNull(QuestionProgress.number(asked1, isCreate = false, latestIsQuestion = true))
        assertNull(QuestionProgress.number(asked1 + false, isCreate = true, latestIsQuestion = true))
        assertNull(QuestionProgress.number(asked1, isCreate = true, latestIsQuestion = false))
        assertNull(QuestionProgress.number(emptyList(), isCreate = true, latestIsQuestion = true))
    }

    @Test
    fun hiddenBeyondMax() {
        val four = listOf(false, true, false, true, false, true, false, true)
        assertNull(QuestionProgress.number(four, isCreate = true, latestIsQuestion = true))
        assertEquals(3, QuestionProgress.number(four.dropLast(2), isCreate = true, latestIsQuestion = true))
    }

    @Test
    fun labelInBothLanguages() {
        Strings.withLang(Lang.JA) {
            assertEquals("質問 2 / 3", QuestionProgress.label(2))
            assertEquals("質問 2、全部で 3", QuestionProgress.spoken(2))
        }
        Strings.withLang(Lang.EN) {
            assertEquals("Question 2 of 3", QuestionProgress.label(2))
            assertEquals("Question 2, out of 3", QuestionProgress.spoken(2))
        }
    }
}
