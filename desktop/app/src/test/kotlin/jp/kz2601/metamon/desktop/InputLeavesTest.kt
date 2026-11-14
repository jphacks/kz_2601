package jp.kz2601.metamon.desktop

import androidx.compose.ui.graphics.Color
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

/** slider / stepper の値の丸めと colorPicker の 16 進の読み方(#2607)。 */
class InputLeavesTest {
    private fun eq(expected: Double, actual: Double) = assertEquals(expected, actual, 0.0)

    @Test
    fun roundToStepDropsFloatingPointNoise() {
        eq(70.3, roundToStep(70.30000000001, 0.1))
        eq(0.3, roundToStep(0.1 + 0.2, 0.1))
        eq(1.25, roundToStep(1.2500000001, 0.05))
        eq(0.0000001, roundToStep(0.00000010000001, 1e-7))
    }

    @Test
    fun roundToStepIsHalfUpAwayFromZero() {
        eq(3.0, roundToStep(2.5, 1.0))
        eq(-3.0, roundToStep(-2.5, 1.0))
        eq(-1.2, roundToStep(-1.2000000001, 0.1))
        eq(-0.3, roundToStep(-0.25, 0.1))
    }

    @Test
    fun roundToStepUsesOnlyTheDecimalPlacesOfStep() {
        // 整数の step は整数に丸めるだけで、step の倍数には寄せない(寄せるのは snap)
        eq(15.0, roundToStep(15.0, 10.0))
        eq(2.0, roundToStep(1.5, 1.0))
        // step が 0 は小数 0 桁
        eq(1.0, roundToStep(1.4, 0.0))
        eq(2.0, roundToStep(1.5, 0.0))
    }

    @Test
    fun snapMovesToStepsCountedFromMin() {
        eq(0.3, snap(0.34, 0.0, 0.1))
        eq(0.1, snap(0.05, 0.0, 0.1))
        eq(6.0, snap(7.0, 1.0, 5.0))
        eq(11.0, snap(9.0, 1.0, 5.0))
        eq(-0.3, snap(-0.34, -1.0, 0.1))
    }

    @Test
    fun snapWithoutAPositiveStepKeepsTheValue() {
        eq(0.34, snap(0.34, 0.0, null))
        eq(0.34, snap(0.34, 0.0, 0.0))
        eq(0.34, snap(0.34, 0.0, -0.1))
    }

    @Test
    fun boundClampsToEitherSide() {
        eq(5.0, bound(5.0, null, null))
        eq(0.0, bound(-1.0, 0.0, null))
        eq(100.0, bound(100.0, 0.0, null))
        eq(10.0, bound(11.0, null, 10.0))
        eq(-50.0, bound(-50.0, null, 10.0))
        eq(3.0, bound(3.0, 0.0, 10.0))
    }

    @Test
    fun boundWithReversedMinMaxEndsAtMax() {
        // min の後に max で抑えるので、逆転していると max になる
        eq(0.0, bound(5.0, 10.0, 0.0))
        eq(0.0, bound(-5.0, 10.0, 0.0))
        eq(0.0, bound(20.0, 10.0, 0.0))
    }

    @Test
    fun parseHexReadsSixDigitsWithOrWithoutHash() {
        assertEquals(Color(0xFFE53935), parseHex("#E53935"))
        assertEquals(Color(0xFFE53935), parseHex("E53935"))
        assertEquals(Color(0xFFE53935), parseHex("#e53935"))
        assertEquals(Color(0xFF000000), parseHex("#000000"))
        assertEquals(Color(0xFFFFFFFF), parseHex("#FFFFFF"))
    }

    @Test
    fun parseHexRejectsOtherLengthsAndBadCharacters() {
        assertNull(parseHex("#FFF"))
        assertNull(parseHex("FFF"))
        assertNull(parseHex("#FFE53935"))
        assertNull(parseHex("#GGGGGG"))
        assertNull(parseHex("#E5 935"))
        assertNull(parseHex("##E53935"))
        assertNull(parseHex(""))
        assertNull(parseHex("#"))
    }
}
