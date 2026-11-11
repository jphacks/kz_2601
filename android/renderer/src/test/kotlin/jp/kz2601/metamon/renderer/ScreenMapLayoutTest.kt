package jp.kz2601.metamon.renderer

import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.decode
import jp.kz2601.metamon.renderer.navigation.ArrowGeometry
import jp.kz2601.metamon.renderer.navigation.MapPoint
import jp.kz2601.metamon.renderer.navigation.MapRect
import jp.kz2601.metamon.renderer.navigation.ScreenGraph
import jp.kz2601.metamon.renderer.navigation.ScreenMapLayout
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.File

/** #897: 画面マップのカードの並べ方と矢印の形(iOS / Mac の `ScreenMapLayout`・`ArrowGeometry` と同じ式)。 */
class ScreenMapLayoutTest {
    private val eps = 0.001f

    private fun example(name: String): AppDSL =
        AppDSL.decode(File(System.getProperty("metamon.dslDocsDir"), "examples/$name.json").readText())

    @Test
    fun cardsWidenToFitAndCenterPerRow() {
        // 段: [a, other] / [b, c]。幅 400: (400 - 32 - 18) / 2 = 175 → 最大 150
        val g = ScreenGraph(example("navigation"))
        val l = ScreenMapLayout(g, availableWidth = 400f)
        assertEquals(2, l.maxColumns)
        assertEquals(150f, l.cardWidth, eps)
        assertEquals(400f, l.contentWidth, eps)
        assertEquals(16f * 2 + 2 * 66f + 64f, l.contentHeight, eps)
        val a = l.frame(g.node("a")!!)
        // 段の幅 150*2+18 = 318 → 左端 (400-318)/2 = 41
        assertEquals(MapRect(41f, 16f, 150f, 66f), a)
        assertEquals(41f + 150f + 18f, l.frame(g.node("other")!!).x, eps)
        assertEquals(16f + 66f + 64f, l.frame(g.node("b")!!).y, eps)
    }

    @Test
    fun narrowWidthUsesMinCardAndScrolls() {
        val g = ScreenGraph(example("navigation"))
        val l = ScreenMapLayout(g, availableWidth = 200f)
        assertEquals("収まらなければ最小幅", 104f, l.cardWidth, eps)
        assertEquals("横スクロールのぶん広げる", 16f * 2 + 2 * 104f + 18f, l.contentWidth, eps)
    }

    @Test
    fun unreachableRowLeavesRoomForLabel() {
        val g = ScreenGraph(
            AppDSL.decode(
                """{"schemaVersion":1,"app":{"name":"t"},"initialScreen":"home","screens":[{"id":"home"},{"id":"island"}]}""",
            ),
        )
        val l = ScreenMapLayout(g, availableWidth = 360f)
        assertTrue(l.hasUnreachable)
        assertEquals(listOf(listOf("home"), listOf("island")), g.rows)
        assertEquals(16f + 66f + 64f + 26f, l.rowTop(1), eps)
        assertEquals(16f * 2 + 2 * 66f + 64f + 26f, l.contentHeight, eps)
        // 1 枚の段は中央
        assertEquals((360f - 150f) / 2, l.frame(g.node("home")!!).x, eps)
    }

    @Test
    fun arrowsCurveForReverseAndSameRowAndFadeUnselected() {
        val g = ScreenGraph(example("navigation"))
        val l = ScreenMapLayout(g, availableWidth = 400f)
        val all = l.arrows(selected = null).associateBy { it.link.id }
        assertEquals(g.links.map { it.id }, l.arrows(null).map { it.link.id })
        assertNull("下り(a->b)は直線", all.getValue("a->b").path.control)
        assertNotNull("往復(a->c / c->a)は曲げる", all.getValue("a->c").path.control)
        assertNotNull("上り(other->b は下りだが c->other は上り)", all.getValue("c->other").path.control)
        assertNotNull("同じ段(b->c)はカードの下を回る", all.getValue("b->c").path.control)
        val bc = all.getValue("b->c").path.control!!
        assertEquals(l.frame(g.node("b")!!).maxY + ArrowGeometry.BEND, bc.y, eps)
        assertTrue(all.values.all { it.active && !it.faded })

        val sel = l.arrows(selected = "b").associateBy { it.link.id }
        assertTrue(sel.getValue("a->b").active)
        assertFalse(sel.getValue("c->other").active)
        assertTrue(sel.getValue("c->other").faded)
    }

    @Test
    fun edgePointAndHead() {
        val r = MapRect(0f, 0f, 100f, 40f)
        // 右へ: 縁(50)+3 の外側
        assertEquals(MapPoint(103f, 20f), ArrowGeometry.edgePoint(r, MapPoint(500f, 20f)))
        // 下へ: 中心 20 + 縁(20)+3
        assertEquals(MapPoint(50f, 43f),ArrowGeometry.edgePoint(r, MapPoint(50f, 300f)))
        assertEquals(MapPoint(50f, 20f), ArrowGeometry.edgePoint(r, MapPoint(50f, 20f)))

        val p = ArrowGeometry.path(MapRect(0f, 0f, 100f, 40f), MapRect(0f, 200f, 100f, 40f), curved = false)
        assertNull(p.control)
        assertEquals(MapPoint(50f, 43f), p.start)
        assertEquals(MapPoint(50f, 197f), p.end)
        assertEquals(3, p.head.size)
        assertEquals(p.end, p.head[0])
        // 真下向きの矢じり: 根元は先端の HEAD_LENGTH 上・左右に 0.55 倍
        assertEquals(197f - ArrowGeometry.HEAD_LENGTH, p.head[1].y, eps)
        assertEquals(ArrowGeometry.HEAD_LENGTH * 0.55f, kotlin.math.abs(p.head[1].x - 50f), eps)
    }
}
