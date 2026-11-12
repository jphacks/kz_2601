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
}
