package jp.kz2601.metamon.renderer

import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.ComponentType
import jp.kz2601.metamon.dsl.DslJson
import jp.kz2601.metamon.dsl.expr.DSLClock
import jp.kz2601.metamon.dsl.records.Aggregator
import jp.kz2601.metamon.dsl.records.Entity
import jp.kz2601.metamon.dsl.records.FieldCoding
import jp.kz2601.metamon.dsl.records.JSONValue
import jp.kz2601.metamon.dsl.records.Record
import jp.kz2601.metamon.renderer.components.lists.CalendarMonth
import jp.kz2601.metamon.renderer.components.lists.ChartBuckets
import jp.kz2601.metamon.renderer.components.lists.ChartReadout
import jp.kz2601.metamon.renderer.components.lists.GaugeValue
import jp.kz2601.metamon.renderer.components.lists.LeafData
import jp.kz2601.metamon.renderer.components.lists.ListRows
import jp.kz2601.metamon.renderer.components.lists.ListToggle
import jp.kz2601.metamon.renderer.components.lists.ProgressValue
import jp.kz2601.metamon.renderer.components.lists.StatValue
import jp.kz2601.metamon.renderer.components.lists.TableCells
import jp.kz2601.metamon.renderer.components.lists.TimelineRows
import jp.kz2601.metamon.renderer.components.lists.ValueLabels
import jp.kz2601.metamon.renderer.registry.ComponentRegistry
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test
import java.time.Instant
import java.time.LocalDate
import java.time.YearMonth
import java.time.ZoneId
import java.util.Locale

/** 一覧・集計・グラフの Leaf 10 種(#859)の値。描画と適合テストの描画の木が同じ関数を使う。 */
class ListLeavesTest {
    private val shop = Entity.parse(
        """{"id":"shop","name":"店","titleField":"name","fields":[
            {"id":"name","name":"名前","type":"string"},
            {"id":"area","name":"地域","type":"enum","options":["金沢","富山"]},
            {"id":"visited","name":"行った日","type":"date"},
            {"id":"price","name":"予算","type":"currency","unit":"円"},
            {"id":"done","name":"済み","type":"boolean"}]}""",
    )
    private val records = listOf(
        rec("s1", "name" to "金沢カレー", "area" to "金沢", "visited" to "2026-11-11", "price" to 900, "done" to true),
        rec("s2", "name" to "富山ブラック", "area" to "富山", "visited" to "2026-11-02", "price" to 1000),
        rec("s3", "name" to "金沢おでん", "area" to "金沢", "visited" to "2026-11-10", "price" to 2500),
    )

    private class Runtime(override val entityModels: List<Entity>, records: Map<String, List<Record>>) : DSLRuntime {
        override val recordsFlow: StateFlow<Map<String, List<Record>>> = MutableStateFlow(records)
        override fun onVoiceInput(componentId: String?) = Unit
    }

    private fun data() = LeafData(Runtime(listOf(shop), mapOf("shop" to records)), mapOf("shop" to records))

    private fun c(json: String): Component = DslJson.decodeFromString(Component.serializer(), json)

    private fun rec(id: String, vararg pairs: Pair<String, Any?>) = Record(id, pairs.associate { it.first to JSONValue.of(it.second) })

    private var saved: Triple<Instant?, ZoneId, Locale>? = null

    @Before
    fun fix() {
        saved = Triple(DSLClock.fixed, FieldCoding.zone, Locale.getDefault())
        DSLClock.fixed = Instant.parse("2026-11-12T03:00:00Z")
        FieldCoding.zone = ZoneId.of("Asia/Tokyo")
        Locale.setDefault(Locale.JAPAN)
    }

    @After
    fun restore() {
        saved?.let { (now, zone, locale) ->
            DSLClock.fixed = now
            FieldCoding.zone = zone
            Locale.setDefault(locale)
        }
    }

    @Test
    fun allTenTypesAreRegistered() {
        val types = listOf(
            ComponentType.CALENDAR, ComponentType.CHART, ComponentType.GAUGE, ComponentType.GRID_TABLE, ComponentType.LIST,
            ComponentType.PROGRESS_BAR, ComponentType.SEARCH_BAR, ComponentType.STAT, ComponentType.TABLE, ComponentType.TIMELINE,
        )
        types.forEach { assertTrue(it.wire, ComponentRegistry.standard.builder(it) != null) }
    }

