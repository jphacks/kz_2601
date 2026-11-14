package jp.kz2601.metamon.dsl

import jp.kz2601.metamon.dsl.ai.AiSaveTo
import jp.kz2601.metamon.dsl.records.JSONValue
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import jp.kz2601.metamon.dsl.expr.Expr as EvalExpr
import jp.kz2601.metamon.dsl.records.Filter as RecordFilter

/** JsonElement のまま残るキーの読み口(#1007)。ある・無い・壊れている、で前と同じに読む。 */
class TypedReadersTest {
    private fun comp(extra: String): Component =
        DslJson.decodeFromString(Component.serializer(), """{"type":"progress"${if (extra.isEmpty()) "" else ",$extra"}}""")

    private fun field(extra: String): Field =
        DslJson.decodeFromString(Field.serializer(), """{"id":"f","name":"F","type":"string"${if (extra.isEmpty()) "" else ",$extra"}}""")

    private fun agg(extra: String): ExprAgg =
        DslJson.decodeFromString(ExprAgg.serializer(), """{"agg":"count","entity":"e"${if (extra.isEmpty()) "" else ",$extra"}}""")

    @Test
    fun targetNumberAndExpr() {
        comp(""""target":8""").let {
            assertEquals(8.0, it.targetNumber!!, 0.0)
            assertNull(it.targetExpr)
        }
        comp(""""target":{"setting":"goal"}""").let {
            assertNull(it.targetNumber)
            assertEquals(EvalExpr.Setting("goal"), it.targetExpr)
        }
        // 文字列は数として読まない(式のリテラル)
        comp(""""target":"8"""").let {
            assertNull(it.targetNumber)
            assertEquals(EvalExpr.Literal(JSONValue.Str("8")), it.targetExpr)
        }
        for (missing in listOf("", """"target":null""")) {
            assertNull(comp(missing).targetNumber)
            assertNull(comp(missing).targetExpr)
        }
        // 式として読めない形は null(部品は落とさない)
        comp(""""target":[1,2]""").let {
            assertNull(it.targetNumber)
            assertNull(it.targetExpr)
        }
    }

    @Test
    fun aiSaveTo() {
        assertEquals(AiSaveTo("idea", "title"), comp(""""saveTo":{"entity":"idea","field":"title"}""").aiSaveTo)
        assertNull(comp("").aiSaveTo)
        assertNull(comp(""""saveTo":{"entity":"idea"}""").aiSaveTo)
        assertNull(comp(""""saveTo":{"entity":"idea","field":3}""").aiSaveTo)
        assertNull(comp(""""saveTo":"idea"""").aiSaveTo)
    }

    @Test
    fun chatSaveTo() {
        assertEquals(AIChatSaveTo("meal", listOf("name", "kcal")), comp(""""saveTo":{"entity":"meal","fields":["name","kcal"]}""").chatSaveTo)
        // aiButton の形(fields 無し)は aiChat の saveTo でない
        assertNull(comp(""""saveTo":{"entity":"idea","field":"title"}""").chatSaveTo)
        assertNull(comp("").chatSaveTo)
        assertNull(comp(""""saveTo":{"fields":["a"]}""").chatSaveTo) // entity が無い
        assertNull(comp(""""saveTo":[1]""").chatSaveTo)
    }

    @Test
    fun fieldDefaultValue() {
        assertEquals(JSONValue.Str("now"), field(""""defaultValue":"now"""").defaultJSONValue)
        assertEquals(JSONValue.Num(3.0), field(""""defaultValue":3""").defaultJSONValue)
        assertEquals(JSONValue.Bool(true), field(""""defaultValue":true""").defaultJSONValue)
        assertNull(field("").defaultJSONValue)
        assertNull(field(""""defaultValue":null""").defaultJSONValue)
        // 生成した型の読み口と記録の型の読み方が同じ
        val raw = """{"id":"f","name":"F","type":"string","defaultValue":{"a":[1]}}"""
        assertEquals(
            jp.kz2601.metamon.dsl.records.Field.from(DslJson.parseToJsonElement(raw)).defaultValue,
            DslJson.decodeFromString(Field.serializer(), raw).defaultJSONValue,
        )
    }

    @Test
    fun aggFilters() {
        val one = agg(""""filter":{"field":"done","op":"eq","value":true}""").filters
        assertEquals(listOf(RecordFilter("done", "eq", JSONValue.Bool(true))), one)
        val many = agg(""""filter":[{"field":"a","op":"eq","value":1},{"field":"b","op":"today"}]""").filters
        assertEquals(listOf(RecordFilter("a", "eq", JSONValue.Num(1.0)), RecordFilter("b", "today")), many)
        // 壊れた要素だけ捨てる・無い/壊れた filter は空
        assertEquals(listOf(RecordFilter("b", "today")), agg(""""filter":[{"op":"eq"},{"field":"b","op":"today"}]""").filters)
        assertTrue(agg("").filters.isEmpty())
        assertTrue(agg(""""filter":"x"""").filters.isEmpty())
        assertTrue(agg(""""filter":{"field":"a"}""").filters.isEmpty())
    }
}
