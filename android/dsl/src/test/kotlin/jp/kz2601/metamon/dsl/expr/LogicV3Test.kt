package jp.kz2601.metamon.dsl.expr

import jp.kz2601.metamon.dsl.records.Aggregator
import jp.kz2601.metamon.dsl.records.Entity
import jp.kz2601.metamon.dsl.records.FieldCoding
import jp.kz2601.metamon.dsl.records.Fixtures
import jp.kz2601.metamon.dsl.records.Fixtures.at
import jp.kz2601.metamon.dsl.records.Fixtures.rec
import jp.kz2601.metamon.dsl.records.Fixtures.str
import jp.kz2601.metamon.dsl.records.JSONValue
import jp.kz2601.metamon.dsl.records.Record
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import java.time.ZonedDateTime

/** iOS の LogicV3Tests を写したもの: #133 agg latest/earliest / #134 lookup / #132 毎日リセット。 */
class LogicV3Test {
    private val savedZone = FieldCoding.zone

    init {
        FieldCoding.zone = Fixtures.TOKYO
    }

    @After
    fun restore() {
        FieldCoding.zone = savedZone
    }

    private fun expr(json: String) = Expr.parse(json)

    private val body = Entity.parse(
        """{"id":"body","name":"体重","fields":[{"id":"date","name":"日付","type":"date"},{"id":"weight","name":"体重","type":"number","unit":"kg"},
          {"id":"note","name":"メモ","type":"string"}]}""",
    )

    // MARK: - latest / earliest

    @Test
    fun latestPicksNewestByDate() {
        val rows = listOf(
            rec("a", "date" to "2026-10-20", "weight" to 70, createdAt = "2026-10-20T08:00:00Z"),
            rec("b", "date" to "2026-10-23", "weight" to 68.5, createdAt = "2026-10-21T08:00:00Z"), // 後から過去日を入れても日付で並ぶ
            rec("c", "date" to "2026-10-22", "weight" to 69, createdAt = "2026-10-24T08:00:00Z"),
        )
        val ev = ExprEvaluator(listOf(body), { rows })
        assertEquals(ExprValue.Number(68.5), ev.evaluate(expr("""{"agg":"latest","entity":"body","field":"weight","by":"date"}""")))
        assertEquals(ExprValue.Number(70.0), ev.evaluate(expr("""{"agg":"earliest","entity":"body","field":"weight","by":"date"}""")))
        // by 省略 = 最初の日付 field
        assertEquals(ExprValue.Number(68.5), ev.evaluate(expr("""{"agg":"latest","entity":"body","field":"weight"}""")))
        // 増減 = latest − earliest
        assertEquals(
            ExprValue.Number(-1.5),
            ev.evaluate(expr("""{"op":"sub","args":[{"agg":"latest","entity":"body","field":"weight"},{"agg":"earliest","entity":"body","field":"weight"}]}""")),
        )
        // 日付 field の latest は日付のまま
        assertEquals("2026-10-23", ev.evaluate(expr("""{"agg":"latest","entity":"body","field":"date"}"""))?.text)
    }

    @Test
    fun latestSameDayUsesCreationOrder() {
        val rows = listOf(
            rec("late", "date" to "2026-10-24", "weight" to 66, createdAt = "2026-10-24T20:00:00Z"),
            rec("early", "date" to "2026-10-24", "weight" to 67, createdAt = "2026-10-24T07:00:00Z"),
        )
        assertEquals("late", Aggregator.pick("latest", "date", body, rows)?.id)
        assertEquals("early", Aggregator.pick("earliest", "date", body, rows)?.id)
        // createdAt も同じなら配列の後ろを新しいとみなす
        val tie = listOf(rec("1", "date" to "2026-10-24"), rec("2", "date" to "2026-10-24"))
        assertEquals("2", Aggregator.pick("latest", "date", body, tie)?.id)
    }

    @Test
    fun latestEmptyAndMissing() {
        val ev = ExprEvaluator(listOf(body), { emptyList() })
        assertNull("0 件は空(「—」)", ev.evaluate(expr("""{"agg":"latest","entity":"body","field":"weight"}""")))
        // 日付が空のレコードは並べられないので除く・値が空なら null
        val rows = listOf(rec("x", "weight" to 50), rec("y", "date" to "2026-10-01"))
        val ev2 = ExprEvaluator(listOf(body), { rows })
        assertNull(ev2.evaluate(expr("""{"agg":"latest","entity":"body","field":"weight"}""")))
        assertNull(Aggregator.pickText("latest", "weight", null, null, body, emptyList()))
    }

    @Test
    fun statPickText() {
        val rows = listOf(rec("a", "date" to "2026-10-20", "weight" to 70), rec("b", "date" to "2026-10-23", "weight" to 68.5))
        assertEquals("68.5 kg", Aggregator.pickText("latest", "weight", "date", null, body, rows))
        assertNull("値が空", Aggregator.pickText("latest", "note", "date", null, body, rows))
    }

    @Test
    fun aggByRoundTrips() {
        val e = expr("""{"agg":"latest","entity":"body","field":"weight","by":"date"}""")
        assertEquals(e, Expr.from(e.toJsonElement()))
        assertEquals("date", (e as Expr.Agg).agg.by)
    }

    // MARK: - lookup

