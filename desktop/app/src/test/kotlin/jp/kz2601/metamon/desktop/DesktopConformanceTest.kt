package jp.kz2601.metamon.desktop

import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.decode
import jp.kz2601.metamon.dsl.expr.DSLClock
import jp.kz2601.metamon.dsl.records.FieldCoding
import jp.kz2601.metamon.dsl.records.Record
import jp.kz2601.metamon.dsl.records.entityModels
import jp.kz2601.metamon.dsl.screen
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.booleanOrNull
import kotlinx.serialization.json.contentOrNull
import kotlinx.serialization.json.doubleOrNull
import kotlinx.serialization.json.int
import kotlinx.serialization.json.intOrNull
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Assert.fail
import org.junit.Test
import java.io.File
import java.time.OffsetDateTime
import java.time.ZoneId

/**
 * docs/dsl/conformance/cases(全 OS 共通の期待値・iOS が書く)のうち、デスクトップ版が今比べられる部分を回す。
 * - 全 case の DSL が共有の :dsl で読めて、画面の id・見出し・`layout`(grid / form / voice)が同じ。
 * - 部品の並び(area は (行, 列, DSL の順)、その後に area 無し)・`cell`・`stacked`・`band`・ローカルグリッドの `cell`。
 * - デスクトップ版が描ける部品(DesktopLeaves)は、expected が rendered の時に rendered。値を比べる props は [comparedProps]。
 * 操作(steps)はまだ見ない(#1740)。
 */
class DesktopConformanceTest {
    private val docsDir = File(requireNotNull(System.getProperty("metamon.dslDocsDir")) { "metamon.dslDocsDir が未設定(Gradle から実行する)" })
    private val casesDir = docsDir.resolve("conformance/cases")

    /** 型ごとに比べる props のキー。 */
    private val comparedProps = mapOf(
        "text" to listOf("text"),
        "label" to listOf("text"),
        "badge" to listOf("text"),
        "markdown" to listOf("text"),
        "button" to listOf("label"),
        "link" to listOf("text", "url"),
        "stat" to listOf("value", "unavailable"),
        "progressBar" to listOf("ratio", "percent"),
        "gauge" to listOf("value", "unit"),
        "chart" to listOf("buckets"),
        "list" to listOf("entity", "rows", "emptyText"),
        "table" to listOf("entity", "columns", "rows", "limit", "groupBy"),
        "timeline" to listOf("entity", "rows", "limit"),
        "searchBar" to listOf("field", "placeholder", "query"),
        "calendar" to listOf("entity", "title"),
        "gridTable" to listOf("entity", "title"),
        "feed" to listOf("title", "limit"),
        "matchList" to listOf("entity", "show", "title"),
    )

    private data class Got(val c: Component, val area: GridArea?, val stacked: Boolean, val band: Int?)

    @Test
    fun conformanceCases() {
        val cases = casesDir.listFiles { f -> f.resolve("expected.json").isFile }.orEmpty().sortedBy { it.name }
        assertTrue("case が無い: $casesDir", cases.isNotEmpty())
        val failures = mutableListOf<String>()
        var screens = 0
        val compared = sortedMapOf<String, Int>()
        val zone = FieldCoding.zone
        try {
            for (dir in cases) {
                val case = Json.parseToJsonElement(dir.resolve("case.json").readText()) as JsonObject
                val expected = Json.parseToJsonElement(dir.resolve("expected.json").readText()) as JsonObject
                val dsl = try {
                    decodeDsl(case["dsl"]!!)
                } catch (e: Exception) {
                    failures += "${dir.name}: DSL が読めない $e"
                    continue
                }
                val device = case["device"] as? JsonObject
                DSLClock.fixed = OffsetDateTime.parse(device?.str("now") ?: "2026-11-12T12:00:00+09:00").toInstant()
                FieldCoding.zone = ZoneId.of(device?.str("timeZone") ?: "Asia/Tokyo")
                val records = (case["records"] as? JsonObject).orEmpty().mapValues { (_, v) -> (v as JsonArray).map(Record::from) }
                val data = LeafData(dsl.entityModels(), { records[it].orEmpty() }, device?.str("userId") ?: "me")
                for (s in expected["screens"] as JsonArray) {
                    s as JsonObject
                    screens++
                    val id = s.str("id")!!
                    val screen = dsl.screen(id)
                    if (screen == null) {
                        failures += "${dir.name}/$id: 画面が無い"
                        continue
                    }
                    val at = "${dir.name}/$id"
                    if (screen.title != s.str("title").orEmpty()) failures += "$at: 見出し ${screen.title} != ${s.str("title")}"
                    if (screen.layoutKind != s.str("layout")) failures += "$at: layout ${screen.layoutKind} != ${s.str("layout")}"
                    val placement = screen.gridPlacement
                    val bands = if (screen.isFormLayout) formBands(placement.placed) else emptyList()
                    val got = placement.placedInReadingOrder.map { (c, a) ->
                        Got(c, a, stacked = false, band = bands.indexOfFirst { b -> b.items.any { it.component === c } }.takeIf { it >= 0 })
                    } + placement.stacked.map { Got(it, null, stacked = true, band = null) }
                    compare(at, got, s["nodes"] as JsonArray, data, failures, compared)
                }
            }
        } finally {
            DSLClock.fixed = null
            FieldCoding.zone = zone
        }
        if (failures.isNotEmpty()) fail("${failures.size} 件が違う:\n" + failures.joinToString("\n"))
        println("desktop conformance: ${cases.size} cases / $screens screens OK / props compared: $compared")
    }