    @Test
    fun listRowsSortSearchAndTexts() {
        val list = c("""{"type":"list","entity":"shop","searchField":"q","item":{"title":"name","subtitle":"area","trailing":"price","toggle":"done"},"sort":{"field":"price","direction":"asc"}}""")
        val d = data()
        assertEquals(listOf("s1", "s2", "s3"), ListRows.rows(list, d, null).map { it.id })
        // 検索は title / subtitle の部分一致(大小無視)・前後の空白は無視
        assertEquals(listOf("s1", "s3"), ListRows.rows(list, d, " 金沢 ").map { it.id })
        assertEquals(listOf("s2"), ListRows.rows(list, d, "ブラック").map { it.id })
        val t = ListRows.texts(list.item, records[1], shop, d)
        assertEquals(ListRows.Texts("富山ブラック", "富山", "¥1,000"), t)
        val toggle = ListToggle.field(list.item, shop)!!
        assertTrue(ListToggle.isOn(records[0], toggle.id, false, emptyMap(), DSLClock.now()))
        assertFalse(ListToggle.isOn(records[1], toggle.id, false, emptyMap(), DSLClock.now()))
        assertTrue("保存待ちの値を優先", ListToggle.isOn(records[1], toggle.id, false, mapOf("s2" to true), DSLClock.now()))
        assertEquals(mapOf("done" to JSONValue.Bool(false)), ListToggle.patch(records[0], "done", false, false, DSLClock.now()))
    }

    @Test
    fun statAggregatesWithFilterAndFormat() {
        val d = data()
        val avg = c("""{"type":"stat","label":"金沢の平均","entity":"shop","aggregate":"avg","field":"price","filter":{"field":"area","op":"eq","value":"金沢"}}""")
        assertEquals(StatValue.Resolved("¥1,700", false), StatValue.resolve(avg, d))
        val count = c("""{"type":"stat","label":"今週","entity":"shop","aggregate":"count","filter":{"field":"visited","op":"thisWeek"}}""")
        assertEquals("2 件", StatValue.resolve(count, d).text)
        val latest = c("""{"type":"stat","entity":"shop","aggregate":"latest","field":"name","by":"visited"}""")
        assertEquals("金沢カレー", StatValue.resolve(latest, d).text)
        val expr = c("""{"type":"stat","value":{"op":"div","args":[1,0]}}""")
        assertEquals(StatValue.Resolved("—", true), StatValue.resolve(expr, d))
    }

    @Test
    fun tableAndTimeline() {
        val d = data()
        val table = c("""{"type":"table","entity":"shop","groupBy":"area","columns":[{"field":"area"},{"field":"price","aggregate":"sum"},{"field":"price","aggregate":"avg"},{"field":"name","aggregate":"count"}]}""")
        assertEquals(listOf("地域", "予算の合計", "予算の平均", "件数"), table.columns.map { TableCells.header(it, shop, ValueLabels.JAPANESE) })
        assertEquals(
            listOf(listOf("金沢", "¥3,400", "¥1,700", "2"), listOf("富山", "¥1,000", "¥1,000", "1")),
            TableCells.rows(table, d).map { it.texts },
        )
        val timeline = c("""{"type":"timeline","entity":"shop","dateField":"visited","titleField":"name","limit":2}""")
        assertEquals(listOf("s1", "s3"), TimelineRows.rows(timeline, d).map { it.id })
    }

    @Test
    fun progressAndGauge() {
        val d = data()
        val bar = c("""{"type":"progressBar","label":"予算","entity":"shop","field":"price","target":8000}""")
        assertEquals(4400.0 / 8000, ProgressValue.ratio(bar, d), 1e-9)
        assertEquals("55%", ProgressValue.percentText(ProgressValue.ratio(bar, d), null))
        assertEquals("100%", ProgressValue.percentText(1.2, null))
        assertEquals("120%", ProgressValue.percentText(1.2, "negative"))
        assertEquals("999%+", ProgressValue.percentText(12.0, "positive"))
        assertEquals(ProgressValue.Level.SUCCESS, ProgressValue.level(1.0, "positive"))
        assertEquals(ProgressValue.Level.NORMAL, ProgressValue.level(1.0, "negative"))
        assertEquals(ProgressValue.Level.OVER, ProgressValue.level(1.0, null))
        val literal = c("""{"type":"progressBar","value":0.25}""")
        assertEquals(0.25, ProgressValue.ratio(literal, d), 0.0)
        val gauge = c("""{"type":"gauge","entity":"shop","aggregate":"count","min":0,"max":10}""")
        assertEquals(3.0 to "件", GaugeValue.value(gauge, d))
    }

