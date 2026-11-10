package jp.kz2601.metamon.renderer.components

import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.ReadOnlyComposable
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
import jp.kz2601.metamon.dsl.TextHighlightMatcher
import jp.kz2601.metamon.dsl.TextHighlightStyle
import jp.kz2601.metamon.dsl.resolvedWords

/** 蛍光ペンの色(iOS の `Color.yellow.opacity(0.4)`)。ライト / ダークのどちらでも文字が読める薄さ。 */
private val HighlightMarker = Color(0xFFFFCC00).copy(alpha = 0.4f)

/** text / label の style(title | headline | body | caption)→ 文字の形。 */
@Composable
@ReadOnlyComposable
internal fun leafTextStyle(style: ComponentStyle?): TextStyle {
    val t = MaterialTheme.typography
    return when (style) {
        ComponentStyle.TITLE -> t.headlineSmall.copy(fontWeight = FontWeight.Bold)
        ComponentStyle.HEADLINE -> t.titleMedium.copy(fontWeight = FontWeight.SemiBold)
        ComponentStyle.CAPTION -> t.bodySmall
        else -> t.bodyLarge
    }
}

/**
 * `text`: style と align(leading | center | trailing)。`highlight` の語に印(marker: 背景 + 太字 / bold: 太字だけ)。
 * 印は見た目だけで、読み上げはふつうの文として読む。
 * `{{fieldId}}` の差し込みと field から語を取る highlight は、フォームのドラフトが入る子 Issue で足す(今は書いたままの文字)。
 */
@Composable
fun TextLeaf(c: Component, modifier: Modifier = Modifier) {
    val align = when (c.align) {
        Align.CENTER -> TextAlign.Center
        Align.TRAILING -> TextAlign.End
        else -> TextAlign.Start
    }
    Text(highlighted(c, fieldValue = null), modifier.fillMaxWidth(), style = leafTextStyle(c.style), textAlign = align)
}

/** 印をつけた文。highlight が無ければそのまま。 */
fun highlighted(c: Component, fieldValue: String?): AnnotatedString {
    val text = c.text.orEmpty()
    val h = c.highlight ?: return AnnotatedString(text)
    val ranges = TextHighlightMatcher.ranges(text, h.resolvedWords(fieldValue))
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
