package jp.kz2601.metamon.dsl.logic

import jp.kz2601.metamon.dsl.records.Filter
import jp.kz2601.metamon.dsl.records.JSONValue
import jp.kz2601.metamon.dsl.records.Record

/**
 * `flashcards`(#264)の山札の状態と計算。iOS の Records/FlashcardDeck.swift。
 * どこまで進んだかは部品の表示状態だけに持ち、保存しない。保存するのは toggle の field(覚えた / まだ)だけ。
 */
class FlashcardDeck {
    /** この回で答えたカードの id(答えた順)。 */
    var answered: List<String> = emptyList()
        private set

    /** 今のカードの裏を見ているか。 */
    var flipped: Boolean = false
        private set

    /** 保存待ちの「覚えた / まだ」(楽観的 UI。保存の結果が返ったら [settle] で外す)。 */
    var pending: Map<String, Boolean> = emptyMap()
        private set

    /** まだ答えていない最初のカード。全部答えたら null(完了)。 */
    fun current(cards: List<Record>): Record? = cards.firstOrNull { it.id !in answered }

    /** 「n / 全体」。全体 = 答えたカード + 残り(覚えて山札から外れたカードも答えた数に入る)。 */
    fun progress(cards: List<Record>): Pair<Int, Int> {
        val remaining = cards.count { it.id !in answered }
        val total = answered.size + remaining
        return minOf(answered.size + 1, total) to total
    }

    /** 1 枚以上あって全部答えた(完了)か。最初から 0 枚の時は false(空の表示)。 */
    fun isDone(cards: List<Record>): Boolean = answered.isNotEmpty() && current(cards) == null

    fun flip() {
        flipped = !flipped
    }

    /** 今のカードに答えて次へ。toggle があれば保存する差分(`{toggle: bool}`)を返し、保存待ちに積む。無ければ null(めくるだけ)。 */
    fun answer(card: Record, learned: Boolean, toggle: String?): Map<String, JSONValue>? {
        if (card.id !in answered) answered = answered + card.id
        flipped = false
        if (toggle.isNullOrEmpty()) return null
        pending = pending + (card.id to learned)
        return mapOf(toggle to JSONValue.Bool(learned))
    }

    /** 保存の結果が返った時。その後に別の答えをしていたら(もう一度の回など)その表示は残す。 */
    fun settle(id: String, requested: Boolean) {
        if (pending[id] == requested) pending = pending - id
    }

    /** もう一度(答えた記録を消して最初から)。保存待ちはそのまま。 */
    fun restart() {
        answered = emptyList()
        flipped = false
    }

    companion object {
        /** toggle の field を boolean で比べる filter(eq / neq と true / false)なら、出すカードが覚えたもの(true)かまだ(false)か。 */
        fun learnedFilter(f: Filter, toggle: String?): Boolean? {
            if (toggle.isNullOrEmpty() || f.field != toggle || f.valueExpr != null) return null
            val b = when (val v = f.value) {
                is JSONValue.Bool -> v.value
                JSONValue.Str("true") -> true
                JSONValue.Str("false") -> false
                else -> return null
            }
            return when (f.op) {
                "eq" -> b
                "neq" -> !b
                else -> null
            }
        }

        /** 覚えたか(保存待ちを優先)。値の無いカードは「まだ」。bool 以外("true" / 1)も寛容に読む。 */
        fun isLearned(r: Record, toggle: String, pending: Map<String, Boolean> = emptyMap()): Boolean {
            pending[r.id]?.let { return it }
            return when (val v = r[toggle]) {
                is JSONValue.Bool -> v.value
                is JSONValue.Num -> v.value != 0.0
                is JSONValue.Str -> v.value == "true"
                else -> false
            }
        }

        /** 出題するカード。`learned` が非 null なら覚えた / まだ で絞る。並びは登録順(createdAt の昇順・同じなら id)。 */
        fun cards(records: List<Record>, toggle: String?, learned: Boolean?, pending: Map<String, Boolean> = emptyMap()): List<Record> {
            var out = records
            if (learned != null && toggle != null) out = out.filter { isLearned(it, toggle, pending) == learned }
            return out.sortedWith { a, b ->
                val x = a.createdAt ?: ""
                val y = b.createdAt ?: ""
                if (x == y) a.id.compareTo(b.id) else x.compareTo(y)
            }
        }
    }
}
