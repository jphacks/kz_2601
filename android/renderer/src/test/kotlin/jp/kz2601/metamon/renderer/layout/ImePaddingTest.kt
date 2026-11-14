package jp.kz2601.metamon.renderer.layout

import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.ui.unit.Density
import org.junit.Assert.assertEquals
import org.junit.Test

class ImePaddingTest {
    private val density = Density(1f)

    private fun lift(imePx: Int, belowPx: Int) = imeExcludingBelow(WindowInsets(bottom = imePx), belowPx).getBottom(density)

    @Test
    fun `下に帯があればその分だけ引く`() {
        assertEquals(700, lift(imePx = 900, belowPx = 200))
    }

    @Test
    fun `下に何も無ければキーボードの高さそのまま`() {
        assertEquals(900, lift(imePx = 900, belowPx = 0))
    }

    @Test
    fun `キーボードが閉じていれば帯があっても空けない`() {
        assertEquals(0, lift(imePx = 0, belowPx = 200))
    }

    @Test
    fun `下の分がキーボードより高ければ空けない`() {
        assertEquals(0, lift(imePx = 150, belowPx = 200))
    }

    @Test
    fun `下の分は窓の下端までの距離で端数は切り捨て、はみ出しは0`() {
        assertEquals(120, distanceBelow(rootHeight = 2000, bottom = 1879.6f))
        assertEquals(0, distanceBelow(rootHeight = 2000, bottom = 2000f))
        assertEquals(0, distanceBelow(rootHeight = 2000, bottom = 2050f))
    }
}
