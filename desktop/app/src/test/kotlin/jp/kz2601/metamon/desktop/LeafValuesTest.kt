package jp.kz2601.metamon.desktop

import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.ComponentAggregate
import jp.kz2601.metamon.dsl.ComponentType
import jp.kz2601.metamon.dsl.ExprNumber
import jp.kz2601.metamon.dsl.ExprString
import jp.kz2601.metamon.dsl.Tone
import jp.kz2601.metamon.dsl.records.Entity
import jp.kz2601.metamon.dsl.records.Field
import jp.kz2601.metamon.dsl.records.FieldCoding
import jp.kz2601.metamon.dsl.records.FieldType
import jp.kz2601.metamon.dsl.records.JSONValue
import jp.kz2601.metamon.dsl.records.Record
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/** stat / gauge / progressBar の値の出し方(LeafValues.kt・#2608)。 */
class LeafValuesTest {
    private val empty = LeafData(emptyList(), { emptyList() })
    private val goalExpr = JsonObject(mapOf("setting" to JsonPrimitive("goal")))

    private fun settings(goal: Double): LeafData {
        val single = Entity(id = "settings", single = true, fields = listOf(Field(id = "goal", type = FieldType.NUMBER)))
        val rows = listOf(Record(id = "s", data = mapOf("goal" to JSONValue.Num(goal))))
        return LeafData(listOf(single), { if (it == "settings") rows else emptyList() })
    }

    private fun progress(entity: String? = "expense", target: kotlinx.serialization.json.JsonElement? = null, value: Double? = null) =
        Component(type = ComponentType.PROGRESS_BAR, entity = entity, field = "amount", target = target, value = value?.let { ExprNumber(it) })

    @Test
    fun statIsValueModeOnlyWithValueAndNoAggregate() {
        assertTrue(StatValue.isValueMode(Component(type = ComponentType.STAT, value = ExprNumber(3.0))))
        assertFalse(StatValue.isValueMode(Component(type = ComponentType.STAT, value = ExprNumber(3.0), aggregate = ComponentAggregate.SUM)))
        assertFalse(StatValue.isValueMode(Component(type = ComponentType.STAT, aggregate = ComponentAggregate.COUNT)))
        assertFalse(StatValue.isValueMode(Component(type = ComponentType.STAT)))
    }

    @Test
    fun literalIsOnlyANumberValue() {
        assertEquals(0.4, ProgressValue.literal(progress(value = 0.4))!!, 0.0)
        assertNull(ProgressValue.literal(progress()))
        assertNull(ProgressValue.literal(Component(type = ComponentType.PROGRESS_BAR, value = ExprString("0.4"))))
    }

    @Test
    fun missingTargetWhenTheTargetExpressionHasNoValue() {
        assertTrue("設定が無い", ProgressValue.missingTarget(progress(target = goalExpr), empty))
        assertTrue("設定が 0(正でない)", ProgressValue.missingTarget(progress(target = goalExpr), settings(0.0)))
        assertFalse(ProgressValue.missingTarget(progress(target = goalExpr), settings(50.0)))
        assertEquals(FieldCoding.UNAVAILABLE, ProgressValue.percent(progress(target = goalExpr), empty))
    }

    @Test
    fun noMissingTargetWithoutATargetExpression() {
        assertFalse("target が無い", ProgressValue.missingTarget(progress(), empty))
        assertFalse("数のリテラル(0 でも式ではない)", ProgressValue.missingTarget(progress(target = JsonPrimitive(0)), empty))
        assertFalse("値のリテラル", ProgressValue.missingTarget(progress(target = goalExpr, value = 0.5), empty))
        assertFalse("entity が無い", ProgressValue.missingTarget(progress(entity = null, target = goalExpr), empty))
        assertEquals("0%", ProgressValue.percent(progress(target = JsonPrimitive(0)), empty))
    }

