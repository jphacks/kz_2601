package jp.kz2601.metamon.widget

import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.AppDocument
import jp.kz2601.metamon.dsl.decode
import jp.kz2601.metamon.dsl.records.Entity
import jp.kz2601.metamon.dsl.records.JSONValue
import jp.kz2601.metamon.dsl.records.Record
import jp.kz2601.metamon.dsl.records.entityModels
import jp.kz2601.metamon.renderer.DSLRuntime
import jp.kz2601.metamon.renderer.EntityLoadState
import jp.kz2601.metamon.renderer.components.lists.LeafData
import jp.kz2601.metamon.renderer.components.lists.StatValue
import jp.kz2601.metamon.services.consent.InMemoryKeyValueStore
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import java.time.Instant
import java.time.LocalDate
import java.time.ZoneId
import java.util.Locale

/** 記録 → ウィジェットの中身(#912・iOS の WidgetSnapshotWriterTests)、消す時機、押した時の開き先。 */
class WidgetSnapshotTest {
    private val dsl = AppDSL.decode(
        """
        {"schemaVersion":1,"app":{"name":"家計簿","voiceEntry":"none"},"initialScreen":"home",
         "entities":[{"id":"expense","name":"支出","titleField":"title","fields":[
           {"id":"title","name":"内容","type":"string"},
           {"id":"amount","name":"金額","type":"currency","unit":"円"},
           {"id":"date","name":"日付","type":"date"},
           {"id":"secret","name":"秘密","type":"number","hidden":true}]}],
         "screens":[{"id":"home","title":"家計簿","components":[
           {"type":"stat","label":"今月の支出","entity":"expense","aggregate":"sum","field":"amount","filter":{"field":"date","op":"thisMonth"}},
           {"type":"section","children":[{"type":"stat","label":"件数","entity":"expense","aggregate":"count"}]},
           {"type":"stat","label":"平均","entity":"expense","aggregate":"avg","field":"amount"},
           {"type":"stat","label":"4 個目","entity":"expense","aggregate":"max","field":"amount"}]}]}
        """.trimIndent(),
    )
    private val voiceOnly = AppDSL.decode(
        """{"schemaVersion":1,"app":{"name":"Tailor It","voiceEntry":"fullscreen"},"initialScreen":"home","screens":[{"id":"home","components":[]}]}""",
    )

    private class Runtime(override val entityModels: List<Entity>) : DSLRuntime {
        override fun onVoiceInput(componentId: String?) = Unit
    }

    private val runtime = Runtime(dsl.entityModels())
    private val zone = ZoneId.of("Asia/Tokyo")
    private val now = Instant.parse("2026-11-11T03:00:00Z") // 11/11(水) 12:00 JST

    private fun rec(id: String, title: String, amount: Int, date: String, created: String) = Record(
        id,
        mapOf("title" to JSONValue.of(title), "amount" to JSONValue.of(amount), "date" to JSONValue.of(date), "secret" to JSONValue.of(42)),
        createdAt = created,
    )

    private val records = listOf(
        rec("r1", "コンビニ", 680, "2026-11-11", "2026-11-11T01:00:00Z"),
        rec("r2", "電車", 420, "2026-11-10", "2026-11-10T01:00:00Z"),
        rec("r3", "ランチ", 1100, "2026-11-10", "2026-11-10T03:00:00Z"),
        rec("r4", "本", 2000, "2026-11-01", "2026-11-01T03:00:00Z"), // 7 日より前
    )

    private fun data(recs: Map<String, List<Record>>, states: Map<String, EntityLoadState> = emptyMap()) =
        LeafData(runtime, recs, now = now, loadStates = states)

    private fun build(recs: Map<String, List<Record>>, previous: WidgetSnapshot? = null, states: Map<String, EntityLoadState> = emptyMap()) =
        WidgetSnapshotBuilder.build("app-1", dsl, data(recs, states), previous, now, zone, Locale.JAPAN)

    @Test
    fun firstThreeStatsInScreenOrderWithTheSameValueAsTheApp() {
        val s = build(mapOf("expense" to records))!!
        assertEquals("app-1", s.appId)
        assertEquals("家計簿", s.appName)
        assertEquals(listOf("今月の支出", "件数", "平均"), s.stats.map { it.label })
        assertEquals("expense|sum|amount|今月の支出", s.stats[0].key)
        // アプリ内の stat と同じ関数・同じ値
        val d = data(mapOf("expense" to records))
        val inApp = WidgetSnapshotBuilder.statComponents(dsl).take(3).map { StatValue.resolve(it, d, Locale.JAPAN).text }
        assertEquals(inApp, s.stats.map { it.value })
        assertEquals("¥4,200", s.stats[0].value)
    }

    @Test
    fun recentRowsNewestFirstWithoutHiddenFields() {
        val s = build(mapOf("expense" to records))!!
        assertEquals("支出", s.recentTitle)
        assertEquals(listOf("r1", "r3", "r2", "r4"), s.recent!!.map { it.id })
        assertEquals("コンビニ", s.recent!![0].title)
        assertEquals("¥680", s.recent!![0].value)
        assertTrue(s.recent!!.none { it.value == "42" })
    }

