package jp.kz2601.metamon.renderer.components

import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.AnnotatedString
import androidx.compose.ui.text.SpanStyle
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.buildAnnotatedString
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import jp.kz2601.metamon.dsl.Align
import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.ComponentStyle
import jp.kz2601.metamon.dsl.TextHighlightStyle
import jp.kz2601.metamon.dsl.expr.ExprEvaluator
import jp.kz2601.metamon.dsl.logic.HighlightWords
import jp.kz2601.metamon.dsl.records.FieldType
import jp.kz2601.metamon.dsl.records.JSONValue
import jp.kz2601.metamon.dsl.records.Record
import jp.kz2601.metamon.renderer.components.actions.rememberEvaluatorFactory
import jp.kz2601.metamon.renderer.form.FormDraft
import jp.kz2601.metamon.renderer.registry.RenderContext
import jp.kz2601.metamon.renderer.theme.RenderTheme

/** 蛍光ペンの色(iOS の `Color.yellow.opacity(0.4)`)。ライト / ダークのどちらでも文字が読める薄さ。 */
private val HighlightMarker = Color(0xFFFFCC00).copy(alpha = 0.4f)

/** text / label の style(title | headline | body | caption)→ 文字の形。 */
internal fun leafTextStyle(style: ComponentStyle?): TextStyle = when (style) {
    ComponentStyle.TITLE -> RenderTheme.Fonts.title
    ComponentStyle.HEADLINE -> RenderTheme.Fonts.headline
    ComponentStyle.CAPTION -> RenderTheme.Fonts.caption
    else -> RenderTheme.Fonts.body
}

/**
 * `text`: style と align(leading | center | trailing)。`highlight` の語に印(marker: 背景 + 太字 / bold: 太字だけ)。
 * screen.entity の画面では `{{fieldId}}` を開いているレコードの値にし、`highlight.field` の値から語を取る(iOS の `TextLeaf`)。
 * 印は見た目だけで、読み上げはふつうの文として読む。
 */
@Composable
fun TextLeaf(c: Component, ctx: RenderContext, modifier: Modifier = Modifier) {
    val align = when (c.align) {
        Align.CENTER -> TextAlign.Center
        Align.TRAILING -> TextAlign.End
        else -> TextAlign.Start
    }
    val form = ctx.form
    val raw = c.text.orEmpty()
    // 式を読むのは差し込みがある時だけ(ふつうの文で記録の流れを見ない)
    val text = if (form != null && raw.contains("{{")) TextContent.resolve(raw, form, rememberEvaluatorFactory(ctx.runtime)) else raw
    val fieldValue = c.highlight?.field?.let { form?.values?.get(it)?.displayString }
    Text(highlighted(c, fieldValue, text), modifier.fillMaxWidth(), style = leafTextStyle(c.style), textAlign = align)
}

/** text の `{{id}}` の差し込み(iOS の `TextContent`)。 */
internal object TextContent {
    /** 置き換えてよい field(hidden でない・ふつうの値)だけ値にする。保存値はそのまま、computed は評価。他は文字のまま。 */
    fun resolve(text: String, form: FormDraft?, evaluator: () -> ExprEvaluator): String {
        if (form == null || !text.contains("{{")) return text
        val row by lazy { ExprEvaluator.Row(form.entity, Record(form.recordId ?: "draft", form.values.toMap()), memoizable = false) }
        var s = text
        for (f in form.entity.fields) {
            val token = "{{${f.id}}}"
            if (!s.contains(token) || !HighlightWords.substitutes(form.entity, f.id)) continue
            val value = if (f.isComputed) {
                val v = evaluator().fieldValue(f.id, row)
                // datetime は text だと日付だけになる → 表示の書式で時刻まで
                if (f.type == FieldType.DATETIME) v?.json(f)?.displayString.orEmpty() else v?.text.orEmpty()
            } else {
                (form.values[f.id] ?: JSONValue.Null).displayString
            }
            s = s.replace(token, value)
        }
        return s
    }
}

/** 印をつけた文([text] は差し込み後の文)。highlight が無ければそのまま。 */
fun highlighted(c: Component, fieldValue: String?, text: String = c.text.orEmpty()): AnnotatedString {
    val h = c.highlight ?: return AnnotatedString(text)
    val ranges = HighlightWords.ranges(text, HighlightWords.resolve(h.words, h.field, fieldValue))
    if (ranges.isEmpty()) return AnnotatedString(text)
    val span = if (h.style == TextHighlightStyle.BOLD) {
        SpanStyle(fontWeight = FontWeight.Bold)
    } else {
        SpanStyle(fontWeight = FontWeight.Bold, background = HighlightMarker)
    }
    return buildAnnotatedString {
        append(text)
        ranges.forEach { addStyle(span, it.first, it.last + 1) }
    }
}