    @Test
    fun percentTextWithoutToneStaysBetween0And100() {
        assertEquals("0%", ProgressValue.percentText(0.0, null))
        assertEquals("0%", ProgressValue.percentText(-0.2, null))
        assertEquals("42%", ProgressValue.percentText(0.42, null))
        assertEquals("100%", ProgressValue.percentText(1.0, null))
        assertEquals("100%", ProgressValue.percentText(1.5, null))
    }

    @Test
    fun percentTextWithToneShowsTheRealRatioUpTo999() {
        assertEquals("150%", ProgressValue.percentText(1.5, Tone.POSITIVE.wire))
        assertEquals("0%", ProgressValue.percentText(-1.0, Tone.NEGATIVE.wire))
        assertEquals("999%", ProgressValue.percentText(9.99, Tone.POSITIVE.wire))
        assertEquals("999%+", ProgressValue.percentText(12.0, Tone.POSITIVE.wire))
        // 知らない tone("")も tone ありとして扱う
        assertEquals("150%", ProgressValue.percentText(1.5, Tone.UNKNOWN.wire))
    }

    @Test
    fun percentTextRoundsHalfToEven() {
        assertEquals("12%", ProgressValue.percentText(0.125, null))
        assertEquals("38%", ProgressValue.percentText(0.375, null))
        assertEquals("250%", ProgressValue.percentText(2.5, Tone.POSITIVE.wire))
    }

    @Test
    fun percentTextWithNanAndInfinity() {
        assertEquals("100%", ProgressValue.percentText(Double.POSITIVE_INFINITY, null))
        assertEquals("0%", ProgressValue.percentText(Double.NEGATIVE_INFINITY, null))
        assertEquals("999%+", ProgressValue.percentText(Double.POSITIVE_INFINITY, Tone.POSITIVE.wire))
        assertEquals("0%", ProgressValue.percentText(Double.NEGATIVE_INFINITY, Tone.POSITIVE.wire))
        // NaN は丸めずそのまま出る(target は正の有限に絞るので通常は来ない)
        assertEquals("NaN%", ProgressValue.percentText(Double.NaN, null))
        assertEquals("NaN%", ProgressValue.percentText(Double.NaN, Tone.POSITIVE.wire))
    }

    @Test
    fun typedValueNumbers() {
        assertEquals(JSONValue.Num(1234.0), typedValue("1,234", FieldType.INTEGER))
        assertEquals(JSONValue.Num(-0.5), typedValue("-0.5", FieldType.NUMBER))
        assertEquals(JSONValue.Num(1200.0), typedValue("1200", FieldType.CURRENCY))
        assertEquals("読めない数は文字のまま", JSONValue.Str("abc"), typedValue("abc", FieldType.NUMBER))
        assertEquals(JSONValue.Str(""), typedValue("", FieldType.NUMBER))
    }

    @Test
    fun typedValueBooleanIsOnlyLowercaseTrue() {
        assertEquals(JSONValue.Bool(true), typedValue("true", FieldType.BOOLEAN))
        assertEquals(JSONValue.Bool(false), typedValue("false", FieldType.BOOLEAN))
        assertEquals(JSONValue.Bool(false), typedValue("TRUE", FieldType.BOOLEAN))
        assertEquals(JSONValue.Bool(false), typedValue("", FieldType.BOOLEAN))
    }

    @Test
    fun typedValueOtherTypesStayStrings() {
        assertEquals(JSONValue.Str("2026-11-13"), typedValue("2026-11-13", FieldType.DATE))
        assertEquals(JSONValue.Str("2026-11-13T09:00"), typedValue("2026-11-13T09:00", FieldType.DATETIME))
        assertEquals(JSONValue.Str("12"), typedValue("12", FieldType.STRING))
        assertEquals(JSONValue.Str("12"), typedValue("12", null))
        assertEquals(JSONValue.Str(""), typedValue("", FieldType.DATE))
    }
}
