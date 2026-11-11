package jp.kz2601.metamon.dsl.records

import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.DslJson
import jp.kz2601.metamon.dsl.records.Fixtures.rec
import kotlinx.serialization.json.JsonPrimitive
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import java.time.Instant

private fun component(json: String): Component = DslJson.decodeFromString(Component.serializer(), json)

/** #261: table.groupBy + 集計列(iOS の TableGroupingTests)。 */
class TableGroupingTest {
    private val reviews = listOf(
        rec("1", "shop" to "麺屋こがね", "score" to 5),
        rec("2", "shop" to "らーめん大将", "score" to 3),
        rec("3", "shop" to "麺屋こがね", "score" to 4),
        rec("4", "shop" to "中華そば 一番", "score" to 5),
        rec("5", "shop" to "麺屋こがね ", "score" to 4), // 末尾の空白は同じ店
        rec("6", "shop" to "らーめん大将", "score" to 4),
        rec("7", "score" to 1), // 店が空 = どの行にも入れない
    )
    private val columns = """[{"field":"shop"},{"field":"score","aggregate":"avg"},{"field":"shop","aggregate":"count"}]"""

    @Test
    fun sameShopFoldsIntoOneRowSortedByAverage() {
        val c = component("""{"type":"table","entity":"review","groupBy":"shop","columns":$columns,"sort":{"field":"score","direction":"desc"}}""")
        assertTrue(TableGrouping.isGrouped(c))
        val rows = TableGrouping.rows(c, reviews)
        assertEquals(listOf("中華そば 一番", "麺屋こがね", "らーめん大将"), rows.map { it.key })
        val kogane = rows.first { it.key == "麺屋こがね" }
        assertNull(kogane.values[0])
        assertEquals(13.0 / 3, kogane.values[1]!!, 1e-9)
        assertEquals(3.0, kogane.values[2])
    }

    @Test
    fun sortByCountAndDefault() {
        val byCount = component("""{"type":"table","entity":"review","groupBy":"shop","columns":$columns,"sort":{"field":"shop","direction":"desc"}}""")
        assertEquals(listOf("麺屋こがね", "らーめん大将", "中華そば 一番"), TableGrouping.rows(byCount, reviews).map { it.key })
        val byDefault = component("""{"type":"table","entity":"review","groupBy":"shop","columns":$columns,"limit":2}""")
        assertEquals(listOf("中華そば 一番", "麺屋こがね"), TableGrouping.rows(byDefault, reviews).map { it.key })
        val byName = component("""{"type":"table","entity":"review","groupBy":"shop","columns":[{"field":"shop"},{"field":"score","aggregate":"sum"}],"sort":{"field":"shop","direction":"asc"}}""")
        val abc = listOf(rec("1", "shop" to "B", "score" to 1), rec("2", "shop" to "C", "score" to 9), rec("3", "shop" to "A", "score" to 5))
        assertEquals(listOf("A", "B", "C"), TableGrouping.rows(byName, abc).map { it.key })
    }

    @Test
    fun emptyNumbers() {
        val c = component("""{"type":"table","entity":"review","groupBy":"shop","columns":[{"field":"shop"},{"field":"score","aggregate":"avg"},{"field":"score","aggregate":"sum"},{"field":"shop","aggregate":"count"}]}""")
        val rows = TableGrouping.rows(c, listOf(rec("1", "shop" to "A")))
        assertEquals(1, rows.size)
        assertNull(rows[0].values[1])
        assertEquals(0.0, rows[0].values[2])
        assertEquals(1.0, rows[0].values[3])
    }

    @Test
    fun withoutKnownAggregateIsNotGrouped() {
        assertFalse(TableGrouping.isGrouped(component("""{"type":"table","entity":"review","groupBy":"shop","columns":[{"field":"shop"}]}""")))
        assertFalse(TableGrouping.isGrouped(component("""{"type":"table","entity":"review","groupBy":"shop","columns":[{"field":"shop"},{"field":"score","aggregate":"median"}]}""")))
    }

    @Test
    fun namesCompareNaturally() {
        assertTrue(TableGrouping.nameCompare("2", "10") < 0)
        assertTrue(TableGrouping.nameCompare("店2", "店10") < 0)
        assertTrue(TableGrouping.nameCompare("a", "B") < 0)
    }
}

/** `gridTable`(#234): 見出し・配置・空マスの初期値・今日の列(iOS の GridTableLayoutTests)。 */
class GridTableLayoutTest {
    private val example = Fixtures.load("timetable")
    private val lecture = example.entity("lecture")!!
    private val grid = component(example.allComponents.first { (it["type"] as? JsonPrimitive)?.content == "gridTable" }.toString())

    @Test
    fun headersFromEnumOptionsAndRange() {
        val layout = GridTableLayout(grid, lecture, emptyList())
        assertEquals(listOf("月", "火", "水", "木", "金"), layout.columns.map { it.label })
        assertEquals(listOf("1限", "2限", "3限", "4限", "5限"), layout.rows.map { it.label })
        assertEquals(JSONValue.Num(1.0), layout.rows.first().value)
    }

