package jp.kz2601.metamon.feedback

/**
 * 上部のバナーの枠(iOS の `summaryBanner` は 1 枠で、後から来た方で置き換える)。
 * 変身の要約とホットリロードの「最新の内容に更新しました」は別々の状態から来るので、ここで 1 枠に並べる。
 *
 * - 出すのは 1 つだけ。後から出た方を出し、先に出ていた方は [Pick.superseded] で返す(呼ぶ側が閉じる)
 * - 同時に出た時は [Source] の並び順(要約が先)を優先する
 * - 同じ鍵のまま居続けるものは「新しく出た」と数えない
 */
class TopBannerQueue {
    /** 優先度の高い順 */
    enum class Source { SUMMARY, HOT_RELOAD }

    data class Pick(val shown: Source?, val superseded: Set<Source>)

    private val arrived = mutableMapOf<Source, Pair<Any, Long>>()
    private var seq = 0L

    /** [keys] は出す候補ごとの鍵(出していなければ null)。同じ鍵で何度呼んでも結果は変わらない。 */
    fun pick(keys: Map<Source, Any?>): Pick {
        val fresh = Source.entries.filter { s ->
            val key = keys[s]
            if (key == null) arrived.remove(s)
            key != null && arrived[s]?.first != key
        }
        val stamp = ++seq
        // 同時に来たものは優先度の低い順に小さい番号を振る(優先度の高い方が後に来たことにする)
        fresh.asReversed().forEachIndexed { i, s -> arrived[s] = keys.getValue(s)!! to stamp * Source.entries.size + i }
        val shown = arrived.maxByOrNull { it.value.second }?.key
        return Pick(shown, arrived.keys - setOfNotNull(shown))
    }
}

/** 上部のバナーを出しておく時間の元(iOS の summaryCard)。文字数に応じて 3〜5 秒、ボタン付きは 6 秒。 */
object TopBannerTiming {
    fun baseMillis(text: String, hasAction: Boolean): Long =
        if (hasAction) 6_000L else maxOf(3_000L, minOf(5_000L, text.length * 1_000L / 12))
}
