package jp.kz2601.metamon.ui

import androidx.annotation.DrawableRes
import androidx.compose.animation.core.Animatable
import androidx.compose.animation.core.FastOutSlowInEasing
import androidx.compose.animation.core.RepeatMode
import androidx.compose.animation.core.Spring
import androidx.compose.animation.core.infiniteRepeatable
import androidx.compose.animation.core.spring
import androidx.compose.animation.core.tween
import androidx.compose.foundation.Image
import androidx.compose.foundation.background
import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.remember
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.TransformOrigin
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.semantics.clearAndSetSemantics
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import jp.kz2601.metamon.R
import jp.kz2601.metamon.renderer.theme.LocalReduceMotion
import jp.kz2601.metamon.renderer.theme.RenderTheme
import jp.kz2601.metamon.renderer.theme.colors
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch

/**
 * モチーフキャラのツル(#102)。iOS の App/CraneMascot.swift の写し。本体の画面だけに出す装飾(生成したアプリの中には出さない)。
 * ふわっと出て軽く揺れる。「アニメーションを削除」では動かさない。読み上げからは隠す。
 */
enum class CranePose(@param:DrawableRes val drawable: Int) {
    HAPPY(R.drawable.crane_happy),
    JOY(R.drawable.crane_joy),
    IDEA(R.drawable.crane_idea),
    PANIC(R.drawable.crane_panic),
    POINT(R.drawable.crane_point),
    TEA(R.drawable.crane_tea),
    PC(R.drawable.crane_pc),
    PHONE(R.drawable.crane_phone),
    BYE(R.drawable.crane_bye),
}

/** [sway] は揺れの大きさ(度)。0 なら出現の動きだけ。 */
@Composable
fun CraneMascot(pose: CranePose, modifier: Modifier = Modifier, size: Dp = 88.dp, sway: Float = 4f) {
    val reduceMotion = LocalReduceMotion.current
    val appear = remember { Animatable(if (reduceMotion) 1f else 0f) }
    val angle = remember { Animatable(0f) }

    LaunchedEffect(reduceMotion, sway) {
        if (reduceMotion) {
            appear.snapTo(1f)
            angle.snapTo(0f)
            return@LaunchedEffect
        }
        launch { appear.animateTo(1f, spring(dampingRatio = Spring.DampingRatioMediumBouncy, stiffness = Spring.StiffnessLow)) }
        if (sway > 0f) {
            angle.snapTo(-sway)
            delay(500)
            angle.animateTo(sway, infiniteRepeatable(tween(1600, easing = FastOutSlowInEasing), RepeatMode.Reverse))
        }
    }

    // 線画は黒線なので、ダークでは生成りの円を敷いて見えるようにする。
    val backdrop = if (isSystemInDarkTheme()) Color(0.95f, 0.93f, 0.88f, 0.92f) else RenderTheme.colors.brandA.copy(alpha = 0.06f)
    Image(
        painter = painterResource(pose.drawable),
        contentDescription = null,
        modifier = modifier
            .clearAndSetSemantics {}
            .size(size)
            .graphicsLayer {
                val a = appear.value
                scaleX = 0.6f + 0.4f * a
                scaleY = 0.6f + 0.4f * a
                alpha = a.coerceIn(0f, 1f)
                rotationZ = angle.value
                transformOrigin = TransformOrigin(0.5f, 1f)
            }
            .background(backdrop, CircleShape)
            .padding(size * 0.08f),
    )
}
