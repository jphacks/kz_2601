package jp.kz2601.metamon.renderer.screen

import androidx.compose.animation.core.Animatable
import androidx.compose.animation.core.Spring
import androidx.compose.animation.core.spring
import androidx.compose.animation.core.tween
import androidx.compose.foundation.border
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.compositionLocalOf
import androidx.compose.runtime.remember
import androidx.compose.ui.Modifier
import androidx.compose.ui.composed
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.unit.dp
import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.logic.ChangeHighlight
import jp.kz2601.metamon.renderer.theme.LocalReduceMotion
import jp.kz2601.metamon.renderer.theme.RenderTheme
import jp.kz2601.metamon.renderer.theme.colors
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch

/**
 * 編集の直後に光らせる部品(#894・iOS の `\.changeHighlight`)。ホストが変身の直後に渡し、少し後に [ChangeHighlight.NONE] に戻す。
 * 部品のファイルは触らず、画面のグリッド([GridScreenLayout])がセルの外側に印を付ける。
 */
val LocalChangeHighlight = compositionLocalOf { ChangeHighlight.NONE }

/**
 * 変わった部品の印(iOS の `StaggeredAppear` の highlight の分)。少し縮んだ所から置き直し、枠の光を 1 秒ほどで消す。
 * 「アニメーションを削除」では動かさず、枠の光の淡いフェードだけ。印は読み上げに出さない(飾り)。
 */
internal fun Modifier.changeMark(screen: String, component: Component, index: Int): Modifier = composed {
    val highlight = LocalChangeHighlight.current
    val changed = screen in highlight.screens && highlight.contains(screen, component, index)
    val reduceMotion = LocalReduceMotion.current
    val glow = remember { Animatable(0f) }
    val scale = remember { Animatable(1f) }
    LaunchedEffect(highlight, changed) {
        if (!changed) {
            glow.snapTo(0f)
            scale.snapTo(1f)
            return@LaunchedEffect
        }
        glow.snapTo(1f)
        if (!reduceMotion) {
            scale.snapTo(0.9f)
            launch {
                delay(150)
                scale.animateTo(1f, spring(dampingRatio = Spring.DampingRatioMediumBouncy, stiffness = Spring.StiffnessLow))
            }
        }
        delay(500)
        glow.animateTo(0f, tween(1100))
    }
    val color = RenderTheme.colors.brandB
    val shape = remember { RoundedCornerShape(RenderTheme.Radius.card) }
    this
        .graphicsLayer {
            scaleX = scale.value
            scaleY = scale.value
        }
        .then(if (glow.value > 0f) Modifier.border(2.dp, color.copy(alpha = glow.value), shape) else Modifier)
}
