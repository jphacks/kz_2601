package jp.kz2601.metamon.dsl.logic

import jp.kz2601.metamon.dsl.logic.BracketDraw.Side
import jp.kz2601.metamon.dsl.logic.BracketDraw.TapResult
import jp.kz2601.metamon.dsl.records.Fixtures
import jp.kz2601.metamon.dsl.records.Fixtures.str
import jp.kz2601.metamon.dsl.records.JSONValue
import jp.kz2601.metamon.dsl.records.Record
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotEquals
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/** iOS の BracketDrawTests: 組み合わせ(シード順・不戦勝)・勝ち上がり・勝者のタップ・組み合わせの固定。 */
class BracketDrawTest {
    private fun day(d: Int) = "2026-11-%02dT00:00:00Z".format(d)

    private fun p(id: String, wins: Int? = null, seed: Double? = null, at: Int): Record {
        val data = linkedMapOf<String, JSONValue>("name" to JSONValue.Str(id))
        if (wins != null) data["wins"] = JSONValue.Num(wins.toDouble())
        if (seed != null) data["seed"] = JSONValue.Num(seed)
        return Record(id, data, createdAt = day(at))
    }

    private fun ids(m: BracketDraw.Match) = listOf(m.a?.id, m.b?.id)

    private fun draw(rs: List<Record>, seedField: String? = null, startedField: String? = null, pending: Map<String, Int> = emptyMap()) =
        BracketDraw(rs, "wins", seedField, startedField, pending)

    private fun update(vararg pairs: Pair<String, Int>) = TapResult.Update(mapOf(*pairs))

    @Test
    fun slotSeedsKeepTopSeedsApart() {
        assertEquals(listOf(1, 2), BracketDraw.slotSeeds(2))
        assertEquals(listOf(1, 4, 2, 3), BracketDraw.slotSeeds(4))
        assertEquals(listOf(1, 8, 4, 5, 2, 7, 3, 6), BracketDraw.slotSeeds(8))
    }

    @Test
    fun fewerThanTwoEntrantsHasNoRounds() {
        assertTrue(draw(emptyList()).rounds.isEmpty())
        val one = draw(listOf(p("a", at = 1)))
        assertTrue(one.rounds.isEmpty())
        assertEquals(listOf("a"), one.entrants.map { it.id })
        assertNull(one.champion)
    }

    @Test
    fun byesGoToTopSeedsInCreationOrder() {
        val rs = listOf(p("e", at = 5), p("a", at = 1), p("c", at = 3), p("b", at = 2), p("d", at = 4))
        val d = draw(rs)
        assertEquals(3, d.rounds.size)
        assertEquals(listOf(listOf("a", null), listOf("d", "e"), listOf("b", null), listOf("c", null)), d.rounds[0].map(::ids))
        assertEquals(listOf(true, false, true, true), d.rounds[0].map { it.bye })
        assertEquals(listOf(Side.A, null, Side.A, Side.A), d.rounds[0].map { it.winner })
        assertEquals(listOf(listOf("a", null), listOf("b", "c")), d.rounds[1].map(::ids))
        assertFalse(d.rounds[1][0].isPlayable)
        assertTrue(d.rounds[1][1].isPlayable)
        assertNull(d.champion)
    }

    @Test
    fun seedFieldOrdersEntrants() {
        val rs = listOf(p("a", at = 1), p("b", seed = 2.0, at = 2), p("c", seed = 1.0, at = 3), p("d", at = 4))
        assertEquals(listOf("c", "b", "a", "d"), BracketDraw.seeded(rs, "seed").map { it.id })
        assertEquals(listOf(listOf("c", "d"), listOf("b", "a")), draw(rs, "seed").rounds[0].map(::ids))
    }

    @Test
    fun winsAdvanceToChampion() {
        val rs = listOf(p("a", wins = 2, at = 1), p("b", wins = 0, at = 2), p("c", wins = 1, at = 3), p("d", at = 4))
        val d = draw(rs)
        assertEquals(listOf(listOf("a", "d"), listOf("b", "c")), d.rounds[0].map(::ids))
        assertEquals(listOf(Side.A, Side.B), d.rounds[0].map { it.winner })
        assertEquals(listOf(listOf("a", "c")), d.rounds[1].map(::ids))
        assertEquals("a", d.champion?.id)
        assertEquals(2, d.rounds.size)
    }

    @Test
    fun onlySixtyFourEntrantsAreDrawn() {
        val d = draw((1..70).map { p("p$it", at = 1) })
        assertEquals(BracketDraw.MAX_ENTRANTS, d.entrants.size)
        assertEquals(6, d.rounds.size)
    }

