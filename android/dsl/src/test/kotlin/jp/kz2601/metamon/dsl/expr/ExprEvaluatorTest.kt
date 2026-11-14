package jp.kz2601.metamon.dsl.expr

import jp.kz2601.metamon.dsl.records.Aggregator
import jp.kz2601.metamon.dsl.records.Entity
import jp.kz2601.metamon.dsl.records.FieldCoding
import jp.kz2601.metamon.dsl.records.Filter
import jp.kz2601.metamon.dsl.records.Fixtures
import jp.kz2601.metamon.dsl.records.Fixtures.rec
import jp.kz2601.metamon.dsl.records.Fixtures.str
import jp.kz2601.metamon.dsl.records.JSONValue
import jp.kz2601.metamon.dsl.records.Record
import jp.kz2601.metamon.dsl.records.Settlement
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertThrows
import org.junit.Test
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import java.time.Instant

/** iOS の ExprEvaluatorTests を写したもの(同じ入力 → 同じ出力)。 */
class ExprEvaluatorTest {
    private val savedZone = FieldCoding.zone

    init {
        FieldCoding.zone = Fixtures.TOKYO
    }

    @After
    fun restore() {
        FieldCoding.zone = savedZone
    }

    private fun expr(json: String) = Expr.parse(json)

    private fun eval(
        json: String,
        row: ExprEvaluator.Row? = null,
        records: Map<String, List<Record>> = emptyMap(),
        entities: List<Entity> = emptyList(),
        now: Instant = Instant.now(),
    ): ExprValue? = ExprEvaluator(entities, { records[it] ?: emptyList() }, now).evaluate(expr(json), row)

    private fun num(json: String) = eval(json)?.number

    // MARK: - 算術

    @Test
    fun arithmetic() {
        assertEquals(6.0, num("""{"op":"add","args":[1,2,3]}"""))
        assertEquals(6.0, num("""{"op":"sub","args":[10,4]}"""))
        assertEquals(24.0, num("""{"op":"mul","args":[2,3,4]}"""))
        assertEquals(3.0, num("""{"op":"div","args":[9,3]}"""))
        assertEquals(1.0, num("""{"op":"mod","args":[10,3]}"""))
        assertEquals(-5.0, num("""{"op":"neg","args":[5]}"""))
        assertEquals(5.0, num("""{"op":"abs","args":[-5]}"""))
        assertEquals(4.0, num("""{"op":"sqrt","args":[16]}"""))
        assertEquals(1024.0, num("""{"op":"pow","args":[2,10]}"""))
        assertEquals(3.0, num("""{"op":"round","args":[2.5]}"""))
        assertEquals(3.14, num("""{"op":"round","args":[3.14159,2]}"""))
        assertEquals(2.0, num("""{"op":"floor","args":[2.9]}"""))
        assertEquals(3.0, num("""{"op":"ceil","args":[2.1]}"""))
        assertEquals(1.0, num("""{"op":"min","args":[3,1,2]}"""))
        assertEquals(3.0, num("""{"op":"max","args":[3,1,2]}"""))
        assertEquals(10.0, num("""{"op":"clamp","args":[15,0,10]}"""))
        assertEquals(0.0, num("""{"op":"clamp","args":[-1,0,10]}"""))
        assertEquals("数字文字列は数値として扱う", 1500.0, num("""{"op":"add","args":["1,200",300]}"""))
    }

    @Test
    fun unavailableIsNull() {
        assertNull("ゼロ除算", eval("""{"op":"div","args":[1,0]}"""))
        assertNull(eval("""{"op":"mod","args":[1,0]}"""))
        assertNull(eval("""{"op":"sqrt","args":[-1]}"""))
        assertNull("型不一致", eval("""{"op":"add","args":[1,"abc"]}"""))
        assertNull("参照なし", eval("""{"op":"add","args":[1,{"ref":"missing"}]}""", row = ExprEvaluator.Row(null, Record("r"))))
        assertNull("行コンテキスト無しの ref", eval("""{"ref":"x"}"""))
        assertNull("未知 op", eval("""{"op":"nope","args":[1]}"""))
        assertNull("null は伝播する", eval("""{"op":"mul","args":[{"op":"div","args":[1,0]},2]}"""))
    }

