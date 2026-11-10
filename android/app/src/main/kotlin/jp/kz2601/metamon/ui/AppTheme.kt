package jp.kz2601.metamon.ui

import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.drawBehind
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.unit.dp
import jp.kz2601.metamon.renderer.theme.RenderColors
import jp.kz2601.metamon.renderer.theme.RenderTheme
import jp.kz2601.metamon.renderer.theme.colors

/**
 * アプリ本体(音声画面・サインイン・生成中・設定)のトークン。iOS の App/Theme.swift の写し。
 * 色の正典は描画器の [RenderColors](本体と生成アプリを同じトーンに揃える・#102)。ここは本体だけの色と形。
 */
object AppTheme {
    object Spacing {
        val xxl = 48.dp
    }

    object Radius {
        val sm = 10.dp
        val md = 16.dp
        val card = 22.dp
        val lg = 28.dp
    }

    /** 録音中(赤〜珊瑚)。 */
    val recordA = Color(0xFFFF5F6D)
    val recordB = Color(0xFFFF9966)

    val recordGradient: Brush get() = Brush.linearGradient(listOf(recordA, recordB))

    /** ブランドのグラデーション(左上 → 右下)。押せる・選ばれている・いま注目の 3 つだけに使う。 */
    fun brandGradient(colors: RenderColors): Brush = Brush.linearGradient(listOf(colors.brandA, colors.brandB))
}

/** 音声画面の背景(淡いブランド色の滲み + キャンバス)。滲みの位置と半径は iOS と同じ(左上 20%/10%・420pt、右下 90%/90%・380pt)。 */
@Composable
fun AppBackdrop(modifier: Modifier = Modifier, content: @Composable () -> Unit) {
    val c = RenderTheme.colors
    val dark = isSystemInDarkTheme()
    Box(
        modifier
            .fillMaxSize()
            .drawBehind {
                drawRect(c.canvas)
                val glowA = c.brandA.copy(alpha = if (dark) 0.28f else 0.10f)
                val glowB = c.brandB.copy(alpha = if (dark) 0.22f else 0.08f)
                drawRect(Brush.radialGradient(listOf(glowA, Color.Transparent), Offset(size.width * 0.2f, size.height * 0.1f), 420.dp.toPx()))
                drawRect(Brush.radialGradient(listOf(glowB, Color.Transparent), Offset(size.width * 0.9f, size.height * 0.9f), 380.dp.toPx()))
            },
    ) { content() }
}
