package jp.kz2601.metamon.desktop

import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.ComponentType
import org.junit.Assert.assertEquals
import org.junit.Test

/** #3215: grid は columnCount の列で左から詰めて並べる(iOS と同じ)。 */
class GridColumns3215Test {
    @Test
    fun twoColumnsFillLeftToRight() {
        assertEquals(listOf(0 to 0, 0 to 1, 1 to 0, 1 to 1, 2 to 0), gridColumnCells(5, 2))
    }

    @Test
    fun threeColumnsFillLeftToRight() {
        assertEquals(listOf(0 to 0, 0 to 1, 0 to 2, 1 to 0, 1 to 1), gridColumnCells(5, 3))
        assertEquals(emptyList<Pair<Int, Int>>(), gridColumnCells(0, 3))
    }

    @Test
    fun columnCountIsClampedToTwoThroughFourAndDefaultsToTwo() {
        fun grid(n: Int?) = Component(type = ComponentType.GRID, columnCount = n)
        assertEquals(2, grid(null).gridColumns)
        assertEquals(2, grid(1).gridColumns)
        assertEquals(3, grid(3).gridColumns)
        assertEquals(4, grid(9).gridColumns)
    }
}
