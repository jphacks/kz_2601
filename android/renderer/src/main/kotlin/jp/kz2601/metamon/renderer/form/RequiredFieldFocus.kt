package jp.kz2601.metamon.renderer.form

import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.Screen
import jp.kz2601.metamon.renderer.components.inputs.InputRules
import jp.kz2601.metamon.renderer.layout.formBands
import jp.kz2601.metamon.renderer.layout.gridPlacement

/** 保存で弾かれた時に、最初の空の必須欄へスクロールしてフォーカスする(M10・iOS の `RequiredFieldScroller`)。 */
object RequiredFieldFocus {
    /** キーボードが閉じて表示が落ち着くまで(保存はフォーカスを外してから走る)。 */
    const val SETTLE_MILLIS = 300L

    /** [c] が入力する、このフォームの field。 */
    fun boundField(c: Component, form: FormDraft): String? {
        if (c.type !in InputRules.formBound) return null
        if (c.entity != null && c.entity != form.entity.id) return null
        return c.field
    }

    /** 画面の並び順で最初の、印の付いた欄(area の段 → 列、area 無しはその後。隠す field は除く)。 */
    fun firstFlagged(screen: Screen, form: FormDraft, currentUid: String?): String? {
        val placement = screen.gridPlacement
        val top = formBands(placement.placed).flatMap { band -> band.items.map { it.component } } + placement.stacked
        return top.asSequence().flatMap(::flattened).firstNotNullOfOrNull { c ->
            boundField(c, form)?.takeIf { form.isFlagged(it) && !form.hides(c, currentUid) }
        }
    }

    private fun flattened(c: Component): Sequence<Component> =
        sequenceOf(c) + c.children.asSequence().flatMap(::flattened)
}