    @Test
    fun placesRecordsOnCells() {
        val records = listOf(
            rec("a", "subject" to "線形代数", "weekday" to "月", "period" to 1),
            rec("b", "subject" to "英語", "weekday" to "水", "period" to 2),
            rec("c", "subject" to "英語演習", "weekday" to "水", "period" to "2"), // 文字列の数字も同じマス
            rec("d", "subject" to "土曜講座", "weekday" to "土", "period" to 1), // 列に無い曜日は載らない
            rec("e", "subject" to "7限", "weekday" to "月", "period" to 7), // rowRange 外も載らない
        )
        val layout = GridTableLayout(grid, lecture, records)
        assertEquals(listOf("a"), layout.records(GridTableLayout.Cell(0, 0)).map { it.id })
        assertEquals(listOf("b", "c"), layout.records(GridTableLayout.Cell(2, 1)).map { it.id })
        assertTrue(layout.records(GridTableLayout.Cell(1, 0)).isEmpty())
        assertEquals(3, layout.cells.values.sumOf { it.size })
    }

    @Test
    fun emptyCellPrefillCarriesColumnAndRowValues() {
        val layout = GridTableLayout(grid, lecture, emptyList())
        assertEquals(mapOf("weekday" to JSONValue.Str("木"), "period" to JSONValue.Num(3.0)), layout.prefill(GridTableLayout.Cell(3, 2)))
    }

    private val x = Entity.parse("""{"id":"x","name":"x","fields":[{"id":"t","name":"t","type":"string"},{"id":"day","name":"d","type":"enum","options":["月","火"]},{"id":"p","name":"p","type":"integer"}]}""")
    private val plain = component("""{"type":"gridTable","entity":"x","columnField":"day","rowField":"p","titleField":"t"}""")

    @Test
    fun defaultRowsWithoutRangeExpandToData() {
        assertEquals(listOf("1", "2", "3", "4", "5", "6"), GridTableLayout(plain, x, emptyList()).rows.map { it.label })
        assertEquals(8, GridTableLayout(plain, x, listOf(rec("1", "t" to "x", "day" to "月", "p" to 8))).rows.size)
    }

    @Test
    fun enumRowsAndExplicitColumns() {
        val c = component("""{"type":"gridTable","entity":"s","columnField":"day","rowField":"slot","titleField":"who","columnValues":["土","日","土"]}""")
        val e = Entity.parse("""{"id":"s","name":"s","fields":[{"id":"who","name":"w","type":"string"},{"id":"day","name":"d","type":"enum","options":["月","火","水","木","金","土","日"]},{"id":"slot","name":"s","type":"enum","options":["早番","遅番"]}]}""")
        val layout = GridTableLayout(c, e, listOf(rec("1", "who" to "佐藤", "day" to "日", "slot" to "遅番")))
        assertEquals(listOf("土", "日"), layout.columns.map { it.label })
        assertEquals(listOf("早番", "遅番"), layout.rows.map { it.label })
        assertEquals(listOf("1"), layout.records(GridTableLayout.Cell(1, 1)).map { it.id })
        assertEquals(mapOf("day" to JSONValue.Str("土"), "slot" to JSONValue.Str("遅番")), layout.prefill(GridTableLayout.Cell(0, 1)))
    }

    @Test
    fun todayColumn() {
        val wed = Instant.parse("2026-10-28T03:00:00Z") // 水曜(JST 12:00)
        val tokyo = Fixtures.TOKYO
        assertEquals(2, GridTableLayout.todayColumn(listOf("月", "火", "水", "木", "金"), wed, tokyo))
        assertEquals(2, GridTableLayout.todayColumn(listOf("月曜日", "火曜日", "水曜日"), wed, tokyo))
        assertEquals(2, GridTableLayout.todayColumn(listOf("Mon", "Tue", "Wed", "Thu"), wed, tokyo))
        assertNull(GridTableLayout.todayColumn(listOf("月", "火", "木"), wed, tokyo))
        assertNull(GridTableLayout.todayColumn(listOf("早番", "遅番", "水"), wed, tokyo))
    }

    @Test
    fun hugeValuesDoNotCrash() {
        val records = listOf(
            rec("big", "t" to "x", "day" to "月", "p" to 1e20),
            rec("neg", "t" to "x", "day" to "月", "p" to -1e300),
            rec("max", "t" to "x", "day" to "月", "p" to "9223372036854775807"),
            rec("ok", "t" to "x", "day" to "月", "p" to 2),
        )
        val layout = GridTableLayout(plain, x, records)
        assertEquals(listOf("1", "2", "3", "4", "5", "6"), layout.rows.map { it.label })
        assertEquals(listOf("ok"), layout.records(GridTableLayout.Cell(0, 1)).map { it.id })
    }
}

/** stat の distance / duration(#168・iOS の Track)。 */
class TrackTest {
    @Test
    fun distanceAndDurationSplitBySessionGap() {
        val records = listOf(
            rec("1", "loc" to "35.681236,139.767125", "at" to "2026-11-12T09:00:00+09:00"),
            rec("2", "loc" to "35.682100,139.766300", "at" to "2026-11-12T09:01:00+09:00"),
            // 1 時間後 = 別の記録(繋がない)
            rec("3", "loc" to "35.690000,139.700000", "at" to "2026-11-12T10:01:00+09:00"),
            rec("4", "loc" to "35.690000,139.700000", "at" to "2026-11-12T10:02:30+09:00"),
            rec("5", "loc" to "999,1", "at" to "2026-11-12T10:03:00+09:00"), // 範囲外は捨てる
        )
        val d = Track.aggregate("distance", records, "loc", "at")
        assertEquals(120.0, d, 10.0)
        assertEquals("1:00", Track.formatDuration(60.0))
        assertEquals(150.0, Track.aggregate("duration", records, "loc", "at"), 1e-9)
        assertEquals("2:30", Track.format("duration", 150.0))
        assertEquals("0 m", Track.format("distance", 0.0))
        assertEquals("1.23 km", Track.formatDistance(1234.0))
        assertEquals("1:05:00", Track.formatDuration(3900.0))
    }
}
