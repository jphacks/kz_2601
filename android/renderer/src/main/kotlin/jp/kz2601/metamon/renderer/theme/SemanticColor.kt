package jp.kz2601.metamon.renderer.theme

import androidx.compose.material3.MaterialTheme
import androidx.compose.runtime.Composable
import androidx.compose.runtime.ReadOnlyComposable
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.Size
import jp.kz2601.metamon.dsl.Color as DslColor

// iOS の Support/SemanticColor.swift(`Color.semantic` / `SemanticPalette.badgeFill` / `Component.symbolPointSize`)の写し。

/** badge のピル背景の不透明度。 */
const val BADGE_FILL_ALPHA = 0.15f

/** schema の SemanticColor → 色。red / green / orange / blue は面に対し 4.5:1 のトーン(RenderColors)。未知は文字の色。 */
@Composable
@ReadOnlyComposable
fun semanticColor(c: DslColor?): Color {
    val colors = RenderTheme.colors
    return when (c) {
        DslColor.SECONDARY -> MaterialTheme.colorScheme.onSurfaceVariant
        DslColor.ACCENT -> colors.brandText
        DslColor.RED -> colors.negative
        DslColor.GREEN -> colors.positive
        DslColor.ORANGE -> colors.warning
        DslColor.BLUE -> colors.info
        else -> MaterialTheme.colorScheme.onSurface
    }
}

/** icon / avatar の size(small 16 / large 44 / それ以外 28)。 */
val Component.symbolSize: Dp
    get() = when (size) {
        Size.SMALL -> 16.dp
        Size.LARGE -> 44.dp
        else -> 28.dp
    }
