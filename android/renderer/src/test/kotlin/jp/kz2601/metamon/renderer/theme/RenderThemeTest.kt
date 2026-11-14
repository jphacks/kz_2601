package jp.kz2601.metamon.renderer.theme

import androidx.compose.material3.ColorScheme
import androidx.compose.material3.darkColorScheme
import androidx.compose.material3.lightColorScheme
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.unit.dp
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotEquals
import org.junit.Assert.assertTrue
import org.junit.Test

/** 色が iOS の SemanticPalette と同じ値で、Material 3 の既定の紫が残っていないこと(#842)。 */
class RenderThemeTest {
    private fun rgb(r: Int, g: Int, b: Int) = Color(r, g, b)

    @Test
    fun colorsMatchIosSemanticPalette() {
        val l = RenderColors.Light
        val d = RenderColors.Dark
        assertEquals(rgb(46, 106, 147), l.brandA); assertEquals(rgb(78, 141, 184), d.brandA)
        assertEquals(rgb(62, 154, 156), l.brandB); assertEquals(rgb(59, 158, 159), d.brandB)
        assertEquals(rgb(31, 90, 130), l.brandText); assertEquals(rgb(140, 195, 230), d.brandText)
        assertEquals(rgb(246, 243, 236), l.canvas); assertEquals(rgb(15, 26, 34), d.canvas)
        assertEquals(rgb(255, 253, 248), l.surface); assertEquals(rgb(22, 35, 45), d.surface)
        assertEquals(rgb(249, 246, 239), l.surfaceElevated); assertEquals(rgb(30, 46, 58), d.surfaceElevated)
        assertEquals(rgb(190, 28, 22), l.negative); assertEquals(rgb(255, 105, 97), d.negative)
        assertEquals(rgb(16, 112, 48), l.positive); assertEquals(rgb(72, 214, 110), d.positive)
        assertEquals(rgb(150, 76, 0), l.warning); assertEquals(rgb(255, 170, 60), d.warning)
        assertEquals(rgb(0, 84, 190), l.info); assertEquals(rgb(90, 160, 255), d.info)
        assertEquals(8, l.chartPalette.size); assertEquals(8, d.chartPalette.size)
    }

    @Test
    fun materialSchemeHasNoBaselinePurple() {
        for ((colors, dark) in listOf(RenderColors.Light to false, RenderColors.Dark to true)) {
            val scheme = colors.materialScheme(dark)
            val baseline = if (dark) darkColorScheme() else lightColorScheme()
            for ((name, pick) in roles) {
                assertNotEquals("$name が Material の既定のまま(dark=$dark)", pick(baseline), pick(scheme))
                assertTrue("$name は不透明(dark=$dark)", name == "scrim" || pick(scheme).alpha == 1f)
            }
            assertEquals(colors.brandA, scheme.primary)
            assertEquals(colors.canvas, scheme.background)
            assertEquals(colors.surface, scheme.surface)
            assertEquals(colors.negative, scheme.error)
        }
    }

    @Test
    fun adaptiveLayoutMatchesIos() {
        assertEquals(560.dp, AdaptiveLayout.readableWidth)
        assertTrue(AdaptiveLayout.isCompactHeight(400))
        assertFalse(AdaptiveLayout.isCompactHeight(800))
        assertEquals(132.dp, AdaptiveLayout.micSize(isInitial = true, compactHeight = false))
        assertEquals(84.dp, AdaptiveLayout.micSize(isInitial = false, compactHeight = true))
    }

    private val roles: List<Pair<String, (ColorScheme) -> Color>> = listOf(
        "primary" to { it.primary },
        "primaryContainer" to { it.primaryContainer },
        "onPrimaryContainer" to { it.onPrimaryContainer },
        "secondary" to { it.secondary },
        "secondaryContainer" to { it.secondaryContainer },
        "tertiary" to { it.tertiary },
        "tertiaryContainer" to { it.tertiaryContainer },
        "background" to { it.background },
        "surface" to { it.surface },
        "onSurfaceVariant" to { it.onSurfaceVariant },
        "surfaceVariant" to { it.surfaceVariant },
        "surfaceTint" to { it.surfaceTint },
        "surfaceContainer" to { it.surfaceContainer },
        "surfaceContainerHigh" to { it.surfaceContainerHigh },
        "surfaceContainerHighest" to { it.surfaceContainerHighest },
        "outline" to { it.outline },
        "outlineVariant" to { it.outlineVariant },
        "error" to { it.error },
        "errorContainer" to { it.errorContainer },
    )
}
