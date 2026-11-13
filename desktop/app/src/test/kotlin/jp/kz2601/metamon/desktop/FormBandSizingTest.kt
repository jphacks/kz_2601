package jp.kz2601.metamon.desktop

import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.ComponentType
import org.junit.Assert.assertEquals
import org.junit.Test

/** フォームの段の最小の高さ: 表示の部品は 2 行ぶん、icon・入力は中身の高さ。 */
class FormBandSizingTest {
    @Test
    fun iconInAFormBandKeepsItsOwnHeight() {
        assertEquals(0, FormBandSizing.minRows(Component(type = ComponentType.ICON)))
    }

    @Test
    fun otherDisplayLeavesStillTakeTwoRows() {
        for (t in listOf(ComponentType.IMAGE, ComponentType.CHART, ComponentType.STAT, ComponentType.CALENDAR)) {
            assertEquals(t.name, 2, FormBandSizing.minRows(Component(type = t)))
        }
        assertEquals("表示の rating", 2, FormBandSizing.minRows(Component(type = ComponentType.RATING)))
    }

    @Test
    fun inputsKeepTheirOwnHeight() {
        assertEquals("入力の rating", 0, FormBandSizing.minRows(Component(type = ComponentType.RATING, field = "score")))
        assertEquals(0, FormBandSizing.minRows(Component(type = ComponentType.TEXT_FIELD, field = "name")))
        assertEquals(0, FormBandSizing.minRows(Component(type = ComponentType.TEXT)))
    }

    @Test
    fun bandRowsKeepTheirCountAtTwiceTheTextSize() {
        // 段の行数は部品で決まり、1 行の高さが文字の倍率で伸びる(#2208)。倍率 2.0 でも 2 行の段は 1 行の倍率ぶんの 2 倍
        assertEquals(2, FormBandSizing.minRows(Component(type = ComponentType.CHART)))
        val cellH = DesktopGridMetrics.resolve(480f, 400f, 16f, fontScale = 2f).cellH
        assertEquals(DesktopGridMetrics.MIN_CELL_HEIGHT * 2 * 2, cellH * FormBandSizing.minRows(Component(type = ComponentType.CHART)), 1e-3f)
    }
}
