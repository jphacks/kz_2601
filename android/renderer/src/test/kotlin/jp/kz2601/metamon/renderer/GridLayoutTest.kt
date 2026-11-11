package jp.kz2601.metamon.renderer

import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.ComponentType
import jp.kz2601.metamon.dsl.Screen
import jp.kz2601.metamon.renderer.layout.GridArea
import jp.kz2601.metamon.renderer.layout.GridMetrics
import jp.kz2601.metamon.renderer.layout.Placed
import jp.kz2601.metamon.renderer.layout.formBands
import jp.kz2601.metamon.renderer.layout.gridPlacement
import jp.kz2601.metamon.renderer.layout.isDetached
import jp.kz2601.metamon.renderer.layout.isFormLayout
import jp.kz2601.metamon.renderer.layout.localGridPlacement
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

class GridLayoutTest {
    @Test
    fun parsesAreas() {
        assertEquals(GridArea(0, 1, 5, 6), GridArea.parse("A1:F6"))
        val a = GridArea.parse("B3:D4")!!
        assertEquals(3, a.colSpan)
        assertEquals(2, a.rowSpan)
    }

    @Test
    fun rejectsInvalidAreas() {
        for (s in listOf(null, "", "A1", "A0:B1", "G1:G2", "C1:A2", "A3:B1", "a1:b2", "A1:B2:C3", "A:B2")) {
            assertNull(s, GridArea.parse(s))
        }
    }

    @Test
    fun capsRowsAt48() {
        assertEquals(GridArea(0, 1, 5, 48), GridArea.parse("A1:F999"))
    }

    @Test
    fun placedAndStacked() {
        val screen = Screen(
            id = "s",
            components = listOf(
                Component(ComponentType.TEXT, id = "b", area = "A3:F3"),
                Component(ComponentType.TEXT, id = "x"),
                Component(ComponentType.TEXT, id = "a", area = "D1:F2"),
                Component(ComponentType.TEXT, id = "c", area = "A1:C2"),
            ),
        )
        val p = screen.gridPlacement
        assertEquals(listOf("x"), p.stacked.map { it.id })
        assertEquals(listOf("c", "a", "b"), p.placedInReadingOrder.map { it.component.id })
        assertEquals(3, p.lastRow)
        assertFalse(screen.isFormLayout)
        assertTrue(screen.copy(entity = "e").isFormLayout)
    }

    @Test
    fun formBandsGroupOverlappingRows() {
        fun p(id: String, area: String) = Placed(Component(id = id), GridArea.parse(area)!!)
        val bands = formBands(listOf(p("del", "A11:F12"), p("r", "D1:F2"), p("l", "A1:C1"), p("next", "A3:F3")))
        assertEquals(listOf(listOf("l", "r"), listOf("next"), listOf("del")), bands.map { b -> b.items.map { it.component.id } })
        assertFalse(bands[1].isDetached(bands[0]))
        assertTrue(bands[2].isDetached(bands[1]))
    }

    @Test
    fun localGridOnlyWhenEveryChildHasArea() {
        val all = Component(ComponentType.CARD, children = listOf(Component(area = "A1:C1"), Component(area = "D1:F1")))
        assertEquals(2, all.localGridPlacement?.size)
        val partial = all.copy(children = all.children + Component())
        assertNull(partial.localGridPlacement)
        assertNull(all.copy(type = ComponentType.HSTACK).localGridPlacement)
    }

    @Test
    fun metricsPortraitAndLandscape() {
        val portrait = GridMetrics.resolve(width = 390f, height = 700f, inset = 14f, compactHeight = false)
        assertEquals((390f - 28f) / 6, portrait.cellW, 0.001f)
        assertEquals((700f - 28f) / 12, portrait.cellH, 0.001f)
        assertFalse(portrait.scrollsVertically)

        val landscape = GridMetrics.resolve(width = 844f, height = 360f, inset = 14f, compactHeight = true)
        assertEquals(640f, landscape.frameWidth, 0.001f)
        assertEquals(GridMetrics.LANDSCAPE_CELL_HEIGHT, landscape.cellH, 0.001f)
        assertTrue(landscape.scrollsVertically)
    }
}
