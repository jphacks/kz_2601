package jp.kz2601.metamon.renderer.components

import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.remember
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.AnnotatedString
import androidx.compose.ui.text.LinkAnnotation
import androidx.compose.ui.text.SpanStyle
import androidx.compose.ui.text.TextLinkStyles
import androidx.compose.ui.text.buildAnnotatedString
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontStyle
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextDecoration
import androidx.compose.ui.text.withLink
import androidx.compose.ui.text.withStyle
import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.logic.ExternalUrlPolicy
import jp.kz2601.metamon.renderer.theme.RenderTheme
import jp.kz2601.metamon.renderer.theme.colors

/** `markdown`: インラインの Markdown(範囲は [InlineMarkdown])。リンクは [ExternalUrlPolicy] で許した URL だけ押せる。 */
@Composable
fun MarkdownLeaf(c: Component, modifier: Modifier = Modifier) {
    val raw = c.text.orEmpty()
    val linkColor = RenderTheme.colors.brandText
    val codeBg = RenderTheme.colors.surfaceElevated
    val text = remember(raw, linkColor, codeBg) { markdownAnnotated(raw, linkColor, codeBg) }
    Text(text, modifier.fillMaxWidth(), style = RenderTheme.Fonts.body)
}

internal fun markdownAnnotated(
    raw: String,
    linkColor: androidx.compose.ui.graphics.Color,
    codeBackground: androidx.compose.ui.graphics.Color,
): AnnotatedString = buildAnnotatedString {
    for (s in InlineMarkdown.parse(raw)) {
        val style = SpanStyle(
            fontWeight = if (s.bold) FontWeight.Bold else null,
            fontStyle = if (s.italic) FontStyle.Italic else null,
            textDecoration = if (s.strike) TextDecoration.LineThrough else null,
            fontFamily = if (s.code) FontFamily.Monospace else null,
            background = if (s.code) codeBackground else androidx.compose.ui.graphics.Color.Unspecified,
        )
        val url = s.url?.takeIf { ExternalUrlPolicy.openable(it) != null }
        if (url != null) {
            val linkStyle = TextLinkStyles(SpanStyle(color = linkColor, textDecoration = TextDecoration.Underline))
            withLink(LinkAnnotation.Url(url, linkStyle)) { withStyle(style) { append(s.text) } }
        } else {
            withStyle(style) { append(s.text) }
        }
    }
}
