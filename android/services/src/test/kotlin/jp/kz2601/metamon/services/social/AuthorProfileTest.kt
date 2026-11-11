package jp.kz2601.metamon.services.social

import jp.kz2601.metamon.services.consent.InMemoryKeyValueStore
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/** 投稿に出す名前(iOS の AuthorProfile・#250 #875)。 */
class AuthorProfileTest {
    @Test
    fun unsetNameIsNullAndNotAsked() {
        val p = AuthorProfile(InMemoryKeyValueStore())
        assertNull(p.name)
        assertFalse(p.asked)
    }

    @Test
    fun nameIsTrimmedAndCappedAt20() {
        val p = AuthorProfile(InMemoryKeyValueStore())
        p.name = "  たろう  "
        assertEquals("たろう", p.name)
        p.name = "a".repeat(30)
        assertEquals("a".repeat(20), p.name)
        p.name = "   "
        assertNull(p.name)
    }

    @Test
    fun answerMarksAskedAndAnonymousKeepsName() {
        val p = AuthorProfile(InMemoryKeyValueStore())
        p.answer(null)
        assertTrue(p.asked)
        assertNull(p.name)
        p.answer("はなこ")
        assertEquals("はなこ", p.name)
        p.answer(null) // 匿名で投稿しても決めた名前は消さない
        assertEquals("はなこ", p.name)
    }

    @Test
    fun clearForgetsNameAndAsked() {
        val p = AuthorProfile(InMemoryKeyValueStore())
        p.answer("はなこ")
        p.clear()
        assertNull(p.name)
        assertFalse(p.asked)
    }
}