    @Test
    fun bundledExampleLoads() {
        val dsl = Examples.load("shopping")
        assertEquals("home", dsl.initialScreen)
        assertTrue(dsl.screens.isNotEmpty())
    }

    @Test
    fun mockStoreSavesAndValidates() {
        val dsl = Examples.load("shopping")
        val entity = dsl.entities.first { it.id == "groceries" }
        assertEquals(listOf("品名"), entity.missingRequired(entity.defaultDraft()))
        val store = MockStore().save("groceries", null, entity.defaultDraft() + ("name" to "牛乳"))
        assertEquals("new-1", store.list("groceries").single().id)
        assertEquals("牛乳", store.list("groceries").single().data["name"])
        assertTrue(store.delete("groceries", "new-1").list("groceries").isEmpty())
    }

    @Test
    fun mockStoreFeedsAggregates() {
        val dsl = Examples.load("kakeibo")
        val store = MockStore.fromJson("""{"expense": [{"id": "a", "data": {"amount": 1200, "category": "食費"}}, {"id": "b", "data": {"amount": 320, "category": "交通"}}]}""")
        val data = LeafData.of(dsl, store)
        val chart = dsl.screen("stats")!!.components.first { it.id == "by_month" }
        assertEquals(1520.0, data.filtered(chart).sumOf { it["amount"].doubleValue ?: 0.0 }, 0.0)
    }

    @Test
    fun gridMetricsCapWidthAndRowHeight() {
        val wide = DesktopGridMetrics.resolve(1600f, 1000f, 16f)
        assertEquals(DesktopGridMetrics.MAX_WIDTH, wide.frameWidth)
        assertEquals((DesktopGridMetrics.MAX_WIDTH - 32f) / 6f, wide.cellW)
        assertEquals(DesktopGridMetrics.MIN_CELL_HEIGHT, DesktopGridMetrics.resolve(400f, 300f, 16f).cellH)
        assertEquals(DesktopGridMetrics.MAX_CELL_HEIGHT, DesktopGridMetrics.resolve(400f, 3000f, 16f).cellH)
    }

    @Test
    fun markdownBlocksAndInline() {
        val blocks = Markdown.blocks("# 見出し\n- いち\n1. に\n\n本文 **太字**")
        assertEquals(Markdown.Block.Heading(1, "見出し"), blocks[0])
        assertEquals(Markdown.Block.Item("•", "いち"), blocks[1])
        assertEquals(Markdown.Block.Item("1.", "に"), blocks[2])
        assertEquals("本文 太字 と リンク", Markdown.inline("本文 **太字** と [リンク](https://example.com)").text)
    }

    @Test
    fun tableViewSortIsDisplayOnly() {
        val rows = listOf(TableCells.Row("a", listOf("みかん", "¥1,200")), TableCells.Row("b", listOf("りんご", "¥300")), TableCells.Row("c", listOf("いちご", "¥45")))
        assertEquals(listOf("c", "b", "a"), TableCells.viewSorted(rows, 1, true).map { it.id })
        assertEquals(listOf("a", "b", "c"), TableCells.viewSorted(rows, 1, false).map { it.id })
        assertEquals(listOf("a", "b", "c"), TableCells.viewSorted(rows, null, true).map { it.id })
        assertEquals(1200.0, TableCells.number("¥1,200")!!, 0.0)
    }