    // MARK: - 比較・論理・if

    @Test
    fun comparisonLogicAndIf() {
        val t = ExprValue.Bool(true)
        val f = ExprValue.Bool(false)
        assertEquals(t, eval("""{"op":"eq","args":[1,1]}"""))
        assertEquals(t, eval("""{"op":"eq","args":["a","a"]}"""))
        assertEquals(t, eval("""{"op":"neq","args":[1,2]}"""))
        assertEquals(t, eval("""{"op":"gt","args":[2,1]}"""))
        assertEquals(t, eval("""{"op":"gte","args":[2,2]}"""))
        assertEquals(t, eval("""{"op":"lt","args":[1,2]}"""))
        assertEquals(f, eval("""{"op":"lte","args":[3,2]}"""))
        assertEquals(t, eval("""{"op":"and","args":[true,{"op":"gt","args":[2,1]}]}"""))
        assertEquals(f, eval("""{"op":"or","args":[false,false]}"""))
        assertEquals(t, eval("""{"op":"not","args":[false]}"""))
        assertEquals(ExprValue.Str("大"), eval("""{"if":{"op":"gt","args":[5,3]},"then":"大","else":"小"}"""))
        assertEquals("null 条件は false", ExprValue.Str("y"), eval("""{"if":{"op":"div","args":[1,0]},"then":"x","else":"y"}"""))
    }

    @Test
    fun stringsAndConversion() {
        assertEquals(ExprValue.Str("A→12"), eval("""{"op":"concat","args":["A","→",12]}"""))
        assertEquals(3.0, num("""{"op":"len","args":["割り勘"]}"""))
        assertEquals(42.0, num("""{"op":"toNumber","args":["42"]}"""))
        assertEquals(ExprValue.Str("42"), eval("""{"op":"toString","args":[42]}"""))
        assertEquals(7.0, num("""{"op":"coalesce","args":[{"op":"div","args":[1,0]},7]}"""))
    }

    // MARK: - 日付

    @Test
    fun dates() {
        val now = FieldCoding.strictDay("2026-10-24")!!
        fun d(json: String) = eval(json, now = now)
        assertEquals(23.0, d("""{"op":"daysBetween","args":["2026-10-01","2026-10-24"]}""")?.number)
        assertEquals(61.0, d("""{"op":"daysUntil","args":["2026-12-24"]}""")?.number)
        assertEquals(ExprValue.Str("2026-10-31"), d("""{"op":"toString","args":[{"op":"addDays","args":[{"op":"today"},7]}]}"""))
        assertEquals(2026.0, d("""{"op":"year","args":["2026-10-24"]}""")?.number)
        assertEquals(10.0, d("""{"op":"month","args":["2026-10-24"]}""")?.number)
        assertEquals(24.0, d("""{"op":"day","args":["2026-10-24"]}""")?.number)
        assertEquals("Calendar 準拠: 日=1 … 土=7", 7.0, d("""{"op":"weekday","args":["2026-10-24"]}""")?.number)
        assertEquals("date と日付の文字列は日付として比べる(#552)", ExprValue.Bool(true), d("""{"op":"gt","args":[{"op":"now"},"2026-01-01"]}"""))
        assertNull("日付として読めない文字列とは比べない", d("""{"op":"gt","args":[{"op":"now"},"tomorrow"]}"""))
        assertEquals(ExprValue.Bool(true), d("""{"op":"gt","args":[{"op":"now"},{"op":"addDays","args":["2026-01-01",0]}]}"""))
    }

    @Test
    fun deepNesting() {
        var json = "1"
        repeat(30) { json = """{"op":"add","args":[$json,1]}""" }
        assertEquals(31.0, num(json))
        var tooDeep = "1"
        repeat(ExprEvaluator.MAX_DEPTH + 5) { tooDeep = """{"op":"neg","args":[$tooDeep]}""" }
        assertNull("上限を超えた入れ子は null(スタックを守る)", num(tooDeep))
    }

    // MARK: - agg + 行コンテキスト(割り勘)

