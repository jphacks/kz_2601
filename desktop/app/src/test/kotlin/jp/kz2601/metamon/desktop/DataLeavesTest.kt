package jp.kz2601.metamon.desktop

import jp.kz2601.metamon.dsl.TableColumn
import jp.kz2601.metamon.dsl.TableColumnAggregate
import jp.kz2601.metamon.dsl.records.Entity
import jp.kz2601.metamon.dsl.records.Field
import jp.kz2601.metamon.dsl.records.FieldCoding
import jp.kz2601.metamon.dsl.records.FieldType
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

/** 表の集計の行の文字・数の列の判定・iOS と同じ四捨五入(DataLeaves.kt・#2610)。 */
class DataLeavesTest {
    private val data = LeafData(emptyList(), { emptyList() })
    private val distance = Field(id = "distance", type = FieldType.NUMBER, unit = "km")
    private val plain = Field(id = "score", type = FieldType.NUMBER)
    private val yen = Field(id = "amount", type = FieldType.CURRENCY, unit = "円")
    private val entity = Entity(
        id = "log",
        fields = listOf(distance, plain, yen, Field(id = "count", type = FieldType.INTEGER), Field(id = "memo", type = FieldType.STRING)),
    )

    private fun text(value: Double?, op: String?, field: Field?) = TableCells.aggregateText(value, op, field, data)

    @Test
    fun swiftRoundedGoesAwayFromZeroOnHalves() {
        assertEquals(1.0, swiftRounded(0.5), 0.0)
        assertEquals(-1.0, swiftRounded(-0.5), 0.0)
        assertEquals(3.0, swiftRounded(2.5), 0.0)
        assertEquals(-3.0, swiftRounded(-2.5), 0.0)
        assertEquals(1.0, swiftRounded(1.4), 0.0)
        assertEquals(-1.0, swiftRounded(-1.4), 0.0)
        assertEquals(0.0, swiftRounded(0.0), 0.0)
    }

    @Test
    fun aggregateTextWithoutAValueIsUnavailable() {
        assertEquals(FieldCoding.UNAVAILABLE, text(null, "sum", distance))
        assertEquals(FieldCoding.UNAVAILABLE, text(null, "count", null))
    }

    @Test
    fun aggregateTextCountIsAPlainNumberWithoutUnit() {
        assertEquals("3", text(3.0, "count", distance))
        assertEquals("0", text(0.0, "count", null))
    }

    @Test
    fun aggregateTextSumUsesTheFieldFormat() {
        assertEquals("12.5 km", text(12.5, "sum", distance))
        assertEquals("60", text(60.0, "max", plain))
        assertEquals("¥1,200", text(1200.0, "sum", yen))
        assertEquals("12.35", text(12.345, "sum", null))
    }

    @Test
    fun aggregateTextAvgIsOneDecimalPlaceRoundedLikeSwift() {
        assertEquals("3.3 km", text(3.25, "avg", distance))
        assertEquals("3.3", text(3.333, "avg", plain))
        assertEquals("-0.3", text(-0.25, "avg", plain))
        assertEquals("3.3", text(3.25, "avg", null))
    }

    @Test
    fun aggregateTextAvgOfCurrencyIsWholeYen() {
        // 円は小数 1 桁にせず、円の書式(偶数丸め)に任せる
        assertEquals("¥1,234", text(1234.5, "avg", yen))
        assertEquals("¥1,236", text(1235.5, "avg", yen))
    }

    @Test
    fun isNumericForAggregatesAndNumberFields() {
        assertTrue("集計の列", TableCells.isNumeric(TableColumn(field = "memo", aggregate = TableColumnAggregate.COUNT), entity))
        assertTrue("entity が無くても集計なら数", TableCells.isNumeric(TableColumn(field = "x", aggregate = TableColumnAggregate.SUM), null))
        assertTrue(TableCells.isNumeric(TableColumn(field = "distance"), entity))
        assertTrue(TableCells.isNumeric(TableColumn(field = "amount"), entity))
        assertTrue(TableCells.isNumeric(TableColumn(field = "count"), entity))
    }

    @Test
    fun isNumericIsFalseForTextAndUnknownFields() {
        assertFalse(TableCells.isNumeric(TableColumn(field = "memo"), entity))
        assertFalse("知らない項目", TableCells.isNumeric(TableColumn(field = "nope"), entity))
        assertFalse("entity が無い", TableCells.isNumeric(TableColumn(field = "distance"), null))
    }
}