    @Test
    fun seedPositionsAndByesForManySizes() {
        for (n in listOf(2, 3, 5, 7, 8, 9, 16, 33, 64)) {
            val d = draw((1..n).map { p("p%02d".format(it), at = 1) })
            var size = 2
            while (size < n) size *= 2
            fun seed(r: Record?) = r?.id?.drop(1)?.toInt()
            assertEquals("n=$n", size / 2, d.rounds[0].size)
            assertEquals("n=$n", Integer.numberOfTrailingZeros(size), d.rounds.size)
            val byes = mutableListOf<Int>()
            for (m in d.rounds[0]) {
                val a = seed(m.a)
                val b = seed(m.b)
                assertNotNull("n=$n: the top slot always has an entrant", a)
                if (a != null && b != null) assertEquals("n=$n", size + 1, a + b) else if (a != null) byes.add(a)
            }
            assertEquals("n=$n", if (size > n) (1..(size - n)).toList() else emptyList(), byes.sorted())
            if (size < 4) continue
            assertTrue("n=$n", d.rounds[0].take(size / 4).any { seed(it.a) == 1 })
            assertTrue("n=$n", d.rounds[0].takeLast(size / 4).any { seed(it.a) == 2 })
        }
    }

    @Test
    fun tiesGoToTheLargerWinsThenTheTopSlot() {
        assertEquals(Side.A, four(mapOf("a" to 1, "d" to 1)).rounds[0][0].winner)
        assertEquals(Side.B, four(mapOf("a" to 1, "d" to 2)).rounds[0][0].winner)
    }

    // MARK: - タップ

    private fun four(wins: Map<String, Int> = emptyMap()) =
        draw(listOf("a", "b", "c", "d").mapIndexed { i, id -> p(id, wins = wins[id], at = i + 1) })

    @Test
    fun tapSetsWinnerAndSwapsBack() {
        val m = four().rounds[0][0] // a-d
        assertEquals(update("d" to 1), four().tap(m, Side.B, "wins"))
        val dd = four(mapOf("d" to 1))
        val decided = dd.rounds[0][0]
        assertEquals(Side.B, decided.winner)
        assertEquals(update("a" to 1, "d" to 0), dd.tap(decided, Side.A, "wins"))
        assertEquals(update("d" to 0), dd.tap(decided, Side.B, "wins"))
    }

    @Test
    fun tapUsesPendingWins() {
        val m = four().rounds[0][0]
        assertEquals(update("a" to 1, "d" to 0), four().tap(m, Side.A, "wins", mapOf("d" to 1)))
        assertEquals(Side.B, draw(four().entrants, pending = mapOf("d" to 1)).rounds[0][0].winner)
    }

    @Test
    fun lockedAndNotPlayable() {
        val d = four(mapOf("a" to 2, "b" to 1))
        assertEquals(TapResult.Locked, d.tap(d.rounds[0][0], Side.B, "wins"))
        assertEquals(TapResult.Locked, d.tap(d.rounds[0][0], Side.A, "wins"))
        assertEquals(update("a" to 1), d.tap(d.rounds[1][0], Side.A, "wins"))

        val undecided = four().rounds[1][0]
        assertEquals(TapResult.NotPlayable, four().tap(undecided, Side.A, "wins"))
        val three = draw(listOf("a", "b", "c").mapIndexed { i, id -> p(id, at = i + 1) })
        val bye = three.rounds[0][0]
        assertTrue(bye.bye)
        assertEquals(TapResult.NotPlayable, three.tap(bye, Side.A, "wins"))
    }

    @Test
    fun decidedNextMatchLocksEvenWhenTheFinalistLost() {
        val d = four(mapOf("a" to 1, "b" to 2))
        assertEquals(Side.B, d.rounds[1][0].winner)
        assertEquals(TapResult.Locked, d.tap(d.rounds[0][0], Side.B, "wins"))
        assertEquals(TapResult.Locked, d.tap(d.rounds[0][0], Side.A, "wins"))
        assertEquals(update("b" to 1), d.tap(d.rounds[1][0], Side.B, "wins"))
    }

    @Test
    fun overstatedWinsCanStillBeUndone() {
        val d = four(mapOf("a" to 9, "b" to 1))
        assertEquals("a", d.champion?.id)
        assertEquals(update("a" to 1), d.tap(d.rounds[1][0], Side.A, "wins"))
        val e = four(mapOf("a" to 3))
        assertEquals(update("a" to 0), e.tap(e.rounds[0][0], Side.A, "wins"))
        assertEquals(update("d" to 1, "a" to 0), e.tap(e.rounds[0][0], Side.B, "wins"))
    }