    /** res の strings_lists.xml から [ValueLabels] を作る(書式の `%1$s` は `%s` に)。 */
    private fun labels(dir: String): ValueLabels {
        val xml = java.io.File("src/main/res/$dir/strings_lists.xml").readText()
        val strings = Regex("""<string name="([^"]+)">([^<]*)</string>""").findAll(xml).associate { it.groupValues[1] to it.groupValues[2] }
        fun s(name: String) = strings.getValue("lists_$name").replace(Regex("""%\d\$"""), "%")
        return ValueLabels(
            empty = s("empty"), count = s("count"), sum = s("sum"), avg = s("avg"), min = s("min"), max = s("max"),
            countUnit = s("count_unit"), noData = s("no_data"), chartOne = s("chart_summary_one"), chartMany = s("chart_summary_many"),
            everyone = s("everyone"), member = s("member"),
        )
    }

    @Test
    fun japaneseLabelsMatchStrings() {
        // 適合テストの描画の木が使う日本語は、画面の日本語(strings_lists.xml)と同じ
        assertEquals(labels("values"), ValueLabels.JAPANESE)
    }

    @Test
    fun englishTableHeaders() {
        val table = c("""{"type":"table","entity":"shop","groupBy":"area","columns":[{"field":"area"},{"field":"price","aggregate":"sum"},{"field":"price","aggregate":"avg"},{"field":"price","aggregate":"count"}]}""")
        assertEquals(listOf("地域", "Total 予算", "Avg 予算", "Count"), table.columns.map { TableCells.header(it, shop, labels("values-en")) })
    }

    @Test
    fun chartBucketsAndReadout() {
        val d = data()
        val chart = c("""{"type":"chart","entity":"shop","groupBy":"area","aggregate":"sum","field":"price","kind":"pie"}""")
        val buckets = ChartBuckets.buckets(chart, d)
        assertEquals(listOf("金沢" to 3400.0, "富山" to 1000.0), buckets.map { it.label to it.value })
        assertEquals("2 項目。最大は 金沢 の 3,400、最小は 富山 の 1,000", ChartReadout.summary(buckets, ValueLabels.JAPANESE, Locale.JAPAN))
        assertEquals("1 項目。3月 の 12", ChartReadout.summary(listOf(Aggregator.Bucket("m", "3月", 12.0)), ValueLabels.JAPANESE, Locale.JAPAN))
        assertEquals("データがありません", ChartReadout.summary(emptyList(), ValueLabels.JAPANESE))
        val en = labels("values-en")
        assertEquals("2 items. Highest: 金沢 (3,400). Lowest: 富山 (1,000)", ChartReadout.summary(buckets, en, Locale.US))
        assertEquals("1 items. 3月: 12", ChartReadout.summary(listOf(Aggregator.Bucket("m", "3月", 12.0)), en, Locale.US))
        assertEquals("No data yet", ChartReadout.summary(emptyList(), en))
        assertEquals("3月: 12.5", ChartReadout.line("3月", 12.5, Locale.JAPAN))
        assertEquals(0, ChartReadout.index(1000.0, listOf(3400.0, 1000.0)))
        assertEquals(1, ChartReadout.index(4000.0, listOf(3400.0, 1000.0)))
        assertNull(ChartReadout.index(1.0, emptyList()))
        assertEquals("とても長い…", ChartBuckets.axisLabel("とても長いカテゴリ"))
        assertEquals("短い名前", ChartBuckets.axisLabel("短い名前"))
    }

    @Test
    fun calendarMonthCellsAndDays() {
        // 2026-11-01 は日曜。日曜始まりなら先頭の空白なし、月曜始まりなら 6 つ
        assertEquals(30, CalendarMonth.cells(YearMonth.of(2026, 11), Locale.US).size)
        assertEquals(36, CalendarMonth.cells(YearMonth.of(2026, 11), Locale.FRANCE).size)
        val byDay = CalendarMonth.byDay(records, "visited")
        assertEquals(listOf("s1"), byDay[LocalDate.of(2026, 11, 11)]?.map { it.id })
    }
}
