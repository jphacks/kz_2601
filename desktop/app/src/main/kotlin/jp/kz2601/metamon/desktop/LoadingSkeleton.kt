package jp.kz2601.metamon.desktop

import androidx.compose.animation.core.RepeatMode
import androidx.compose.animation.core.animateFloat
import androidx.compose.animation.core.infiniteRepeatable
import androidx.compose.animation.core.rememberInfiniteTransition
import androidx.compose.animation.core.tween
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.widthIn
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.alpha
import androidx.compose.ui.semantics.LiveRegionMode
import androidx.compose.ui.semantics.clearAndSetSemantics
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.liveRegion
import androidx.compose.ui.unit.dp
import jp.kz2601.metamon.desktop.i18n.Strings

/** 起動時に端末の記録を読む間の形だけの画面(#1887)。見出し 1 本と行をいくつか、ゆっくり明滅させる。読み上げは「読み込み中」。 */
@Composable
fun LoadingSkeleton() {
    val label = Strings["desktop_loading"]
    val pulse by rememberInfiniteTransition().animateFloat(
        initialValue = 0.4f,
        targetValue = 0.9f,
        animationSpec = infiniteRepeatable(tween(800), RepeatMode.Reverse),
    )
    Surface(Modifier.fillMaxSize().clearAndSetSemantics { contentDescription = label; liveRegion = LiveRegionMode.Polite }) {
        Box(Modifier.fillMaxSize(), contentAlignment = Alignment.TopCenter) {
            Column(
                Modifier.widthIn(max = 720.dp).fillMaxWidth().padding(16.dp).alpha(pulse),
                verticalArrangement = Arrangement.spacedBy(12.dp),
            ) {
                Bar(0.5f, 28)
                repeat(SKELETON_ROWS) { Bar(1f, 48) }
            }
        }
    }
}

private const val SKELETON_ROWS = 5

@Composable
private fun Bar(width: Float, height: Int) {
    Box(Modifier.fillMaxWidth(width).height(height.dp).background(MaterialTheme.colorScheme.surfaceVariant, MaterialTheme.shapes.small))
}
