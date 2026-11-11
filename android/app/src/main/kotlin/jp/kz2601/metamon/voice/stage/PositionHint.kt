package jp.kz2601.metamon.voice.stage

/**
 * 要約(「右下のボタンから◯◯を追加できます」など)に含まれる位置語を画面上の点に変える(#894・iOS の `PositionHint`)。
 * 部品単位の特定は DSL と要約の対応が無いのでせず、領域(四隅・上下・左右)のハイライトに留める。
 */
object PositionHint {
    /** 画面の幅・高さに対する割合(0〜1) */
    data class Anchor(val x: Float, val y: Float)

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
        // en の要約(小文字にして照合する)
        "bottom right" to Anchor(0.85f, 0.85f),
        "bottom left" to Anchor(0.15f, 0.85f),
        "top right" to Anchor(0.85f, 0.15f),
        "top left" to Anchor(0.15f, 0.15f),
        "at the bottom" to Anchor(0.5f, 0.85f),
        "at the top" to Anchor(0.5f, 0.15f),
        "in the center" to Anchor(0.5f, 0.5f),
    )

    /** 表の順で最初に当たる位置語(「右下」は「右」より先に判定)。無ければ null。 */
    fun anchor(summary: String): Anchor? {
        val lower = summary.lowercase()
        return TABLE.firstOrNull { lower.contains(it.first) }?.second
    }
}