    @Test
    fun trendIsSevenDaysEndingTodayWithoutThePeriodFilter() {
        val t = build(mapOf("expense" to records))!!.trend!!
        val today = LocalDate.of(2026, 11, 11)
        assertEquals((6 downTo 0).map { today.minusDays(it.toLong()).toEpochDay() }, t.points.map { it.epochDay })
        assertEquals(listOf(0.0, 0.0, 0.0, 0.0, 0.0, 1520.0, 680.0), t.points.map { it.value })
        assertEquals("¥2,200", t.total) // 11/1 は入れない
        assertEquals("金額", t.metric)
    }

    @Test
    fun emptyEntityShowsNoRecords() {
        val s = build(mapOf("expense" to emptyList()))!!
        assertEquals(emptyList<WidgetSnapshot.Row>(), s.recent)
        assertEquals(List(7) { 0.0 }, s.trend!!.points.map { it.value })
    }

    @Test
    fun loadingKeepsThePreviousValuesOfTheSameApp() {
        val previous = build(mapOf("expense" to records))!!
        val loading = mapOf("expense" to EntityLoadState.LOADING)
        val s = build(emptyMap(), previous, loading)!!
        assertEquals(previous.stats.map { it.value }, s.stats.map { it.value })
        assertEquals(previous.recent, s.recent)
        // 別のアプリの前回値は使わない
        val other = WidgetSnapshotBuilder.build("app-2", dsl, data(emptyMap(), loading), previous, now, zone, Locale.JAPAN)!!
        assertTrue(other.stats.all { it.value == null })
        assertNull(other.recent)
    }

    @Test
    fun voiceOnlyHasNoSnapshot() {
        assertNull(WidgetSnapshotBuilder.build("app-1", voiceOnly, data(emptyMap()), null, now, zone, Locale.JAPAN))
    }

    @Test
    fun encodeDecodeRoundTrip() {
        val s = build(mapOf("expense" to records))!!
        assertEquals(s, WidgetSnapshot.decode(s.encode()))
        assertNull(WidgetSnapshot.decode("{broken"))
        assertTrue(s.hasSameContent(s.copy(updatedAtMillis = 1)))
        assertFalse(s.hasSameContent(s.copy(appName = "x")))
    }

    // 書く時機・消す時機

    private val store = WidgetSnapshotStore(InMemoryKeyValueStore())
    private var refreshed = 0
    private val writer = WidgetSnapshotWriter(store, onChanged = { refreshed++ })
    private val doc = AppDocument(appId = "app-1", dsl = dsl)

    @Test
    fun writesOnlyWhenTheContentChanges() {
        assertTrue(writer.write(doc, runtime, mapOf("expense" to records)))
        assertFalse("同じ中身なら描き直さない", writer.write(doc, runtime, mapOf("expense" to records)))
        assertTrue(writer.write(doc, runtime, mapOf("expense" to records.drop(1))))
        assertEquals(2, refreshed)
        assertEquals("app-1", store.load()!!.appId)
    }

    @Test
    fun voiceOnlyClearsTheSnapshot() {
        writer.write(doc, runtime, mapOf("expense" to records))
        assertTrue(writer.write(AppDocument(appId = "app-1", dsl = voiceOnly), runtime, emptyMap()))
        assertNull(store.load())
    }

    @Test
    fun wipeClearsTheSnapshot() {
        writer.write(doc, runtime, mapOf("expense" to records))
        writer.clear()
        assertNull(store.load())
        assertEquals(2, refreshed)
    }

    @Test
    fun deletingTheShownAppClearsItButOtherAppsDoNot() {
        writer.write(doc, runtime, mapOf("expense" to records))
        writer.forgetApp("app-other")
        assertEquals("app-1", store.load()?.appId)
        writer.forgetApp("app-1")
        assertNull(store.load())
    }

    // 押した時の開き先

    @Test
    fun deepLinkFormat() {
        assertEquals("tailorit://widget?app=app-1", WidgetLink.uri("app-1"))
        assertEquals("tailorit://widget", WidgetLink.uri(null))
        assertEquals("a b/c", WidgetLink.appId(WidgetLink.uri("a b/c")))
        assertNull(WidgetLink.appId("tailorit://widget"))
        assertNull(WidgetLink.appId("tailorit://import/abc?app=x"))
        assertNull(WidgetLink.appId("tailorit://widgetx?app=x"))
    }

    @Test
    fun onlyTheWidgetActionOpensAnApp() {
        WidgetLink.consume()
        WidgetLink.offer("android.intent.action.VIEW", WidgetLink.uri("app-1"))
        assertNull(WidgetLink.consume())
        WidgetLink.offer(WidgetLink.ACTION, WidgetLink.uri("app-1"))
        assertEquals("app-1", WidgetLink.consume())
        assertNull(WidgetLink.consume())
    }
}