    @Test
    fun listRowsFor1000RecordsAndSearch() {
        val dsl = Examples.load("shopping")
        val list = dsl.screen("home")!!.components.flatMap { it.children }.first { it.type == jp.kz2601.metamon.dsl.ComponentType.LIST }
        val entityId = list.entity!!
        val store = MockStore(mapOf(entityId to (1..1000).map { Row("r$it", mapOf("name" to "品$it", "done" to "false", "added" to "2026-11-01")) }))
        val data = LeafData.of(dsl, store)
        val started = System.nanoTime()
        val rows = ListRows.rows(list, data, null)
        assertEquals(1000, rows.size)
        assertTrue("1000 行の行の計算が遅い", (System.nanoTime() - started) / 1_000_000 < 2000)
        assertEquals(999, LeafData.of(dsl, store.delete(entityId, "r1")).let { ListRows.rows(list, it, null).size })
    }

    private fun decodeDsl(e: JsonElement): AppDSL =
        if (e is JsonPrimitive && e.isString) AppDSL.decode(docsDir.resolve(e.content).readText()) else AppDSL.decode(e.toString())

    /** 部品の木を比べる(並び・配置・state・値)。 */
    private fun compare(
        at: String,
        got: List<Got>,
        nodes: JsonArray,
        data: LeafData,
        failures: MutableList<String>,
        compared: MutableMap<String, Int>,
    ) {
        val want = nodes.map { it as JsonObject }
        val wantKeys = want.map { key(it.str("id"), it.str("type")) }
        val gotKeys = got.map { key(it.c.id, DesktopLeaves.typeName(it.c)) }
        if (wantKeys != gotKeys) {
            failures += "$at: 部品の並び $gotKeys != $wantKeys"
            return
        }
        for ((g, n) in got.zip(want)) {
            val c = g.c
            val here = "$at/${key(c.id, DesktopLeaves.typeName(c))}"
            val mineCell = g.area?.let { a -> listOf(a.row1 - 1, a.col1, a.rowSpan, a.colSpan) }
            val wantCell = (n["cell"] as? JsonObject)?.let { o -> listOf("row", "col", "rowSpan", "colSpan").map { (o[it] as JsonPrimitive).int } }
            if (mineCell != wantCell) failures += "$here: cell $mineCell != $wantCell"
            if (g.stacked != ((n["stacked"] as? JsonPrimitive)?.booleanOrNull == true)) failures += "$here: stacked ${g.stacked}"
            val wantBand = (n["band"] as? JsonPrimitive)?.intOrNull
            if (g.band != wantBand) failures += "$here: band ${g.band} != $wantBand"

            val state = n.str("state")
            val mine = DesktopLeaves.state(c)
            // デスクトップ版で描けない部品は unavailable(iOS は rendered)でよい。知らない type の判定は同じであること
            if ((state == "unknown") != (mine == "unknown")) failures += "$here: state $mine != $state"
            val props = n["props"] as? JsonObject
            if (mine == "rendered" && state == "rendered" && props != null) {
                val type = DesktopLeaves.typeName(c)
                val mineProps = resolvedProps(c, data)
                for (k in comparedProps[type].orEmpty()) {
                    val w = props[k] ?: continue
                    val m = mineProps[k]
                    if (m == null || !sameJson(m, w)) failures += "$here: $k $m != $w"
                    compared[type] = (compared[type] ?: 0) + 1
                }
            }
            val children = n["children"] as? JsonArray ?: continue
            val local = c.localGridPlacement
            if (((n["localGrid"] as? JsonPrimitive)?.booleanOrNull == true) != (local != null)) failures += "$here: localGrid ${local != null}"
            val kids = if (local != null) {
                gridPlacement(c.children).placedInReadingOrder.map { Got(it.component, it.area, false, null) }
            } else {
                c.children.map { Got(it, null, false, null) }
            }
            compare(here, kids, children, data, failures, compared)
        }
    }

