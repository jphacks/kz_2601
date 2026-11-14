package jp.kz2601.metamon.renderer.layout

import androidx.compose.ui.geometry.Rect
import kotlin.math.ceil

/**
 * マイク FAB(ホストが左下 or 右下に置く)と DSL 部品の衝突回避(iOS の `FabClearance`・#83 / #52 / #157)。
 * - 最下段に寄せた操作部品(下 2 行・吸着した側の 2 列)があれば、FAB をその上端まで上げる([obstructedRows])
 * - 背の高い list などは FAB が上に浮く背景なので、中身の末尾に FAB ぶんの余白を足す([contentMargin])
 */
object FabClearance {
    /** FAB が座る帯 = 最下段 2 行 */
    const val BAND_ROWS = 2

    /** FAB が重なる列 = 吸着している側の端 2 列(左: A〜B / 右: E〜F) */
    const val BAND_COLUMNS = 2

    /** 「最下段に寄せた操作部品」とみなす行の下限(これより上から始まる背の高い部品は、FAB が浮く背景として扱う) */
    const val ANCHORED_ROW_MIN = GridArea.VISIBLE_ROWS - 3

    /** FAB の上端と最後の行の間に空ける距離(dp) */
    const val CONTENT_GAP_DP = 8f

    /** FAB の上にこれだけも残らない(低い器を FAB が丸ごと覆う)時は余白を足さない(dp) */
    const val MIN_VISIBLE_DP = 32f

    /**
     * 帯に掛かる部品のうち、最も上から始まるものが占める「下からの行数」(0 = 障害なし)。
     * 例(左): `A11:F12` → 2 / `A12:F12` → 1 / `D11:F12`(右下の追加ボタン)→ 0 / `A1:F12`(全面 list)→ 0。
     * 例(右): `D11:F12` → 2 / `A11:D12` → 0。
     */
    fun obstructedRows(areas: List<GridArea>, leading: Boolean = true): Int = areas
        .filter { a ->
            val inBand = if (leading) a.col1 < BAND_COLUMNS else a.col2 >= GridArea.COLUMNS - BAND_COLUMNS
            inBand && a.row2 > GridArea.VISIBLE_ROWS - BAND_ROWS && a.row1 >= ANCHORED_ROW_MIN
        }
        .maxOfOrNull { GridArea.VISIBLE_ROWS - it.row1 + 1 } ?: 0

    /**
     * スクロール領域 [viewport](ルート座標・px)の末尾に足す余白(px)。FAB と横に重ならない / FAB が領域より下 or 上 /
     * FAB が無い(隠れている・非表示)なら 0。FAB の上に [minVisible] も残らない時も 0(空白までスクロールさせない)。
     */
    fun contentMargin(viewport: Rect, fab: Rect?, gap: Float, minVisible: Float): Float {
        if (fab == null || viewport.isEmpty || fab.isEmpty) return 0f
        if (fab.right <= viewport.left || fab.left >= viewport.right) return 0f
        if (fab.top >= viewport.bottom || fab.bottom <= viewport.top) return 0f
        if (fab.top - gap - viewport.top < minVisible) return 0f
        return ceil(viewport.bottom - fab.top + gap)
    }
}
