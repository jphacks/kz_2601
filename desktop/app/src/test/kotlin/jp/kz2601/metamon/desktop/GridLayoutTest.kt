package jp.kz2601.metamon.desktop

import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.ComponentType
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/** 6 列の配置: area の読み(parse)・段の離れ(isDetached)・重なる部品の段(formBands)・寸法(resolve)。 */
class GridLayoutTest {
    @Test
    fun parseReadsAFullRange() {
        val a = GridArea.parse("A1:F12")!!
        assertEquals(GridArea(0, 1, 5, 12), a)
        assertEquals(6, a.colSpan)
        assertEquals(12, a.rowSpan)
    }

    @Test
    fun parseReadsASingleCell() {
        val a = GridArea.parse("C3:C3")!!
        assertEquals(GridArea(2, 3, 2, 3), a)
        assertEquals(1, a.colSpan)
        assertEquals(1, a.rowSpan)
    }

    @Test
    fun parseRejectsReversedRanges() {
        assertNull("列が逆", GridArea.parse("D1:B2"))
        assertNull("行が逆", GridArea.parse("A5:B2"))
    }

    @Test
    fun parseRejectsBrokenText() {
        for (s in listOf(null, "", "A1", "A1:B2:C3", "G1:G2", "a1:b2", "A:B2", "A0:B2", "A1:B-2", "A1:Bx", ":")) {
            assertNull("$s", GridArea.parse(s))
        }
    }

    @Test
    fun parseClampsRowsToTheMax() {
        assertEquals(GridArea(0, 1, 0, GridArea.MAX_ROWS), GridArea.parse("A1:A999"))
    }

    @Test
    fun adjacentBandsAreNotDetached() {
        val prev = Band(emptyList(), 1, 2)
        assertFalse("最初の段", prev.isDetached(null))
        assertFalse("すぐ下", Band(emptyList(), 3, 4).isDetached(prev))
        assertFalse("1 行空き", Band(emptyList(), 4, 4).isDetached(prev))
    }

    @Test
    fun bandsThreeRowsApartAreDetached() {
        val prev = Band(emptyList(), 1, 2)
        assertTrue("row1 - 前の row2 = 3", Band(emptyList(), 5, 5).isDetached(prev))
        assertTrue(Band(emptyList(), 9, 9).isDetached(prev))
    }

    @Test
    fun overlappingComponentsShareABandAndTheNextOneMovesBelow() {
        val first = placed("a", "A1:C2")
        val overlapping = placed("b", "D2:F3")
        val below = placed("c", "A5:F5")
        val bands = formBands(listOf(below, overlapping, first))
        assertEquals(2, bands.size)
        assertEquals(listOf("a", "b"), bands[0].items.map { it.component.id })
        assertEquals(1 to 3, bands[0].row1 to bands[0].row2)
        assertEquals(listOf("c"), bands[1].items.map { it.component.id })
        assertFalse(bands[1].isDetached(bands[0]))
    }

    @Test
    fun resolveCapsWidthAndClampsRowHeight() {
        val wide = DesktopGridMetrics.resolve(width = 1600f, height = 900f, inset = 12f)
        assertEquals(DesktopGridMetrics.MAX_WIDTH, wide.frameWidth, 0f)
        assertEquals((720f - 24f) / 6, wide.cellW, 0.001f)
        assertEquals((900f - 24f) / 12, wide.cellH, 0.001f)

        assertEquals(DesktopGridMetrics.MIN_CELL_HEIGHT, DesktopGridMetrics.resolve(400f, 200f, 0f).cellH, 0f)
        assertEquals(DesktopGridMetrics.MAX_CELL_HEIGHT, DesktopGridMetrics.resolve(400f, 4000f, 0f).cellH, 0f)
        assertEquals("余白が幅より大きい", 0f, DesktopGridMetrics.resolve(10f, 10f, 20f).cellW, 0f)
    }

    private fun placed(id: String, area: String): Placed =
        Placed(Component(id = id, type = ComponentType.TEXT, area = area), GridArea.parse(area)!!)
}
