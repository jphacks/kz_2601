package jp.kz2601.metamon.renderer.components

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Button
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import jp.kz2601.metamon.dsl.Action
import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.renderer.registry.RenderContext
import jp.kz2601.metamon.renderer.theme.RenderTheme
import jp.kz2601.metamon.renderer.theme.colors

// #357 で入れた最小の Leaf。残りの Leaf は子 Issue で 1 部品 1 ファイルで足す。

/** `text`: style(title | headline | body | caption)と align(leading | center | trailing)。 */
@Composable
fun TextLeaf(c: Component, modifier: Modifier = Modifier) {
    val typography = MaterialTheme.typography
    val style = when (c.style) {
        "title" -> typography.headlineSmall.copy(fontWeight = FontWeight.Bold)
        "headline" -> typography.titleMedium.copy(fontWeight = FontWeight.SemiBold)
        "caption" -> typography.bodySmall
        else -> typography.bodyLarge
    }
    val align = when (c.align) {
        "center" -> TextAlign.Center
        "trailing" -> TextAlign.End
        else -> TextAlign.Start
    }
    Text(c.text.orEmpty(), modifier.fillMaxWidth(), style = style, textAlign = align)
}

/** `spacer`: size(small 8 / medium 16 / large 32 / それ以外は 0)。 */
@Composable
fun SpacerLeaf(c: Component, modifier: Modifier = Modifier) {
    val h = when (c.size) {
        "small" -> 8.dp
        "medium" -> 16.dp
        "large" -> 32.dp
        else -> 0.dp
    }
    Spacer(modifier.height(h))
}

@Composable
fun DividerLeaf(modifier: Modifier = Modifier) {
    HorizontalDivider(modifier, color = RenderTheme.colors.hairline)
}

/** button が見えるか(iOS の `ButtonLeaf.isVisible`)。フォームは子 Issue で入れるので、今は新規のフォーム扱い。 */
fun isButtonVisible(action: Action?, isEditing: Boolean = false, readOnly: Boolean = false): Boolean = when (action?.type) {
    "deleteRecord" -> isEditing
    "saveRecord" -> !readOnly
    else -> true
}

/** `button`: style(primary | secondary | plain | destructive)。押すと action を実行する。 */
@Composable
fun ButtonLeaf(c: Component, ctx: RenderContext, modifier: Modifier = Modifier) {
    if (!isButtonVisible(c.action)) return
    val label = c.label.orEmpty()
    val onClick = { c.action?.let(ctx.perform) ?: Unit }
    val shape = RoundedCornerShape(RenderTheme.Radius.button)
    val m = modifier.fillMaxWidth()
    when (c.style) {
        "secondary" -> OutlinedButton(onClick, m, shape = shape) { Text(label) }
        "plain" -> TextButton(onClick, m) { Text(label) }
        else -> Button(onClick, m, shape = shape) { Text(label, fontWeight = FontWeight.SemiBold) }
    }
}

/** 音声の入口(仮)。認識は #359。 */
@Composable
fun VoiceInputLeaf(c: Component, ctx: RenderContext, modifier: Modifier = Modifier) {
    Column(
        modifier = modifier.fillMaxWidth().padding(vertical = 48.dp),
        horizontalAlignment = Alignment.CenterHorizontally,
        verticalArrangement = Arrangement.spacedBy(RenderTheme.Spacing.lg),
    ) {
        Text(
            c.prompt ?: "話しかけてください",
            style = MaterialTheme.typography.headlineSmall,
            textAlign = TextAlign.Center,
        )
        val buttonSize = if (c.size == "large") 120.dp else 72.dp
        Button(
            onClick = { ctx.runtime.onVoiceInput(c.id) },
            shape = CircleShape,
            modifier = Modifier.size(buttonSize),
        ) {
            Text("話す", style = MaterialTheme.typography.titleMedium)
        }
    }
}