    @Test
    fun undoWhenBothHaveWinsClearsTheMatch() {
        val d = four(mapOf("a" to 1, "d" to 2))
        val m = d.rounds[0][0]
        assertEquals(Side.B, m.winner)
        assertEquals(update("d" to 0, "a" to 0), d.tap(m, Side.B, "wins"))
        assertNull(four(mapOf("a" to 0, "d" to 0)).rounds[0][0].winner)
        val pending = mapOf("a" to 1, "d" to 1)
        val pd = draw(four().entrants, pending = pending)
        assertEquals(Side.A, pd.rounds[0][0].winner)
        assertEquals(update("a" to 0, "d" to 0), pd.tap(pd.rounds[0][0], Side.A, "wins", pending))
        val locked = four(mapOf("a" to 1, "d" to 2, "b" to 1))
        assertEquals(Side.A, locked.rounds[1][0].winner)
        assertEquals(TapResult.Locked, locked.tap(locked.rounds[0][0], Side.B, "wins"))
    }

    @Test
    fun hugeWinsDoNotTrap() {
        assertEquals(BracketDraw.MAX_WINS, BracketDraw.wins(Record("a", mapOf("wins" to JSONValue.Num(1e20))), "wins"))
        assertEquals(0, BracketDraw.wins(Record("b", mapOf("wins" to JSONValue.Num(-1e20))), "wins"))
        val d = draw(listOf(p("x", at = 1), Record("y", mapOf("wins" to JSONValue.Num(1e300)), createdAt = day(2))))
        assertEquals("y", d.champion?.id)
    }

    @Test
    fun tournamentExampleHasBracket() {
        val c = Fixtures.load("tournament").allComponents.first { it.str("type") == "bracket" }
        assertEquals("wins", c.str("winsField"))
        assertEquals("seed", c.str("seedField"))
        assertEquals("name", c.str("titleField"))
    }

    // MARK: - 組み合わせの固定

    private fun q(id: String, wins: Int = 0, seed: Double? = null, at: Int, updated: Int? = null): Record =
        p(id, wins, seed, at).copy(updatedAt = day(updated ?: at))

    private fun pairs(d: BracketDraw) = d.rounds.firstOrNull()?.map(::ids) ?: emptyList()

    @Test
    fun beforeAnyResultAddingEntrantRebuildsDraw() {
        val four = listOf(q("a", at = 1), q("b", at = 2), q("c", at = 3), q("d", at = 4))
        val before = draw(four)
        val after = draw(four + q("e", at = 6))
        assertEquals(5, after.entrants.size)
        assertEquals(0, after.lateEntrants)
        assertNotEquals("結果が無いうちは組み直す", pairs(before), pairs(after))
    }

    @Test
    fun afterFirstResultLateEntrantIsNotPlacedAndCounted() {
        val four = listOf(q("a", wins = 1, at = 1, updated = 5), q("b", at = 2), q("c", at = 3), q("d", at = 4))
        val frozen = draw(four, "seed")
        val late = draw(four + q("e", seed = 1.0, at = 6), "seed")
        assertEquals(frozen.entrants.map { it.id }, late.entrants.map { it.id })
        assertEquals(frozen.rounds, late.rounds)
        assertEquals(1, late.lateEntrants)
        assertEquals(0, frozen.lateEntrants)
        assertEquals(listOf(listOf("a", "d"), listOf("b", "c")), pairs(late))
        assertEquals(Side.A, late.rounds[0][0].winner)
    }

    @Test
    fun removingLateEntrantKeepsFrozenDraw() {
        val four = listOf(q("a", wins = 1, at = 1, updated = 5), q("b", at = 2), q("c", at = 3), q("d", at = 4))
        val withLate = draw(four + listOf(q("e", at = 6), q("f", at = 7)))
        val removed = draw(four + q("f", at = 7))
        assertEquals(withLate.rounds, removed.rounds)
        assertEquals(2, withLate.lateEntrants)
        assertEquals(1, removed.lateEntrants)
    }

    @Test
    fun undoingAllResultsUnfreezes() {
        val d = draw(listOf(q("a", at = 1, updated = 8), q("b", at = 2), q("c", at = 3), q("d", at = 4), q("e", at = 6)))
        assertEquals(5, d.entrants.size)
        assertEquals(0, d.lateEntrants)
    }

    @Test
    fun freezeIgnoresPendingAndMissingTimestamps() {
        val four = listOf(q("a", at = 1), q("b", at = 2), q("c", at = 3), q("d", at = 4))
        assertEquals(0, draw(four + q("e", at = 6), pending = mapOf("a" to 1)).lateEntrants)
        assertEquals(3, draw(listOf(p("a", wins = 1, at = 1), p("b", at = 2), p("c", at = 6))).entrants.size)
    }

    // MARK: - startedField

