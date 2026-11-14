package jp.kz2601.metamon.renderer.layout

import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.ComponentType
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

/** #1003: セル内スクロールの端のフェード・出現の遅延・list を行の高さで止める(iOS の `FadingScrollView` / `StaggeredAppear` / `hugsContent`)。 */
class GridCellRulesTest {
    private val slack = 2f

    @Test
    fun `はみ出さない中身はどちらもフェードしない`() {
        assertEquals(EdgeFade.Edges(top = false, bottom = false), EdgeFade.edges(0f, 0f, 0f, slack))
    }

    @Test
    fun `先頭では下だけ・途中は両方・末尾では上だけ`() {
        assertEquals(EdgeFade.Edges(top = false, bottom = true), EdgeFade.edges(0f, 300f, 0f, slack))
        assertEquals(EdgeFade.Edges(top = true, bottom = true), EdgeFade.edges(150f, 300f, 0f, slack))
        assertEquals(EdgeFade.Edges(top = true, bottom = false), EdgeFade.edges(300f, 300f, 0f, slack))
    }

    @Test
    fun `誤差ぶんのはみ出しはフェードしない`() {
        assertEquals(EdgeFade.Edges(top = false, bottom = false), EdgeFade.edges(0f, 2f, 0f, slack))
        assertEquals(EdgeFade.Edges(top = false, bottom = true), EdgeFade.edges(0f, 3f, 0f, slack))
        assertEquals(EdgeFade.Edges(top = false, bottom = false), EdgeFade.edges(2f, 2f, 0f, slack))
    }

    @Test
    fun `FAB ぶんの末尾の余白だけが残っている時は下をフェードしない`() {
        // 中身は 100 はみ出し、うち 80 は FAB の余白
        assertEquals(EdgeFade.Edges(top = true, bottom = false), EdgeFade.edges(20f, 100f, 80f, slack))
        assertEquals(EdgeFade.Edges(top = false, bottom = true), EdgeFade.edges(0f, 100f, 80f, slack))
        // 余白だけで中身ははみ出さない
        assertEquals(EdgeFade.Edges(top = false, bottom = false), EdgeFade.edges(0f, 80f, 80f, slack))
    }

    @Test
    fun `出現の遅延は index × stagger で 12 番目以降は同じ`() {
        assertEquals(0, AppearStagger.delayMillis(0, 30))
        assertEquals(90, AppearStagger.delayMillis(3, 30))
        assertEquals(360, AppearStagger.delayMillis(12, 30))
        assertEquals(360, AppearStagger.delayMillis(40, 30))
        assertEquals(0, AppearStagger.delayMillis(-1, 30))
    }

    @Test
    fun `アニメーションを減らす設定と光らせる画面では出現を動かさない`() {
        assertTrue(AppearStagger.animates(reduceMotion = false, screenHighlighted = false))
        assertFalse(AppearStagger.animates(reduceMotion = true, screenHighlighted = false))
        assertFalse(AppearStagger.animates(reduceMotion = false, screenHighlighted = true))
        assertFalse(AppearStagger.animates(reduceMotion = true, screenHighlighted = true))
    }

    private fun list(id: String) = Component(ComponentType.LIST, id = id, entity = id)

    @Test
    fun `行のある list は行の高さで止める・空なら器いっぱい`() {
        assertTrue(RowHeightStop.hugsContent(list("a")) { true })
        assertFalse(RowHeightStop.hugsContent(list("a")) { false })
    }

    @Test
    fun `list だけの section は全部に行がある時だけ止める`() {
        val section = Component(ComponentType.SECTION, id = "s", children = listOf(list("a"), list("b")))
        assertTrue(RowHeightStop.hugsContent(section) { true })
        assertFalse(RowHeightStop.hugsContent(section) { it.id == "a" })
    }

    @Test
    fun `list 以外を含む section・空の section・他の型は止めない`() {
        val mixed = Component(ComponentType.SECTION, id = "s", children = listOf(list("a"), Component(ComponentType.TEXT, id = "t")))
        assertTrue(RowHeightStop.lists(mixed).isEmpty())
        assertFalse(RowHeightStop.hugsContent(mixed) { true })
        assertFalse(RowHeightStop.hugsContent(Component(ComponentType.SECTION, id = "e")) { true })
        assertFalse(RowHeightStop.hugsContent(Component(ComponentType.TEXT, id = "t")) { true })
    }
}