    private val warikanRecords: Map<String, List<Record>> = mapOf(
        "members" to listOf(rec("m1", "name" to "たろう"), rec("m2", "name" to "はなこ"), rec("m3", "name" to "じろう")),
        "expenses" to listOf(
            rec("e1", "title" to "宿", "amount" to 9000, "payer" to "m1"),
            rec("e2", "title" to "夕食", "amount" to 4500, "payer" to "m2"),
            rec("e3", "title" to "ガソリン", "amount" to 1500, "payer" to "m1"),
        ),
    )

    private fun warikan() = Fixtures.load("warikan")

    @Test
    fun warikanComputedFields() {
        val dsl = warikan()
        val ev = ExprEvaluator(dsl.entities, { warikanRecords[it] ?: emptyList() })
        val members = ev.viewRecords("members")
        val byName = members.associateBy { it["name"].stringValue!! }
        // 合計 15000 / 3 人 = 5000
        assertEquals(10500.0, byName["たろう"]?.get("paid")?.doubleValue)
        assertEquals(4500.0, byName["はなこ"]?.get("paid")?.doubleValue)
        assertEquals(0.0, byName["じろう"]?.get("paid")?.doubleValue)
        assertEquals(5000.0, byName["たろう"]?.get("share")?.doubleValue)
        assertEquals(5500.0, byName["たろう"]?.get("balance")?.doubleValue)
        assertEquals(-500.0, byName["はなこ"]?.get("balance")?.doubleValue)
        assertEquals(-5000.0, byName["じろう"]?.get("balance")?.doubleValue)
        assertNull("computed は保存レコードに入れない", warikanRecords.getValue("members")[0].data["balance"])

        // agg の field に computed を指定: balance の合計は 0
        assertEquals(0.0, ev.evaluate(Expr.Agg(AggExpr("sum", "members", "balance")))?.number)

        val t = Settlement.transfers(members.map { it["name"].stringValue!! to it["balance"].doubleValue!! })
        assertEquals(listOf(Settlement.Transfer("じろう", "たろう", 5000.0), Settlement.Transfer("はなこ", "たろう", 500.0)), t)
    }

    /** 正典 warikan.json のホーム: stat 合計 ¥15,000 / 1人あたり ¥5,000 / 支出の per_person。 */
    @Test
    fun warikanHomeStatsAndPerPerson() {
        val dsl = warikan()
        val ev = ExprEvaluator(dsl.entities, { warikanRecords[it] ?: emptyList() })
        val stats = dsl.components("home").drop(1).take(2)
        assertEquals(listOf("合計", "1人あたり"), stats.map { it.str("label") })
        val total = Aggregator.aggregate(stats[0].str("aggregate"), stats[0].str("field"), ev.viewRecords("expenses"))
        assertEquals(15000.0, total, 0.0)
        assertEquals("¥15,000", Aggregator.format(total, stats[0].str("format"), locale = java.util.Locale.JAPAN))
        assertNull(stats[1].str("aggregate"))
        val perPerson = ev.evaluate(Expr.from(stats[1]["value"]!!))!!.number!!
        assertEquals(5000.0, perPerson, 0.0)
        assertEquals("¥5,000", Aggregator.format(perPerson, stats[1].str("format"), locale = java.util.Locale.JAPAN))
        val expenses = ev.viewRecords("expenses").associateBy { it.id }
        assertEquals(3000.0, expenses["e1"]?.get("per_person")?.doubleValue)
        assertEquals(1500.0, expenses["e2"]?.get("per_person")?.doubleValue)
    }

    /** 正典 bmi.json: 体重 65kg / 身長 170cm → 22.5(小数 1 桁)。 */
    @Test
    fun bmiExample() {
        val dsl = Fixtures.load("bmi")
        val records = mapOf(
            "body" to listOf(
                rec("b1", "date" to "2026-10-24", "weight" to 65, "height" to 170),
                rec("b2", "date" to "2026-10-23", "weight" to 70, "height" to 175),
            ),
        )
        val ev = ExprEvaluator(dsl.entities, { records[it] ?: emptyList() })
        val rows = ev.viewRecords("body").associateBy { it.id }
        assertEquals(22.5, rows["b1"]?.get("bmi")?.doubleValue)
        assertEquals(22.9, rows["b2"]?.get("bmi")?.doubleValue)
        val avgStat = dsl.components("home").first()
        assertEquals(22.7, Aggregator.aggregate(avgStat.str("aggregate"), avgStat.str("field"), ev.viewRecords("body")), 1e-9)
    }

