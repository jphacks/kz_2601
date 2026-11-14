package jp.kz2601.metamon.dsl.logic

import jp.kz2601.metamon.dsl.records.Record
import kotlin.random.Random

/**
 * `randomPicker`(#263)の選び方。iOS の Records/RandomPick.swift。
 * 乱数は Expr に入れない(描画のたびに値が変わる)。ボタンを押した時だけ呼び、結果の id を部品の状態に持つ。
 * 乱数源は [Random] で差し替える(テストは種を固定)。
 */
object RandomPick {
    /** 候補から 1 件。候補が 2 件以上なら `previous`(直前に選んだ id)は選ばない。候補が無ければ null。 */
    fun pick(records: List<Record>, previous: String?, random: Random = Random.Default): Record? {
        val pool = if (records.size > 1) records.filter { it.id != previous } else records
        return pool.randomOrNull(random)
    }

    /**
     * シャッフル演出のコマ(当選の前に順に見せる候補)。隣り合うコマ・最後のコマと当選が同じにならないよう並べる。
     * 候補が 1 件以下なら演出しない(空)。
     */
    fun reel(records: List<Record>, winner: Record, frames: Int, random: Random = Random.Default): List<Record> {
        if (records.size <= 1 || frames <= 0) return emptyList()
        val out = mutableListOf<Record>()
        var last = winner.id // 最後のコマ(= 当選の直前)が当選と同じだと止まって見える
        repeat(frames) {
            val next = records.filter { it.id != last }.randomOrNull(random) ?: winner
            out.add(next)
            last = next.id
        }
        // 当選側から遡って作ったので逆順にする
        return out.reversed()
    }
}
