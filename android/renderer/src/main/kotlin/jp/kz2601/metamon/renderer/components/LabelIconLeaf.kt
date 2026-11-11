package jp.kz2601.metamon.renderer.components

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.semantics.clearAndSetSemantics
import androidx.compose.ui.semantics.semantics
import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.renderer.theme.RenderTheme
import jp.kz2601.metamon.renderer.theme.SymbolIcon
import jp.kz2601.metamon.renderer.theme.semanticColor
import jp.kz2601.metamon.renderer.theme.symbolSize

/** `label`: アイコン + 文字の 1 行(iOS の Label)。style は text と同じ。読み上げは文字だけ。 */
@Composable
fun LabelLeaf(c: Component, modifier: Modifier = Modifier) {
    val style = leafTextStyle(c.style)
    val iconSize = with(LocalDensity.current) { style.fontSize.toDp() }
    Row(
        modifier.fillMaxWidth().semantics(mergeDescendants = true) {},
        horizontalArrangement = Arrangement.spacedBy(RenderTheme.Spacing.sm),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        SymbolIcon(c.icon ?: "circle", iconSize, semanticColor(null))
        Text(c.text.orEmpty(), style = style)
    }
}

/** `icon`: アイコン 1 個。size(small | medium | large)と semantic color。装飾なので読み上げから隠す。 */
@Composable
fun IconLeaf(c: Component, modifier: Modifier = Modifier) {
    Box(modifier.fillMaxWidth().clearAndSetSemantics {}, contentAlignment = Alignment.Center) {
        SymbolIcon(c.name ?: "questionmark.circle", c.symbolSize, semanticColor(c.color))
    }
}