    @Test
    fun aggFilterArrayAndEmpty() {
        val ev = ExprEvaluator(warikan().entities, { warikanRecords[it] ?: emptyList() })
        val e = expr("""{"agg":"sum","entity":"expenses","field":"amount","filter":[{"field":"payer","op":"eq","value":"m1"},{"field":"amount","op":"gt","value":2000}]}""")
        assertEquals(9000.0, ev.evaluate(e)?.number)
        assertNull("空の avg は null", ev.evaluate(expr("""{"agg":"avg","entity":"nothing","field":"x"}""")))
        assertEquals(0.0, ev.evaluate(expr("""{"agg":"count","entity":"nothing"}"""))?.number)
    }

    @Test
    fun zeroMembersShareIsNull() {
        val dsl = warikan()
        val ev = ExprEvaluator(dsl.entities, { emptyList() })
        // #231: 1人あたり = 金額 / 割り勘する人数(未選択 = 全員 = 0 人)
        val row = ExprEvaluator.Row(dsl.entity("expenses"), rec("x", "amount" to 1000), memoizable = false)
        assertNull("0 人で割る = —", ev.fieldValue("per_person", row))
        assertEquals("—", FieldCoding.display(JSONValue.Null, dsl.entity("expenses")?.field("per_person")))
    }

    @Test
    fun cycleIsNull() {
        val entity = Entity.parse(
            """{"id":"e","name":"E","fields":[{"id":"a","name":"a","type":"number","computed":{"ref":"b"}},{"id":"b","name":"b","type":"number","computed":{"op":"add","args":[{"ref":"a"},1]}}]}""",
        )
        val ev = ExprEvaluator(listOf(entity), { listOf(Record("1")) })
        assertNull(ev.viewRecords("e")[0].data["a"])
    }

    // MARK: - JSON / 後方互換

    @Test
    fun filterBackwardCompatAndExprValue() {
        val legacy = Filter.parse("""{"field":"category","op":"eq","value":"食費"}""")
        assertEquals(JSONValue.Str("食費"), legacy.value)
        assertNull(legacy.valueExpr)
        val modern = Filter.parse("""{"field":"payer","op":"eq","value":{"ref":"id"}}""")
        assertNull(modern.value)
        assertEquals(Expr.Ref("id"), modern.valueExpr)
        assertEquals(modern, Filter.from(modern.toJsonElement()))
    }

    @Test
    fun exprRoundTrip() {
        val e = expr(
            """{"if":{"op":"gt","args":[{"ref":"balance"},0]},"then":{"agg":"sum","entity":"expenses","field":"amount","filter":{"field":"payer","op":"eq","value":{"ref":"id"}}},"else":null}""",
        )
        assertEquals(e, Expr.from(e.toJsonElement()))
        assertEquals(setOf("expenses"), e.referencedEntities)
        assertThrows(Exception::class.java) { expr("""{"foo":1}""") }
    }

    @Test
    fun settlementEdgeCases() {
        assertEquals(emptyList<Settlement.Transfer>(), Settlement.transfers(listOf("A" to 0.0, "B" to 0.0)))
        assertEquals("1 円未満は精算不要", emptyList<Settlement.Transfer>(), Settlement.transfers(listOf("A" to 0.3, "B" to -0.3)))
        // 1000 円を 3 人: 端数があっても転送は円に丸める
        val t = Settlement.transfers(listOf("A" to 666.67, "B" to -333.33, "C" to -333.34))
        assertEquals(666.0, t.sumOf { it.amount }, 0.0)
        assertEquals(setOf("A"), t.map { it.to }.toSet())
    }

    @Test
    fun supportedOpsAndFilterOpsMatchSchema() {
        val defs = Fixtures.schemaDefs()
        fun enumOf(def: String) = ((defs[def] as JsonObject)["properties"] as JsonObject)["op"]!!.let { it as JsonObject }["enum"]!!
            .let { it as JsonArray }.map { (it as JsonPrimitive).content }.toSet()
        assertEquals(enumOf("ExprOp"), ExprEvaluator.supportedOps)
        assertEquals(enumOf("Filter"), Aggregator.filterOps)
    }
}
