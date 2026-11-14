package jp.kz2601.metamon.renderer.components

import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.height
import androidx.compose.material3.HorizontalDivider
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.Size
import jp.kz2601.metamon.renderer.theme.RenderTheme
import jp.kz2601.metamon.renderer.theme.colors

/** spacer の高さ(small 8 / medium 16 / large 32 / それ以外は 0。iOS の flexible は詰めて 0)。 */
val Component.spacerHeight: Dp
    get() = when (size) {
        Size.SMALL -> 8.dp
        Size.MEDIUM -> 16.dp
        Size.LARGE -> 32.dp
        else -> 0.dp
    }

/** `spacer`: 縦の空き。読み上げには出ない。 */
@Composable
fun SpacerLeaf(c: Component, modifier: Modifier = Modifier) {
    Spacer(modifier.height(c.spacerHeight))
}

/** `divider`: 区切り線(hairline)。 */
@Composable
fun DividerLeaf(modifier: Modifier = Modifier) {
    HorizontalDivider(modifier, color = RenderTheme.colors.hairline)
}