    @Test
    fun lookupReadsReferencedRecord() {
        val chores = Entity.parse(
            """{"id":"chores","name":"家事","fields":[{"id":"name","name":"名前","type":"string"},{"id":"points","name":"点","type":"integer"},{"id":"double","name":"2倍","type":"integer","computed":{"op":"mul","args":[{"ref":"points"},2]}}]}""",
        )
        val log = Entity.parse(
            """{"id":"log","name":"記録","fields":[{"id":"chore","name":"家事","type":"ref","refEntity":"chores"},{"id":"pts","name":"点","type":"integer","computed":{"ref":"chore.points"}}]}""",
        )
        val data = mapOf(
            "chores" to listOf(rec("c1", "name" to "皿洗い", "points" to 3)),
            "log" to listOf(rec("l1", "chore" to "c1"), rec("l2", "chore" to "gone"), Record("l3")),
        )
        val ev = ExprEvaluator(listOf(chores, log), { data[it] ?: emptyList() })
        fun row(id: String) = ExprEvaluator.Row(log, data.getValue("log").first { it.id == id })
        assertEquals(ExprValue.Number(3.0), ev.evaluate(Expr.Ref("chore.points"), row("l1")))
        assertEquals(ExprValue.Str("皿洗い"), ev.evaluate(Expr.Ref("chore.name"), row("l1")))
        assertEquals("参照先の computed も読める", ExprValue.Number(6.0), ev.evaluate(Expr.Ref("chore.double"), row("l1")))
        assertEquals(ExprValue.Str("c1"), ev.evaluate(Expr.Ref("chore.id"), row("l1")))
        assertNull("参照先が消えている = 空", ev.evaluate(Expr.Ref("chore.points"), row("l2")))
        assertNull("未選択 = 空", ev.evaluate(Expr.Ref("chore.points"), row("l3")))
        assertNull("無い field", ev.evaluate(Expr.Ref("chore.ghost"), row("l1")))
        assertNull("2 段は読まない", ev.evaluate(Expr.Ref("chore.points.x"), row("l1")))
        assertNull("ref でない field", ev.evaluate(Expr.Ref("pts.points"), row("l1")))
        // 記録の computed として合成 → 合計
        assertEquals(JSONValue.Num(3.0), ev.viewRecords("log").first()["pts"])
        assertEquals(ExprValue.Number(3.0), ev.evaluate(expr("""{"agg":"sum","entity":"log","field":"pts"}""")))
    }

    // MARK: - daily toggle

    @Test
    fun dailyCheckDateBoundary() {
        val today = at(2026, 10, 24, 23, 59)
        assertTrue(DailyCheck.isDone(JSONValue.Str("2026-10-24"), today))
        assertFalse("昨日チェックした = 今日は未チェック", DailyCheck.isDone(JSONValue.Str("2026-10-23"), today))
        assertFalse("0:00 を過ぎたら戻る", DailyCheck.isDone(JSONValue.Str("2026-10-24"), at(2026, 10, 25, 0, 0)))
        assertFalse(DailyCheck.isDone(JSONValue.Null, today))
        assertFalse(DailyCheck.isDone(JSONValue.Str("x"), today))
        assertTrue("RFC3339 も読む", DailyCheck.isDone(JSONValue.Str(FieldCoding.formatDatetime(at(2026, 10, 24, 8))), today))
        assertEquals(JSONValue.Str("2026-10-24"), DailyCheck.stored(true, today))
        assertEquals(JSONValue.Null, DailyCheck.stored(false, today))
    }

    @Test
    fun debugTodayOverride() {
        val real = at(2026, 10, 24, 21, 30)
        val d = DSLClock.debugNow(real, mapOf("METAMON_DEBUG_TODAY" to "2026-10-25"))
        assertEquals("2026-10-25", d?.let { FieldCoding.formatDay(it) })
        assertEquals("時刻は実時刻のまま", 21, d?.let { ZonedDateTime.ofInstant(it, Fixtures.TOKYO).hour })
        assertNull(DSLClock.debugNow(real, emptyMap()))
        assertNull(DSLClock.debugNow(real, mapOf("METAMON_DEBUG_TODAY" to "tomorrow")))
    }

    /** iOS の testMedicineExampleClearsToggleDefault のうち、式の部分(今日飲んだ数 = count filter today)。 */
    @Test
    fun medicineTodayCount() {
        val dsl = Fixtures.load("medicine")
        val rows = listOf(rec("1", "taken_on" to "2026-10-24"), rec("2", "taken_on" to "2026-10-23"), Record("3"))
        val ev = ExprEvaluator(dsl.entities, { rows }, at(2026, 10, 24))
        val stat = dsl.allComponents.first { it.str("label") == "今日飲んだ(回)" }
        assertEquals(ExprValue.Number(1.0), ev.evaluate(Expr.from(stat["value"]!!)))
    }

    @Test
    fun streakAndDoneInDays() {
        val rows = listOf(rec("r", "done" to "2026-10-24", "done__dates" to listOf("2026-10-21", "2026-10-22", "2026-10-23")))
        val e = Entity.parse("""{"id":"h","name":"習慣","fields":[{"id":"done","name":"済","type":"date"}]}""")
        val ev = ExprEvaluator(listOf(e), { rows }, at(2026, 10, 24))
        val row = ExprEvaluator.Row(e, rows[0])
        assertEquals(ExprValue.Number(4.0), ev.evaluate(expr("""{"op":"streak","args":[{"ref":"done"}]}"""), row))
        assertEquals(ExprValue.Number(2.0), ev.evaluate(expr("""{"op":"doneInDays","args":[{"ref":"done"},2]}"""), row))
        val patch = DailyCheck.patch(rows[0], "done", false, at(2026, 10, 24))
        assertEquals(JSONValue.Str("2026-10-23"), patch["done"])
    }
}
