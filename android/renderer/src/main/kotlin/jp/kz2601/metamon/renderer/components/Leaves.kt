package jp.kz2601.metamon.renderer.components

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Button
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
import jp.kz2601.metamon.dsl.ActionType
import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.ComponentStyle
import jp.kz2601.metamon.dsl.Size
import jp.kz2601.metamon.renderer.registry.RenderContext
import jp.kz2601.metamon.renderer.theme.RenderTheme

// #357 で入れた最小の Leaf(button・voiceInput)。表示系の Leaf は 1 部品 1 ファイル(TextLeaf.kt など・#857)。

/** button が見えるか(iOS の `ButtonLeaf.isVisible`)。フォームは子 Issue で入れるので、今は新規のフォーム扱い。 */
fun isButtonVisible(action: Action?, isEditing: Boolean = false, readOnly: Boolean = false): Boolean = when (action?.type) {
    ActionType.DELETE_RECORD -> isEditing
    ActionType.SAVE_RECORD -> !readOnly
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
        ComponentStyle.SECONDARY -> OutlinedButton(onClick, m, shape = shape) { Text(label) }
        ComponentStyle.PLAIN -> TextButton(onClick, m) { Text(label) }
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
        val buttonSize = if (c.size == Size.LARGE) 120.dp else 72.dp
        Button(
            onClick = { ctx.runtime.onVoiceInput(c.id) },
            shape = CircleShape,
            modifier = Modifier.size(buttonSize),
        ) {
            Text("話す", style = MaterialTheme.typography.titleMedium)
        }
    }
}
