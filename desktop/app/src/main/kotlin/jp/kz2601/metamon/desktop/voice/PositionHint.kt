package jp.kz2601.metamon.desktop.voice

/**
 * 生成の要約(「右下のボタンから◯◯を追加できます」など)に含まれる位置の言葉を、ウィンドウ上の点に変える(#2983)。
 * iOS の `PositionHint`・Android の `voice/stage/PositionHint.kt` と同じ規則。
 * 部品の特定はせず、領域(四隅・上下・左右)の枠を一瞬出すだけに使う。
 */
object PositionHint {
    /** ウィンドウの幅・高さに対する割合(0〜1) */
    data class Anchor(val x: Float, val y: Float)

    /** 枠を出しておく時間 */
    const val HIGHLIGHT_MILLIS = 1000L

    private val TABLE: List<Pair<String, Anchor>> = listOf(
        "右下" to Anchor(0.85f, 0.85f),
        "左下" to Anchor(0.15f, 0.85f),
        "右上" to Anchor(0.85f, 0.15f),
        "左上" to Anchor(0.15f, 0.15f),
        "一番下" to Anchor(0.5f, 0.88f),
        "一番上" to Anchor(0.5f, 0.12f),
        "下部" to Anchor(0.5f, 0.85f),
        "上部" to Anchor(0.5f, 0.15f),
        "中央" to Anchor(0.5f, 0.5f),
        "右" to Anchor(0.85f, 0.5f),
        "左" to Anchor(0.15f, 0.5f),
        // 英語の要約(小文字にして照合する)
        "bottom right" to Anchor(0.85f, 0.85f),
        "bottom left" to Anchor(0.15f, 0.85f),
        "top right" to Anchor(0.85f, 0.15f),
        "top left" to Anchor(0.15f, 0.15f),
        "at the bottom" to Anchor(0.5f, 0.85f),
        "at the top" to Anchor(0.5f, 0.15f),
        "in the center" to Anchor(0.5f, 0.5f),
    )

    /** 表の順で最初に当たる位置の言葉(「右下」は「右」より先に判定)。無ければ null。 */
    fun anchor(summary: String): Anchor? {
        val lower = summary.lowercase()
        return TABLE.firstOrNull { lower.contains(it.first) }?.second
    }
}
