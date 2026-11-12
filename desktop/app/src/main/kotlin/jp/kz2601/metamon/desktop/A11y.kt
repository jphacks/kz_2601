package jp.kz2601.metamon.desktop

import androidx.compose.ui.Modifier
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import jp.kz2601.metamon.desktop.i18n.Strings
import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.ComponentType
import jp.kz2601.metamon.dsl.ExprNumber
import jp.kz2601.metamon.dsl.records.Aggregator

/**
 * 読み上げ(Windows の Narrator・Linux の Orca)向けの説明(#1759)。
 * 絵で描く部品(stat・progressBar・gauge・chart・rating・image・avatar・label 付きの icon)は 1 つにまとめ、値を言葉にして読ませる。
 * 文字や Material のボタン・入力で描く部品は Compose の既定(文字・役割)のまま。
 */
object A11y {
    /** 部品を 1 つにまとめて読む時の説明。まとめない部品は null。 */
    fun describe(c: Component, data: LeafData): String? = when (c.type) {
        ComponentType.STAT -> join(c.label ?: c.title, StatValue.resolve(c, data).first)
        ComponentType.PROGRESS_BAR -> join(c.label ?: c.title, ProgressValue.percent(c, data))
        ComponentType.GAUGE -> GaugeValue.value(c, data).let { (v, unit) -> join(c.label ?: c.title, Aggregator.format(v, null) + unit.orEmpty()) }
        ComponentType.CHART -> {
            val buckets = ChartBuckets.buckets(c, data)
            val body = if (buckets.isEmpty()) Strings["desktop_chart_empty"] else buckets.joinToString(Strings["desktop_list_separator"]) { "${it.label} ${Aggregator.format(it.value, null)}" }
            join(c.title ?: c.label, Strings.format("desktop_a11y_chart", body))
        }
        ComponentType.RATING -> if (c.field == null) {
            val max = (c.max ?: 5.0).toInt().coerceIn(1, 10)
            val value = (c.value as? ExprNumber)?.value?.toInt() ?: 0
            Strings.format("desktop_a11y_rating", c.label.orEmpty(), value, max).trim()
        } else {
            null
        }
        ComponentType.IMAGE -> c.label ?: Strings["desktop_a11y_image"]
        ComponentType.AVATAR -> c.label ?: c.title ?: Strings["inputs_avatar"]
        // 飾り(読まない)。label があればそれを読む
        ComponentType.ICON -> c.label?.takeIf { it.isNotBlank() }
        else -> null
    }

    /** 写真のプレビュー(PhotoPreview.kt)のダイアログの名前。項目名を付ける。 */
    fun previewTitle(name: String): String = Strings.format("desktop_a11y_preview", name.ifBlank { Strings["desktop_a11y_image"] })

    private fun join(label: String?, value: String): String = listOfNotNull(label?.takeIf { it.isNotBlank() }, value).joinToString(" ")
}

/** 部品 1 つ分の読み上げのまとまり。説明の無い部品はそのまま。 */
fun Modifier.leafSemantics(c: Component, data: LeafData): Modifier {
    val text = runCatching { A11y.describe(c, data) }.getOrNull() ?: return this
    return semantics(mergeDescendants = true) { contentDescription = text }
}