    /** デスクトップ版が描く値(画面と同じ関数から)。 */
    private fun resolvedProps(c: Component, data: LeafData): Map<String, JsonElement> = buildMap {
        fun putText(k: String, v: String?) {
            v?.let { put(k, JsonPrimitive(it)) }
        }
        when (c.type.wire) {
            "text", "markdown" -> putText("text", c.text)
            "label", "badge" -> putText("text", c.text ?: c.label)
            "button" -> putText("label", c.label)
            "link" -> {
                putText("text", c.text ?: c.label ?: c.url)
                putText("url", c.url)
            }
            "stat" -> StatValue.resolve(c, data).let { (text, unavailable) ->
                put("value", JsonPrimitive(text))
                put("unavailable", JsonPrimitive(unavailable))
            }
            "progressBar" -> {
                put("ratio", JsonPrimitive(ProgressValue.ratio(c, data)))
                put("percent", JsonPrimitive(ProgressValue.percent(c, data)))
            }
            "gauge" -> GaugeValue.value(c, data).let { (v, unit) ->
                put("value", JsonPrimitive(v))
                unit?.let { put("unit", JsonPrimitive(it)) }
            }
            "chart" -> put(
                "buckets",
                JsonArray(ChartBuckets.buckets(c, data).map { JsonObject(mapOf("label" to JsonPrimitive(it.label), "value" to JsonPrimitive(it.value))) }),
            )
            else -> Unit
        }
        dataProps(c, data).forEach { (k, v) -> put(k, v) }
    }

    /** データの部品(#1744)。行は画面と同じ ListRows・TableCells・TimelineRows から。検索欄は空(steps は #1740)。 */
    private fun dataProps(c: Component, data: LeafData): Map<String, JsonElement> = buildMap {
        fun putText(k: String, v: String?) {
            v?.let { put(k, JsonPrimitive(it)) }
        }
        if (c.type !in DATA_LEAVES && c.type.wire != "calendar") return@buildMap
        putText("entity", c.entity)
        putText("title", c.title)
        c.limit?.let { put("limit", JsonPrimitive(it)) }
        val entity = data.entity(c.entity)
        when (c.type.wire) {
            "list" -> {
                val rows = ListRows.rows(c, data, null)
                val toggle = ListToggle.field(c.item, entity)
                put("rows", JsonArray(rows.map { r ->
                    val t = ListRows.texts(c.item, r, entity, data)
                    JsonObject(buildMap {
                        put("recordId", JsonPrimitive(r.id))
                        put("title", JsonPrimitive(t.title))
                        t.subtitle?.let { put("subtitle", JsonPrimitive(it)) }
                        t.trailing?.let { put("trailing", JsonPrimitive(it)) }
                        toggle?.let { f -> put("checked", JsonPrimitive(ListToggle.isOn(r, f.id, ListToggle.isDaily(c.item), DSLClock.now()))) }
                    })
                }))
                if (rows.isEmpty()) putText("emptyText", c.emptyText ?: DataText.EMPTY)
            }
            "table" -> {
                putText("groupBy", c.groupBy)
                put("columns", JsonArray(c.columns.map { JsonPrimitive(TableCells.header(it, entity)) }))
                put("rows", JsonArray(TableCells.rows(c, data).map { r -> JsonArray(r.texts.map(::JsonPrimitive)) }))
            }
            "timeline" -> put("rows", JsonArray(TimelineRows.rows(c, data).map { r ->
                JsonObject(buildMap {
                    put("recordId", JsonPrimitive(r.id))
                    put("date", JsonPrimitive(DataText.text(data, r, c.dateField, entity)))
                    put("title", JsonPrimitive(DataText.text(data, r, c.titleField, entity)))
                    c.subtitleField?.let { DataText.text(data, r, it, entity) }?.takeIf { it.isNotEmpty() }?.let { put("subtitle", JsonPrimitive(it)) }
                })
            }))
            "searchBar" -> {
                putText("field", c.field)
                putText("placeholder", c.placeholder)
                put("query", JsonPrimitive(""))
            }
            "matchList" -> putText("show", c.show?.wire)
        }
    }

    /** JSON として同じか(数の 1200 と 1200.0 は同じ)。 */
    private fun sameJson(a: JsonElement, b: JsonElement): Boolean = when {
        a is JsonPrimitive && b is JsonPrimitive && !a.isString && !b.isString && a.doubleOrNull != null && b.doubleOrNull != null ->
            a.doubleOrNull == b.doubleOrNull
        a is JsonArray && b is JsonArray -> a.size == b.size && a.zip(b).all { (x, y) -> sameJson(x, y) }
        a is JsonObject && b is JsonObject -> a.keys == b.keys && a.keys.all { sameJson(a.getValue(it), b.getValue(it)) }
        else -> a == b
    }

    private fun key(id: String?, type: String?) = "${id ?: "-"}:${type.orEmpty()}"

    private fun JsonObject.str(k: String): String? = (this[k] as? JsonPrimitive)?.contentOrNull
}
