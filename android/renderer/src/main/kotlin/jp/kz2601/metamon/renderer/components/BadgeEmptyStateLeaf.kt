package jp.kz2601.metamon.renderer.components

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.renderer.theme.BADGE_FILL_ALPHA
import jp.kz2601.metamon.renderer.theme.RenderTheme
import jp.kz2601.metamon.renderer.theme.SymbolIcon
import jp.kz2601.metamon.renderer.theme.colors
import jp.kz2601.metamon.renderer.theme.semanticColor

/** `badge`: 短い文字の角丸ピル。color は semantic(背景は同じ色の薄塗り)。折り返さず 1 行。 */
@Composable
fun BadgeLeaf(c: Component, modifier: Modifier = Modifier) {
    val tint = semanticColor(c.color)
    Text(
        c.text.orEmpty(),
        modifier
            .background(tint.copy(alpha = tint.alpha * BADGE_FILL_ALPHA), CircleShape)
            .padding(horizontal = 10.dp, vertical = 5.dp),
        color = tint,
        style = MaterialTheme.typography.labelMedium.copy(fontWeight = FontWeight.SemiBold),
        maxLines = 1,
        overflow = TextOverflow.Ellipsis,
    )
}

/** `emptyState`: アイコン + 見出し + 説明の空状態(iOS の ContentUnavailableView)。見出しは読み上げで見出し扱い。 */
@Composable
fun EmptyStateLeaf(c: Component, modifier: Modifier = Modifier) {
    Column(
        modifier.fillMaxWidth().padding(vertical = RenderTheme.Spacing.xl, horizontal = RenderTheme.Spacing.md),
        horizontalAlignment = Alignment.CenterHorizontally,
        verticalArrangement = Arrangement.spacedBy(RenderTheme.Spacing.sm),
    ) {
        SymbolIcon(c.icon ?: "tray", 44.dp, RenderTheme.colors.brandA.copy(alpha = 0.7f))
        Text(
            c.title.orEmpty(),
            Modifier.semantics { heading() },
            style = MaterialTheme.typography.titleLarge.copy(fontWeight = FontWeight.Bold),
            textAlign = TextAlign.Center,
        )
        c.message?.let {
            Text(
                it,
                style = MaterialTheme.typography.bodyMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                textAlign = TextAlign.Center,
            )
        }
    }
}
