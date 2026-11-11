package jp.kz2601.metamon.renderer.theme

import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.ColorScheme
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.darkColorScheme
import androidx.compose.material3.lightColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.Immutable
import androidx.compose.runtime.ReadOnlyComposable
import androidx.compose.runtime.staticCompositionLocalOf
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.draw.shadow
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp

// 生成アプリ側のデザイントークン。iOS の Support/RenderTheme.swift・SemanticColor.swift(SemanticPalette)の写し。
// 値を変える時は iOS と揃える(色の正典は iOS の SemanticPalette)。

object RenderTheme {
    object Spacing {
        val xs = 4.dp
        val sm = 8.dp
        val md = 16.dp
        val lg = 24.dp
        val xl = 32.dp

        /** グリッドのセル間ギャップ。 */
        val gridGap = 10.dp

        /** 画面端の余白(グリッド外周)。 */
        val screenInset = 14.dp
        val inputH = 14.dp
        val inputV = 12.dp
        val formGap = 12.dp
        val row = 12.dp
    }

    object Radius {
        val control = 12.dp
        val inner = 10.dp
        val button = 14.dp
        val card = 16.dp
        val sheet = 24.dp
        val surface = 20.dp
    }

    object Shadow {
        /** iOS の radius 10 / y 3 / 不透明度 0.06 に近い控えめな影。 */
        val cardElevation = 2.dp
        val brandElevation = 6.dp
    }
}

/** ライト / ダークの色の組。 */
@Immutable
data class RenderColors(
    val brandA: Color,
    val brandB: Color,
    val brandText: Color,
    val brandFillStart: Color,
    val brandFillEnd: Color,
    val canvas: Color,
    val surface: Color,
    val surfaceElevated: Color,
    val hairline: Color,
    val positive: Color,
    val negative: Color,
    val warning: Color,
    val info: Color,
    val chartPalette: List<Color>,
) {
    val brandSoft: Color get() = brandA.copy(alpha = 0.12f)
    val brandSoftStroke: Color get() = brandA.copy(alpha = 0.22f)

    /** 文字列(カテゴリ名など)→ パレットの安定した 1 色(iOS と同じ FNV-1a)。 */
    fun category(key: String): Color {
        var h = 2166136261u
        for (b in key.encodeToByteArray()) h = (h xor b.toUByte().toUInt()) * 16777619u
        return chartPalette[(h % chartPalette.size.toUInt()).toInt()]
    }

    companion object {
        private fun rgb(r: Int, g: Int, b: Int) = Color(r, g, b)

        val Light = RenderColors(
            brandA = rgb(46, 106, 147),
            brandB = rgb(62, 154, 156),
            brandText = rgb(31, 90, 130),
            brandFillStart = rgb(36, 94, 134),
            brandFillEnd = rgb(40, 111, 126),
            canvas = rgb(246, 243, 236),
            surface = rgb(255, 253, 248),
            surfaceElevated = rgb(249, 246, 239),
            hairline = rgb(51, 82, 103).copy(alpha = 0.12f),
            positive = rgb(16, 112, 48),
            negative = rgb(190, 28, 22),
            warning = rgb(150, 76, 0),
            info = rgb(0, 84, 190),
            chartPalette = listOf(
                rgb(46, 106, 147), rgb(62, 154, 156), rgb(201, 154, 34), rgb(181, 82, 63),
                rgb(108, 139, 163), rgb(111, 150, 110), rgb(168, 139, 94), rgb(84, 98, 158),
            ),
        )

        val Dark = RenderColors(
            brandA = rgb(78, 141, 184),
            brandB = rgb(59, 158, 159),
            brandText = rgb(140, 195, 230),
            brandFillStart = rgb(36, 94, 134),
            brandFillEnd = rgb(40, 111, 126),
            canvas = rgb(15, 26, 34),
            surface = rgb(22, 35, 45),
            surfaceElevated = rgb(30, 46, 58),
            hairline = rgb(244, 241, 232).copy(alpha = 0.10f),
            positive = rgb(72, 214, 110),
            negative = rgb(255, 105, 97),
            warning = rgb(255, 170, 60),
            info = rgb(90, 160, 255),
            chartPalette = listOf(
                rgb(78, 141, 184), rgb(59, 158, 159), rgb(232, 192, 84), rgb(232, 128, 106),
                rgb(150, 180, 204), rgb(150, 196, 146), rgb(206, 178, 130), rgb(140, 154, 214),
            ),
        )
    }
}

val LocalRenderColors = staticCompositionLocalOf { RenderColors.Light }

/** 描画中のトークン。`RenderTheme.colors.surface` のように使う。 */
val RenderTheme.colors: RenderColors
    @Composable @ReadOnlyComposable get() = LocalRenderColors.current

private fun RenderColors.materialScheme(dark: Boolean): ColorScheme {
    val base = if (dark) darkColorScheme() else lightColorScheme()
    return base.copy(
        primary = brandA,
        onPrimary = Color.White,
        secondary = brandB,
        onSecondary = Color.White,
        tertiary = brandText,
        background = canvas,
        surface = surface,
        surfaceVariant = surfaceElevated,
        surfaceContainerLowest = surface,
        surfaceContainerLow = surface,
        surfaceContainer = surfaceElevated,
        outline = hairline,
        outlineVariant = hairline,
        error = negative,
    )
}

/** 生成アプリの見た目(ブランド色 + 面の色)。描画器の外側を 1 回これで包む。 */
@Composable
fun MetamonRenderTheme(dark: Boolean = isSystemInDarkTheme(), content: @Composable () -> Unit) {
    val colors = if (dark) RenderColors.Dark else RenderColors.Light
    CompositionLocalProvider(LocalRenderColors provides colors) {
        MaterialTheme(colorScheme = colors.materialScheme(dark), content = content)
    }
}

/** 面(カード): surface + card 角丸 + hairline 枠 + 控えめな影(iOS の `renderCard`)。 */
@Composable
fun Modifier.renderCard(padding: Dp = RenderTheme.Spacing.md, radius: Dp = RenderTheme.Radius.card): Modifier {
    val shape = RoundedCornerShape(radius)
    val colors = RenderTheme.colors
    return this
        .fillMaxWidth()
        .shadow(RenderTheme.Shadow.cardElevation, shape, clip = false)
        .clip(shape)
        .background(colors.surface, shape)
        .border(BorderStroke(1.dp, colors.hairline), shape)
        .padding(padding)
}
