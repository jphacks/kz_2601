package jp.kz2601.metamon.desktop

import androidx.compose.ui.input.key.Key
import jp.kz2601.metamon.desktop.i18n.Lang
import jp.kz2601.metamon.desktop.i18n.Strings
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/** carousel の送り(‹ ›・← →・点)の番号。端で止まる。 */
class CarouselPagerTest {
    @Test
    fun stopsAtEnds() {
        assertEquals(0, CarouselPager.prev(0, 3))
        assertEquals(1, CarouselPager.next(0, 3))
        assertEquals(2, CarouselPager.next(2, 3))
        assertEquals(1, CarouselPager.prev(2, 3))
        assertFalse(CarouselPager.canPrev(0, 3))
        assertTrue(CarouselPager.canNext(0, 3))
        assertFalse(CarouselPager.canNext(2, 3))
    }

    @Test
    fun clampsWhenPagesShrink() {
        assertEquals("子が減ったら最後の 1 枚へ", 1, CarouselPager.clamp(4, 2))
        assertEquals(0, CarouselPager.clamp(3, 0))
        assertFalse(CarouselPager.canNext(0, 1))
        assertFalse(CarouselPager.canPrev(0, 1))
    }

    @Test
    fun arrowKeysMoveOthersPass() {
        assertEquals(1, CarouselPager.onKey(Key.DirectionRight, 0, 3))
        assertEquals(0, CarouselPager.onKey(Key.DirectionLeft, 0, 3))
        assertNull(CarouselPager.onKey(Key.Enter, 0, 3))
    }

    @Test
    fun pageLabelInBothLanguages() {
        assertEquals("2 / 5 枚目", Strings.withLang(Lang.JA) { CarouselPager.pageLabel(1, 5) })
        assertEquals("2 of 5", Strings.withLang(Lang.EN) { CarouselPager.pageLabel(1, 5) })
    }
}
