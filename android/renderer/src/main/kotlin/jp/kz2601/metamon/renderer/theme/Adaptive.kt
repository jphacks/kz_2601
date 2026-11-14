package jp.kz2601.metamon.renderer.theme

import android.provider.Settings
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.BoxScope
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.widthIn
import androidx.compose.runtime.Composable
import androidx.compose.runtime.remember
import androidx.compose.runtime.staticCompositionLocalOf
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalConfiguration
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp

// iOS の App/LandscapeLayout.swift(横向き・#625)と accessibilityReduceMotion の写し。
// 方針: 縦と同じ部品を使い、横・タブレット・折りたたみでは「幅を絞って中央に寄せる」「大きな飾りを小さくする」。

object AdaptiveLayout {
    /** 文章・ボタン・グリッドを読みやすく保つ最大幅(iOS の readableWidth 560pt)。 */
    val readableWidth: Dp = 560.dp

    /** 高さが足りない(横向きのスマホ)。iOS の verticalSizeClass == .compact に当たる。 */
    fun isCompactHeight(screenHeightDp: Int): Boolean = screenHeightDp < 480

    /** 音声画面の大きなマイクの径。 */
    fun micSize(isInitial: Boolean, compactHeight: Boolean): Dp = when {
        isInitial && !compactHeight -> 132.dp
        !isInitial && !compactHeight -> 104.dp
        isInitial -> 96.dp
        else -> 84.dp
    }

    /** チュートリアルのツルの大きさ。 */
    fun tutorialMascotSize(compactHeight: Boolean): Dp = if (compactHeight) 84.dp else 132.dp
}

/** 今の画面が横向きのスマホのように低いか。 */
@Composable
fun isCompactHeight(): Boolean = AdaptiveLayout.isCompactHeight(LocalConfiguration.current.screenHeightDp)

/** 横に広い時だけ幅を [width] までにして中央に寄せる(縦向きのスマホでは何も変わらない)。 */
@Composable
fun ReadableWidth(
    modifier: Modifier = Modifier,
    width: Dp = AdaptiveLayout.readableWidth,
    content: @Composable BoxScope.() -> Unit,
) {
    Box(modifier.fillMaxSize(), contentAlignment = Alignment.TopCenter) {
        Box(Modifier.widthIn(max = width).fillMaxWidth().fillMaxSize(), content = content)
    }
}

/** 端末の「アニメーションを削除」(アニメーション再生時間スケール 0)。iOS の「視差効果を減らす」に当たる。 */
val LocalReduceMotion = staticCompositionLocalOf { false }

@Composable
internal fun rememberSystemReduceMotion(): Boolean {
    val resolver = LocalContext.current.contentResolver
    return remember(resolver) {
        Settings.Global.getFloat(resolver, Settings.Global.ANIMATOR_DURATION_SCALE, 1f) == 0f
    }
}
