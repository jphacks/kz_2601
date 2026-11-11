package jp.kz2601.metamon.renderer.screen

import androidx.compose.animation.core.Animatable
import androidx.compose.animation.core.FastOutSlowInEasing
import androidx.compose.animation.core.animateFloatAsState
import androidx.compose.animation.core.snap
import androidx.compose.animation.core.tween
import androidx.compose.foundation.ScrollState
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.derivedStateOf
import androidx.compose.runtime.getValue
import androidx.compose.runtime.remember
import androidx.compose.ui.Modifier
import androidx.compose.ui.composed
import androidx.compose.ui.draw.drawWithContent
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.geometry.Size
import androidx.compose.ui.graphics.BlendMode
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.CompositingStrategy
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import jp.kz2601.metamon.renderer.layout.AppearStagger
import jp.kz2601.metamon.renderer.layout.EdgeFade
import jp.kz2601.metamon.renderer.theme.LocalReduceMotion
import jp.kz2601.metamon.renderer.theme.RenderTheme
import kotlinx.coroutines.launch

/**
 * セル内スクロールの上下の端を、続きがある側だけフェード(iOS の `FadingScrollView`)。
 * [trailing] は末尾に足した FAB ぶんの余白(判定に含めない)。アニメーションを減らす設定では切り替えを即時に。
 */
internal fun Modifier.scrollEdgeFade(scroll: ScrollState, trailing: Dp = 0.dp): Modifier = composed {
    val reduceMotion = LocalReduceMotion.current
    val density = LocalDensity.current
    val edges by remember(scroll, trailing, density) {
        derivedStateOf {
            with(density) {
                EdgeFade.edges(scroll.value.toFloat(), scroll.maxValue.toFloat(), trailing.toPx(), EdgeFade.SLACK_DP.dp.toPx())
            }
        }
    }
    val spec = if (reduceMotion) snap() else tween<Float>(EdgeFade.SWITCH_MILLIS)
    // 0 = 端まで濃い / 1 = 端を透明に
    val top by animateFloatAsState(if (edges.top) 1f else 0f, spec, label = "fadeTop")
    val bottom by animateFloatAsState(if (edges.bottom) 1f else 0f, spec, label = "fadeBottom")
    graphicsLayer { compositingStrategy = CompositingStrategy.Offscreen }
        .drawWithContent {
            drawContent()
            val fade = EdgeFade.FADE_DP.dp.toPx().coerceAtMost(size.height / 2)
            if (top > 0f) {
                val edge = Color.Black.copy(alpha = 1f - top)
                drawRect(
                    Brush.verticalGradient(0f to edge, 1f to Color.Black, startY = 0f, endY = fade),
                    size = Size(size.width, fade),
                    blendMode = BlendMode.DstIn,
                )
            }
            if (bottom > 0f) {
                val edge = Color.Black.copy(alpha = 1f - bottom)
                val y = size.height - fade
                drawRect(
                    Brush.verticalGradient(0f to Color.Black, 1f to edge, startY = y, endY = size.height),
                    topLeft = Offset(0f, y),
                    size = Size(size.width, fade),
                    blendMode = BlendMode.DstIn,
                )
            }
        }
}

/**
 * 変身の直後の出現(iOS の `StaggeredAppear`)。index × stagger の遅延で、薄く少し小さい所から出す。
 * アニメーションを減らす設定・編集の直後に光らせる画面(変わった部品は [changeMark] が置き直す)では出さない。
 */
internal fun Modifier.staggeredAppear(screen: String, index: Int): Modifier = composed {
    val reduceMotion = LocalReduceMotion.current
    val highlighted = screen in LocalChangeHighlight.current.screens
    // 最初に描いた時だけ決める(光が消えた後に出現を始め直さない)
    val animates = remember { AppearStagger.animates(reduceMotion, highlighted) }
    if (!animates) return@composed this
    val alpha = remember { Animatable(0f) }
    val scale = remember { Animatable(RenderTheme.Motion.APPEAR_SCALE) }
    LaunchedEffect(Unit) {
        val spec = tween<Float>(
            durationMillis = RenderTheme.Motion.APPEAR_MILLIS,
            delayMillis = AppearStagger.delayMillis(index, RenderTheme.Motion.STAGGER_MILLIS),
            easing = FastOutSlowInEasing,
        )
        launch { alpha.animateTo(1f, spec) }
        scale.animateTo(1f, spec)
    }
    graphicsLayer {
        this.alpha = alpha.value
        scaleX = scale.value
        scaleY = scale.value
    }
}
