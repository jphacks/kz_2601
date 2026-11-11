package jp.kz2601.metamon.dsl.logic

import jp.kz2601.metamon.dsl.records.FieldCoding
import jp.kz2601.metamon.dsl.records.JSONValue
import jp.kz2601.metamon.dsl.records.Record
import java.time.Duration
import java.time.Instant

/**
 * `bracket`(トーナメント表)の組み合わせと勝ち上がり。iOS の Records/BracketDraw.swift。
 * 組み合わせは保存しない: 出場者を seedField の昇順(値の無い人は後ろ)→ 登録順に並べ、2 のべき乗の枠に標準のシード配置で入れる。
 * 勝ち上がりは winsField(勝った回戦の数)だけで決まる。結果が 1 つでも付いたら組み合わせを固定する([frozen])。
 *
 * @param pending 保存待ちの勝ち数(楽観的 UI)。保存値より優先する。
 * @param startedField 開始時刻の datetime field([frozen])。
 */
class BracketDraw(
    records: List<Record>,
    winsField: String,
    private val seedField: String?,
    startedField: String? = null,
    pending: Map<String, Int> = emptyMap(),
) {
    enum class Side { A, B }

    data class Match(
        /** 1 始まりの回戦 */
        val round: Int,
        val index: Int,
        /** 上の枠・下の枠。1 回戦の null は不戦勝の空き枠、2 回戦からの null は前の試合がまだ決まっていない。 */
        val a: Record?,
        val b: Record?,
        val winner: Side?,
        /** 1 回戦の不戦勝(相手の枠が空き)。 */
        val bye: Boolean,
    ) {
        val winnerRecord: Record?
            get() = when (winner) {
                Side.A -> a
                Side.B -> b
                null -> null
            }

        /** 勝者を選べる試合か(2 人がそろい、不戦勝でない)。 */
        val isPlayable: Boolean get() = a != null && b != null && !bye
    }

    sealed interface TapResult {
        /** 保存する勝ち数(recordId → wins) */
        data class Update(val wins: Map<String, Int>) : TapResult
        /** 選べない試合(不戦勝・相手が未定) */
        data object NotPlayable : TapResult
        /** 先の回戦に進んだ人の試合(先の結果を取り消すまで変えられない) */
        data object Locked : TapResult
    }

    /** 回戦ごとの試合(rounds[0] = 1 回戦)。出場者が 2 人未満なら空。 */
    val rounds: List<List<Match>>

    /** 枠に入れた出場者(シード順)。 */
    val entrants: List<Record>

    /** 開始後に追加されたため枠に入れなかった人数。 */
    val lateEntrants: Int

    val champion: Record? get() = rounds.lastOrNull()?.firstOrNull()?.winnerRecord

    init {
        val (pool, late) = frozen(records, winsField, startedField)
        entrants = seeded(pool, seedField).take(MAX_ENTRANTS)
        lateEntrants = late
        rounds = if (entrants.size < 2) emptyList() else build(entrants) { wins(it, winsField, pending) }
    }

    private fun build(entrants: List<Record>, wins: (Record) -> Int): List<List<Match>> {
        var size = 2
        while (size < entrants.size) size *= 2
        val slots = slotSeeds(size).map { if (it <= entrants.size) entrants[it - 1] else null }
        val out = mutableListOf<List<Match>>()
        out.add(
            (0 until size / 2).map { i ->
                val a = slots[2 * i]
                val b = slots[2 * i + 1]
                val bye = (a == null) != (b == null)
                val winner = if (bye) (if (a == null) Side.B else Side.A) else winner(a, b, 1, wins)
                Match(1, i, a, b, winner, bye)
            },
        )
        var round = 1
        while (out[round - 1].size > 1) {
            round += 1
            val prev = out[round - 2]
            val r = round
            out.add(
                (0 until prev.size / 2).map { i ->
                    val a = prev[2 * i].winnerRecord
                    val b = prev[2 * i + 1].winnerRecord
                    Match(r, i, a, b, winner(a, b, r, wins), false)
                },
            )
        }
        return out
    }

    /** 試合の勝者が上がった先の試合に結果が付いているか(決勝は先が無いので false)。 */
    fun nextDecided(match: Match): Boolean {
        if (match.round >= rounds.size) return false
        val next = rounds[match.round]
        val i = match.index / 2
        return i < next.size && next[i].winner != null
    }

    /** 試合の side の人をタップした時(この表の先の試合を見て止める)。 */
    fun tap(match: Match, side: Side, winsField: String, pending: Map<String, Int> = emptyMap()): TapResult =
        tap(match, side, winsField, pending, nextDecided(match))

    /**
     * 勝ち数の書き込み(tap の結果)と一緒に送る startedField の値(recordId → 値)。
     * 結果が 0 → 1 つになる時: 登録済みの全員(シード順に最大 [MAX_ENTRANTS])に now、ほかで値の残っている人は null。
     * 結果が 1 → 0 になる時(全部取り消し): 値のある人を null。
     */
    fun startWrites(
        update: Map<String, Int>,
        records: List<Record>,
        winsField: String,
        startedField: String?,
        pending: Map<String, Int> = emptyMap(),
        now: String,
    ): Map<String, JSONValue> {
        if (startedField.isNullOrEmpty()) return emptyMap()
        val before = records.any { wins(it, winsField, pending) >= 1 }
        val after = records.any { (update[it.id] ?: wins(it, winsField, pending)) >= 1 }
        val out = linkedMapOf<String, JSONValue>()
        if (before == after) return out
        for (r in records) if (!r[startedField].isNull) out[r.id] = JSONValue.Null
        if (after) for (r in seeded(records, seedField).take(MAX_ENTRANTS)) out[r.id] = JSONValue.Str(now)
        return out
    }

    companion object {
        /** 描く出場者の上限(6 回戦)。Go の `dsl.MaxBracketEntrants` と同じ。 */
        const val MAX_ENTRANTS = 64

        /** 読む wins の上限。 */
        const val MAX_WINS = 1_000

        /** 同じ開始時刻とみなす幅(秒)。 */
        internal const val START_TOLERANCE_SECONDS = 1.0

        private fun date(s: String?): Instant? = s?.let { FieldCoding.strictDatetime(it) }

        /**
         * 開始後に追加された人を外す。保存済みの wins >= 1 の人がいる時だけ固定する。wins >= 1 の人は常に残す。
         * - startedField に値がある: 勝者の最も早い値を開始とし、それと同じ値(±1 秒)の人と createdAt がそれ以前の人を残す。
         * - 値が無い(古いアプリ): wins >= 1 の人の updatedAt の最も早い時刻を開始とし、createdAt がそれ以前の人を残す。
         * 時刻が読めない時は全員。保存待ち(pending)は見ない。
         */
        fun frozen(records: List<Record>, winsField: String, startedField: String? = null): Pair<List<Record>, Int> {
            val winners = records.filter { wins(it, winsField) >= 1 }
            if (winners.isEmpty()) return records to 0
            val field = startedField?.takeIf { it.isNotEmpty() }
            val times: List<Pair<String, Instant>> =
                field?.let { f -> records.mapNotNull { r -> date(r[f].stringValue)?.let { r.id to it } } } ?: emptyList()
            val winnerIds = winners.map { it.id }.toSet()
            val earliest = times.filter { it.first in winnerIds }.minOfOrNull { it.second }
            val stampedIds = earliest?.let { e ->
                times.filter { seconds(Duration.between(e, it.second)) <= START_TOLERANCE_SECONDS }.map { it.first }.toSet()
            } ?: emptySet()
            val started = earliest ?: winners.mapNotNull { date(it.updatedAt) }.minOrNull() ?: return records to 0
            val kept = records.filter { r ->
                if (wins(r, winsField) >= 1 || r.id in stampedIds) return@filter true
                val created = date(r.createdAt) ?: return@filter true
                created <= started
            }
            return kept to (records.size - kept.size)
        }

        private fun seconds(d: Duration): Double = d.seconds + d.nano / 1e9

        /** seedField の昇順(数値の無い人は後ろ)→ 登録順(createdAt の昇順・同じなら id)。 */
        fun seeded(records: List<Record>, seedField: String?): List<Record> = records.sortedWith { x, y ->
            if (!seedField.isNullOrEmpty()) {
                val sx = x[seedField].doubleValue
                val sy = y[seedField].doubleValue
                if (sx != sy) {
                    return@sortedWith when {
                        sx != null && sy != null -> sx.compareTo(sy)
                        sx != null -> -1
                        else -> 1
                    }
                }
            }
            val cx = x.createdAt ?: ""
            val cy = y.createdAt ?: ""
            if (cx == cy) x.id.compareTo(y.id) else cx.compareTo(cy)
        }

        /** 枠の順のシード番号(1 始まり)。8 枠なら [1, 8, 4, 5, 2, 7, 3, 6]。 */
        fun slotSeeds(size: Int): List<Int> {
            var seeds = listOf(1)
            while (seeds.size < size) {
                val m = seeds.size * 2
                seeds = seeds.flatMap { listOf(it, m + 1 - it) }
            }
            return seeds
        }

        /** 勝った回戦の数。保存待ちを優先し、数値でない値は 0。 */
        fun wins(r: Record, field: String, pending: Map<String, Int> = emptyMap()): Int {
            pending[r.id]?.let { return it }
            return when (val v = r[field]) {
                is JSONValue.Num -> if (v.value.isFinite()) v.value.coerceIn(0.0, MAX_WINS.toDouble()).toInt() else 0
                is JSONValue.Str -> maxOf(0, v.value.toIntOrNull() ?: 0)
                else -> 0
            }
        }

        /** r 回戦の勝者: wins >= r の方。両方なら値の大きい方、同じなら上の枠。 */
        internal fun winner(a: Record?, b: Record?, round: Int, wins: (Record) -> Int): Side? {
            if (a == null || b == null) return null
            val wa = wins(a)
            val wb = wins(b)
            return when {
                wa >= round && wb >= round -> if (wb > wa) Side.B else Side.A
                wa >= round -> Side.A
                wb >= round -> Side.B
                else -> null
            }
        }

        /**
         * 試合の side の人をタップした時。勝者でなければ勝者に(相手が勝者なら戻す)、勝者なら取り消す(相手も wins >= r なら相手も r-1)。
         * 先の試合(nextDecided)に結果が付いていれば変えない。
         */
        fun tap(match: Match, side: Side, winsField: String, pending: Map<String, Int> = emptyMap(), nextDecided: Boolean): TapResult {
            val a = match.a
            val b = match.b
            if (!match.isPlayable || a == null || b == null) return TapResult.NotPlayable
            val (me, other) = if (side == Side.A) a to b else b to a
            val r = match.round
            if (match.winner != null && nextDecided) return TapResult.Locked
            val wo = wins(other, winsField, pending)
            val out = linkedMapOf(me.id to if (match.winner == side) r - 1 else r)
            if (wo >= r) out[other.id] = r - 1
            return TapResult.Update(out)
        }
    }
}
