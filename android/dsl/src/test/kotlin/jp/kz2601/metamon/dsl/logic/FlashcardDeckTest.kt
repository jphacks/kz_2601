package jp.kz2601.metamon.dsl.logic

import jp.kz2601.metamon.dsl.records.Filter
import jp.kz2601.metamon.dsl.records.Fixtures
import jp.kz2601.metamon.dsl.records.Fixtures.str
import jp.kz2601.metamon.dsl.records.JSONValue
import jp.kz2601.metamon.dsl.records.Record
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/** iOS の FlashcardDeckTests: 覚えた / まだ の filter・並び・めくる・答える(保存の差分)・進み具合・完了・もう一度。 */
class FlashcardDeckTest {
    private val words = listOf(
        Record("w3", mapOf("term" to JSONValue.Str("consider")), createdAt = "2026-11-03T00:00:00Z"),
        Record("w1", mapOf("term" to JSONValue.Str("apple"), "learned" to JSONValue.Bool(false)), createdAt = "2026-11-01T00:00:00Z"),
        Record("w2", mapOf("term" to JSONValue.Str("borrow"), "learned" to JSONValue.Bool(true)), createdAt = "2026-11-02T00:00:00Z"),
    )

    @Test
    fun exampleHasFlashcards() {
        val c = Fixtures.load("vocab").allComponents.first { it.str("type") == "flashcards" }
        assertEquals("word", c.str("entity"))
        assertEquals("term", c.str("front"))
        assertEquals("meaning", c.str("back"))
        assertEquals("learned", c.str("toggle"))
        assertEquals(Filter("learned", "eq", JSONValue.Bool(false)), Filter.from(c["filter"]!!))
    }

    @Test
    fun learnedFilter() {
        assertEquals(false, FlashcardDeck.learnedFilter(Filter("learned", "eq", JSONValue.Bool(false)), "learned"))
        assertEquals(false, FlashcardDeck.learnedFilter(Filter("learned", "neq", JSONValue.Bool(true)), "learned"))
        assertEquals(true, FlashcardDeck.learnedFilter(Filter("learned", "eq", JSONValue.Bool(true)), "learned"))
        assertEquals(false, FlashcardDeck.learnedFilter(Filter("learned", "eq", JSONValue.Str("false")), "learned"))
        assertNull("toggle 以外の field", FlashcardDeck.learnedFilter(Filter("level", "eq", JSONValue.Bool(false)), "learned"))
        assertNull("toggle 無し", FlashcardDeck.learnedFilter(Filter("learned", "eq", JSONValue.Bool(false)), null))
        assertNull(FlashcardDeck.learnedFilter(Filter("learned", "gt", JSONValue.Bool(false)), "learned"))
    }

    @Test
    fun filterExcludesLearnedAndKeepsOrder() {
        assertEquals(listOf("w1", "w3"), FlashcardDeck.cards(words, "learned", false).map { it.id })
        assertEquals(listOf("w2"), FlashcardDeck.cards(words, "learned", true).map { it.id })
        assertEquals(listOf("w1", "w2", "w3"), FlashcardDeck.cards(words, "learned", null).map { it.id })
        assertEquals(listOf("w3"), FlashcardDeck.cards(words, "learned", false, mapOf("w1" to true)).map { it.id })
        assertTrue(FlashcardDeck.isLearned(Record("x", mapOf("learned" to JSONValue.Str("true"))), "learned"))
        assertTrue(FlashcardDeck.isLearned(Record("x", mapOf("learned" to JSONValue.Num(1.0))), "learned"))
    }

    @Test
    fun flip() {
        val deck = FlashcardDeck()
        assertFalse(deck.flipped)
        deck.flip()
        assertTrue(deck.flipped)
        deck.flip()
        assertFalse(deck.flipped)
    }

    @Test
    fun answerReturnsPatchAndAdvances() {
        val deck = FlashcardDeck()
        val cards = FlashcardDeck.cards(words, "learned", null)
        assertEquals("w1", deck.current(cards)?.id)
        deck.flip()
        assertEquals(mapOf("learned" to JSONValue.Bool(true)), deck.answer(cards[0], true, "learned"))
        assertFalse("次のカードは表から", deck.flipped)
        assertEquals(mapOf("w1" to true), deck.pending)
        assertEquals("w2", deck.current(cards)?.id)
        assertEquals(mapOf("learned" to JSONValue.Bool(false)), deck.answer(cards[1], false, "learned"))
        assertEquals(mapOf("w1" to true, "w2" to false), deck.pending)
        deck.settle("w1", true)
        deck.settle("w2", true) // 別の値を頼んだ結果は消さない
        assertEquals(mapOf("w2" to false), deck.pending)
        // toggle 無し = めくるだけ
        assertNull(deck.answer(cards[2], true, null))
        assertNull(deck.pending["w3"])
    }

    @Test
    fun progressDoneAndRestart() {
        var store = words
        val deck = FlashcardDeck()
        fun cards() = FlashcardDeck.cards(store, "learned", false, deck.pending)
        assertEquals(1 to 2, deck.progress(cards()))

        val first = deck.current(cards())!!
        assertEquals("w1", first.id)
        deck.answer(first, true, "learned")
        assertFalse("覚えたカードは山札から外れる", cards().any { it.id == "w1" })
        assertEquals(2 to 2, deck.progress(cards()))
        // 保存が終わってストアに反映
        store = store.map { if (it.id == "w1") it.copy(data = it.data + ("learned" to JSONValue.Bool(true))) else it }
        deck.settle("w1", true)
        assertFalse(cards().any { it.id == "w1" })

        val second = deck.current(cards())!!
        assertEquals("w3", second.id)
        assertFalse(deck.isDone(cards()))
        deck.answer(second, false, "learned")
        assertNull(deck.current(cards()))
        assertTrue(deck.isDone(cards()))
        assertEquals(2, deck.progress(cards()).first)

        deck.restart()
        assertFalse(deck.isDone(cards()))
        assertEquals("もう一度は覚えていないカードだけ", listOf("w3"), cards().map { it.id })
        assertEquals(1, deck.progress(cards()).second)
    }

    @Test
    fun emptyDeck() {
        val deck = FlashcardDeck()
        val cards = FlashcardDeck.cards(listOf(words[2]), "learned", false)
        assertTrue(cards.isEmpty())
        assertNull(deck.current(cards))
        assertFalse(deck.isDone(cards))
        assertEquals(0, deck.progress(cards).second)
    }
}
