package jp.kz2601.metamon.renderer.layout

import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.ComponentType

/**
 * セル内スクロールの上下の端のフェード(iOS の `FadingScrollView`)。続きがある側だけ薄くする。
 * FAB ぶんの末尾の余白([trailing])はフェードの判定に含めない(余白だけが残っている時は「続き」ではない)。
 */
object EdgeFade {
    /** フェードの高さ(dp) */
    const val FADE_DP = 18f

    /** これ以下のはみ出しは誤差とみなす(dp) */
    const val SLACK_DP = 2f

    /** 切り替えのアニメーション(ms)。アニメーションを減らす設定では即時 */
    const val SWITCH_MILLIS = 180

    data class Edges(val top: Boolean, val bottom: Boolean)

    /** [scroll] = 今のスクロール量・[maxScroll] = 最大スクロール量・[trailing] = 末尾に足した余白(すべて px) */
    fun edges(scroll: Float, maxScroll: Float, trailing: Float, slack: Float): Edges =
        Edges(top = scroll > slack, bottom = maxScroll - trailing - scroll > slack)
}

/** 変身の直後の出現(iOS の `StaggeredAppear`)。index × stagger の遅延で、薄く少し小さい所から出す。 */
object AppearStagger {
    /** これより後ろの部品は同じ遅延(長い画面で最後の部品を待たせない) */
    const val MAX_INDEX = 12

    fun delayMillis(index: Int, staggerMillis: Int): Int = index.coerceIn(0, MAX_INDEX) * staggerMillis

    /**
     * 出現を動かすか。アニメーションを減らす設定では出さない。編集の直後で光らせる画面では、
     * 変わった部品は印(`changeMark`)が置き直すので、変わっていない部品も待たせずに出す。
     */
    fun animates(reduceMotion: Boolean, screenHighlighted: Boolean): Boolean = !reduceMotion && !screenHighlighted
}

/**
 * list(または list だけの section)に行がある時は、セルを行の高さで止める(iOS の `GridCell.hugsContent`・#131)。
 * 空の時やそれ以外の型は器の高さまで伸ばし、空状態をセル全体に出す(#84)。
 */
object RowHeightStop {
    /** 行の有無を見る list。対象外の型なら空 */
    fun lists(component: Component): List<Component> = when {
        component.type == ComponentType.LIST -> listOf(component)
        component.type == ComponentType.SECTION && component.children.isNotEmpty() &&
            component.children.all { it.type == ComponentType.LIST } -> component.children
        else -> emptyList()
    }

    fun hugsContent(component: Component, hasRows: (Component) -> Boolean): Boolean {
        val lists = lists(component)
        return lists.isNotEmpty() && lists.all(hasRows)
    }
}
