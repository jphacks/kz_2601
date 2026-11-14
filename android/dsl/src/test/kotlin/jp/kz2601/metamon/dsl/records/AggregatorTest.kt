package jp.kz2601.metamon.dsl.records

import jp.kz2601.metamon.dsl.records.Fixtures.rec
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Test
import java.time.Instant
import java.util.Locale

/** iOS の AggregatorTests を写したもの。 */
class AggregatorTest {
    private val savedZone = FieldCoding.zone

    init {
        FieldCoding.zone = Fixtures.TOKYO
    }

    @After
    fun restore() {
        FieldCoding.zone = savedZone
    }

    private val records = listOf(
        rec("1", "amount" to 100, "category" to "食費", "date" to FieldCoding.formatDay(Instant.now())),
        rec("2", "amount" to 50, "category" to "交通", "date" to "2000-01-01"),
    )

    /** #235: ref の groupBy は id で束ね、label で参照先の見出しに置き換える(未指定なら id のまま)。 */
    @Test
    fun bucketsRelabelRefGroupBy() {
        val rows = listOf(rec("1", "tag" to "t1"), rec("2", "tag" to "t1"), rec("3", "tag" to "t2"))
        val titles = mapOf("t1" to "小説", "t2" to "技術")
        val b = Aggregator.buckets("tag", null, "count", null, null, rows, label = { titles[it] ?: it })
        assertEquals(listOf("t1", "t2"), b.map { it.key })
        assertEquals(listOf("小説", "技術"), b.map { it.label })
        assertEquals(listOf(2.0, 1.0), b.map { it.value })
        assertEquals(listOf("t1", "t2"), Aggregator.buckets("tag", null, "count", null, null, rows).map { it.label })
    }

    /** chart の日付の見出し: 日本語(既定)は「3月」「3/4」、英語は「Mar」「3/4」。 */
    @Test
    fun dateBucketLabelsFollowLocale() {
        val entity = Entity.parse("""{"id":"e","name":"e","fields":[{"id":"d","name":"d","type":"date"}]}""")
        val rows = listOf(rec("1", "d" to "2026-03-04"), rec("2", "d" to "2026-04-10"))
        fun labels(bucket: String, locale: Locale?) = Aggregator.buckets("d", bucket, "count", null, entity, rows, locale = locale).map { it.label }
        assertEquals(listOf("3月", "4月"), labels("month", null))
        assertEquals(listOf("3月", "4月"), labels("month", Locale.JAPAN))
        assertEquals(listOf("Mar", "Apr"), labels("month", Locale.US))
        assertEquals(listOf("3/4", "4/10"), labels("day", Locale.US))
        assertEquals(listOf("3/4", "4/10"), labels("day", null))
        assertFalse(labels("week", Locale.US).any { it.contains("週") })
    }

    @Test
    fun aggregate() {
        assertEquals(150.0, Aggregator.aggregate("sum", "amount", records), 0.0)
        assertEquals(75.0, Aggregator.aggregate("avg", "amount", records), 0.0)
        assertEquals(100.0, Aggregator.aggregate("max", "amount", records), 0.0)
        assertEquals(2.0, Aggregator.aggregate("count", null, records), 0.0)
    }

    @Test
    fun filter() {
        assertEquals(1, Aggregator.apply(Filter("date", "thisMonth"), null, records).size)
        assertEquals(listOf("1"), Aggregator.apply(Filter("amount", "gte", JSONValue.number(100)), null, records).map { it.id })
        assertEquals(listOf("2"), Aggregator.apply(Filter("category", "eq", JSONValue.Str("交通")), null, records).map { it.id })
    }

    @Test
    fun sortAndFormat() {
        // entity の field 型が数値なら数値比較、不明なら文字列比較
        val entity = Entity.parse("""{"id":"e","name":"E","fields":[{"id":"amount","name":"a","type":"integer"}]}""")
        assertEquals(listOf("2", "1"), Aggregator.sorted(records, Sort("amount", "asc"), entity).map { it.id })
        assertEquals(listOf("1", "2"), Aggregator.sorted(records, Sort("amount", "asc"), null).map { it.id })
        assertEquals("¥1,234", Aggregator.format(1234.0, "currency", locale = Locale.JAPAN))
        assertEquals("-¥500", FieldCoding.currency(-500.0, Locale.JAPAN))
        assertEquals("¥0", FieldCoding.currency(-0.2, Locale.JAPAN))
        assertEquals("12.5 kg", Aggregator.format(12.5, null, "kg"))
        assertEquals("40%", Aggregator.format(0.4, "percent"))
    }

    /** percent は比率(0〜1)を 100 倍で出す。 */
    @Test
    fun percentIsRatio() {
        assertEquals("50%", Aggregator.format(0.5, "percent"))
        assertEquals("142%", Aggregator.format(1.42, "percent"))
        assertEquals("0%", Aggregator.format(0.0, "percent"))
        val rs = listOf(0.0, 1.0, 0.0, 0.0).mapIndexed { i, v -> rec("$i", "done" to v) }
        assertEquals("25%", Aggregator.format(Aggregator.aggregate("avg", "done", rs), "percent"))
    }

    @Test
    fun fieldCodingNormalize() {
        val entity = Entity.parse(
            """{"id":"e","name":"E","fields":[{"id":"n","name":"n","type":"integer"},{"id":"b","name":"b","type":"boolean"},{"id":"s","name":"s","type":"string"}]}""",
        )
        assertEquals(JSONValue.number(1234), FieldCoding.normalize(JSONValue.Str("1,234"), entity.field("n")!!))
        assertNull(FieldCoding.normalize(JSONValue.Str(""), entity.field("s")!!))
        assertEquals(JSONValue.Bool(false), FieldCoding.normalize(JSONValue.Null, entity.field("b")!!))
        assertEquals("980", FieldCoding.display(JSONValue.number(980), null))
    }

    @Test
    fun thisWeekIsMondayToSunday() {
        // 2026-10-28 は水曜。今週 = 10/26(月) 0:00 〜 11/2(月) 0:00
        val now = Fixtures.at(2026, 10, 28)
        val rows = listOf(rec("sun", "d" to "2026-10-25"), rec("mon", "d" to "2026-10-26"), rec("sun2", "d" to "2026-11-01"), rec("mon2", "d" to "2026-11-02"))
        assertEquals(listOf("mon", "sun2"), rows.filter { Aggregator.matches(Filter("d", "thisWeek"), it, null, now) }.map { it.id })
    }
}