    private fun s(id: String, wins: Int = 0, at: Int, updated: Int? = null, started: Int? = null): Record {
        val r = q(id, wins, at = at, updated = updated)
        return if (started == null) r else r.copy(data = r.data + ("started" to JSONValue.Str(day(started))))
    }

    @Test
    fun startedFieldFreezesExactlyWhenFirstWinnerAdvances() {
        val rs = listOf(
            s("a", wins = 2, at = 1, updated = 8, started = 5), s("b", wins = 1, at = 2, updated = 7, started = 5),
            s("c", at = 3, started = 5), s("d", at = 4, started = 5), s("e", at = 6),
        )
        val exact = draw(rs, startedField = "started")
        assertEquals(listOf("a", "b", "c", "d"), exact.entrants.map { it.id })
        assertEquals(1, exact.lateEntrants)
        assertEquals(listOf(listOf("a", "d"), listOf("b", "c")), pairs(exact))
        assertEquals("a", exact.champion?.id)
        val fallback = draw(rs.map { it.copy(data = it.data - "started") }, startedField = "started")
        assertEquals(5, fallback.entrants.size)
    }

    @Test
    fun startedFieldKeepsStampedEntrantsEvenIfCreatedLaterByClock() {
        val rs = listOf(s("a", wins = 1, at = 1, updated = 5, started = 3), s("b", at = 4, started = 3), s("e", at = 6))
        val d = draw(rs, startedField = "started")
        assertEquals(listOf("a", "b"), d.entrants.map { it.id })
        assertEquals(1, d.lateEntrants)
    }

    @Test
    fun startedFieldIgnoredWithoutResults() {
        val rs = listOf(s("a", at = 1, started = 5), s("b", at = 2, started = 5), s("e", at = 6))
        assertEquals(0, draw(rs, startedField = "started").lateEntrants)
    }

    @Test
    fun startWritesOnFirstResultAndUndoAll() {
        val fresh = listOf(s("a", at = 1), s("b", at = 2), s("x", at = 3, started = 1))
        val d = draw(fresh, startedField = "started")
        val first = (d.tap(d.rounds[0][1], Side.A, "wins") as TapResult.Update).wins
        val w = d.startWrites(first, fresh, "wins", "started", now = day(9))
        assertEquals("枠の全員に開始時刻", 3, w.size)
        assertTrue(w.values.all { it == JSONValue.Str(day(9)) })
        assertTrue(d.startWrites(first, fresh, "wins", null, now = day(9)).isEmpty())

        val started = listOf(s("a", wins = 1, at = 1, updated = 9, started = 9), s("b", at = 2, started = 9), s("c", at = 3, started = 9), s("d", at = 4, started = 9))
        val sd = draw(started, startedField = "started")
        val second = (sd.tap(sd.rounds[0][1], Side.A, "wins") as TapResult.Update).wins
        assertTrue(sd.startWrites(second, started, "wins", "started", now = day(10)).isEmpty())
        val undo = (sd.tap(sd.rounds[0][0], Side.A, "wins") as TapResult.Update).wins
        assertEquals(mapOf("a" to 0), undo)
        val clear = sd.startWrites(undo, started, "wins", "started", now = day(10))
        assertEquals(setOf("a", "b", "c", "d"), clear.keys)
        assertTrue(clear.values.all { it.isNull })
    }

    @Test
    fun lateEntrantWithFormDefaultStartedIsExcluded() {
        val rs = listOf(s("a", wins = 1, at = 1, updated = 3, started = 3), s("b", at = 2, started = 3), s("e", at = 6, started = 6))
        val d = draw(rs, startedField = "started")
        assertEquals(listOf("a", "b"), d.entrants.map { it.id })
        assertEquals(1, d.lateEntrants)
    }

    @Test
    fun stampedEntrantCreatedAfterStartIsKeptWithinTolerance() {
        val b = s("b", at = 4).let { it.copy(data = it.data + ("started" to JSONValue.Str("2026-11-03T00:00:00.500Z"))) }
        val rs = listOf(s("a", wins = 1, at = 1, updated = 5, started = 3), b, s("e", at = 6, started = 6))
        val d = draw(rs, startedField = "started")
        assertEquals(listOf("a", "b"), d.entrants.map { it.id })
        assertEquals(1, d.lateEntrants)
    }

    @Test
    fun lateFormStartedDoesNotOverrideTimestampRuleForOldDraws() {
        val rs = listOf(s("a", wins = 1, at = 1, updated = 3), s("b", at = 2), s("e", at = 6, started = 6))
        val d = draw(rs, startedField = "started")
        assertEquals(listOf("a", "b"), d.entrants.map { it.id })
        assertEquals(1, d.lateEntrants)
    }
}
