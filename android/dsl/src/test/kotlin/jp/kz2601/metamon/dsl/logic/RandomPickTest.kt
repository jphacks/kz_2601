package jp.kz2601.metamon.dsl.logic

import jp.kz2601.metamon.dsl.records.Fixtures
import jp.kz2601.metamon.dsl.records.Fixtures.str
import jp.kz2601.metamon.dsl.records.JSONValue
import jp.kz2601.metamon.dsl.records.Record
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotEquals
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Test
import kotlin.random.Random

/** iOS の RandomPickTests: 選び方(直前と同じものを続けて出さない・空・1 件)・演出のコマ。乱数は種を固定。 */
class RandomPickTest {
    private fun records(n: Int) = (1..n).map { Record("r$it", mapOf("name" to JSONValue.Str("料理$it"))) }

    @Test
    fun exampleHasRandomPicker() {
        val picker = Fixtures.load("dinner").allComponents.first { it.str("type") == "randomPicker" }
        assertEquals("dish", picker.str("entity"))
        assertEquals("name", picker.str("titleField"))
        assertEquals("genre", picker.str("subtitleField"))
        assertEquals("今日の晩ごはんを決める", picker.str("label"))
    }

    @Test
    fun emptyAndSingle() {
        val g = Random(1)
        assertNull(RandomPick.pick(emptyList(), null, g))
        val one = records(1)
        // 1 件しか無ければ直前と同じでも選ぶ
        assertEquals("r1", RandomPick.pick(one, "r1", g)?.id)
        assertEquals(emptyList<Record>(), RandomPick.reel(one, one[0], 9, g))
    }

    @Test
    fun neverRepeatsPreviousAndCoversAll() {
        val g = Random(42)
        val rs = records(4)
        var previous: String? = null
        val seen = mutableSetOf<String>()
        repeat(200) {
            val r = RandomPick.pick(rs, previous, g)
            assertNotNull(r)
            assertNotEquals("直前と同じ 1 件が続いた", previous, r?.id)
            previous = r?.id
            seen.add(r?.id ?: "")
        }
        assertEquals("選ばれない候補がある", setOf("r1", "r2", "r3", "r4"), seen)
    }

    @Test
    fun twoCandidatesAlternate() {
        val g = Random(7)
        val rs = records(2)
        var previous = "r1"
        repeat(10) {
            val r = RandomPick.pick(rs, previous, g)
            assertEquals(if (previous == "r1") "r2" else "r1", r?.id)
            previous = r?.id ?: ""
        }
    }

    @Test
    fun reelEndsBeforeWinnerWithoutStalling() {
        val g = Random(3)
        val rs = records(3)
        repeat(50) {
            val winner = rs[g.nextInt(0, 3)]
            val frames = RandomPick.reel(rs, winner, 9, g)
            assertEquals(9, frames.size)
            assertNotEquals("最後のコマが当選と同じだと止まって見える", winner.id, frames.last().id)
            for ((a, b) in frames.zipWithNext()) assertNotEquals("同じコマが続いた", a.id, b.id)
        }
    }
}
