package jp.kz2601.metamon.renderer

import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.DslJson
import jp.kz2601.metamon.dsl.records.Entity
import jp.kz2601.metamon.dsl.records.JSONValue
import jp.kz2601.metamon.dsl.records.Record
import jp.kz2601.metamon.renderer.components.lists.AggregateLoad
import jp.kz2601.metamon.renderer.components.lists.GridTableNotice
import jp.kz2601.metamon.renderer.components.lists.LeafData
import jp.kz2601.metamon.renderer.components.lists.RecordsPlaceholder
import jp.kz2601.metamon.renderer.components.lists.StatValue
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test
import java.util.Locale

/** stat / progressBar / gauge / calendar / gridTable の読み込み中・取得失敗の出し分け(#1116)。 */
class AggregateLoadStateTest {
    private val shop = Entity.parse(
        """{"id":"shop","name":"店","titleField":"name","fields":[
            {"id":"name","name":"名前","type":"string"},
            {"id":"area","name":"地域","type":"string"},
            {"id":"price","name":"予算","type":"currency","unit":"円"}]}""",
    )
    private val one = listOf(Record("s1", mapOf("name" to JSONValue.of("カレー"), "area" to JSONValue.of("金沢"), "price" to JSONValue.of(900))))

    private class Runtime(override val entityModels: List<Entity>) : DSLRuntime {
        override fun onVoiceInput(componentId: String?) = Unit
    }

    private fun data(records: List<Record>?, state: EntityLoadState?) = LeafData(
        Runtime(listOf(shop)),
        records?.let { mapOf("shop" to it) } ?: emptyMap(),
        loadStates = state?.let { mapOf("shop" to it) } ?: emptyMap(),
    )

    private fun c(json: String): Component = DslJson.decodeFromString(Component.serializer(), json)

    private val sum = c("""{"type":"stat","entity":"shop","aggregate":"sum","field":"price"}""")
    private val bar = c("""{"type":"progressBar","entity":"shop","field":"price","target":1000}""")
    private val gauge = c("""{"type":"gauge","entity":"shop","aggregate":"count","min":0,"max":10}""")

    @Test
    fun statLoadingShowsDashNotZero() {
        val v = StatValue.resolve(sum, data(null, EntityLoadState.LOADING), Locale.JAPAN)
        assertEquals(StatValue.Resolved("—", unavailable = false, loading = true), v)
    }

    @Test
    fun statUnavailableShowsDash() {
        val v = StatValue.resolve(sum, data(emptyList(), EntityLoadState.UNAVAILABLE), Locale.JAPAN)
        assertEquals(StatValue.Resolved("—", unavailable = true), v)
    }

    @Test
    fun statWithLocalRecordsAggregatesEvenIfFailed() {
        // 手元に記録があれば取得失敗でも通常どおり(iOS と同じ)
        assertEquals("¥900", StatValue.resolve(sum, data(one, EntityLoadState.UNAVAILABLE), Locale.JAPAN).text)
        // 一度読めた entity の裏での取り直しは値のまま
        assertEquals("¥900", StatValue.resolve(sum, data(one, EntityLoadState.LOADING), Locale.JAPAN).text)
    }

    @Test
    fun statLoadedEmptyIsZero() {
        val v = StatValue.resolve(sum, data(emptyList(), EntityLoadState.LOADED), Locale.JAPAN)
        assertEquals(StatValue.Resolved("¥0", false), v)
    }

    @Test
    fun statFilteredToNothingWhileFailedIsUnavailable() {
        val filtered = c("""{"type":"stat","entity":"shop","aggregate":"count","filter":{"field":"area","op":"eq","value":"富山"}}""")
        assertEquals(StatValue.Resolved("—", true), StatValue.resolve(filtered, data(one, EntityLoadState.UNAVAILABLE), Locale.JAPAN))
    }

    @Test
    fun statValueModeFollowsReferencedEntities() {
        val expr = c("""{"type":"stat","value":{"op":"add","args":[{"agg":"count","entity":"shop"},1]}}""")
        assertEquals(true, StatValue.resolve(expr, data(null, EntityLoadState.LOADING)).loading)
        assertEquals(true, StatValue.resolve(expr, data(emptyList(), EntityLoadState.UNAVAILABLE)).unavailable)
        assertEquals(StatValue.Resolved("2", false), StatValue.resolve(expr, data(one, EntityLoadState.LOADED)))
    }

    @Test
    fun progressAndGaugeSelection() {
        for (comp in listOf(bar, gauge)) {
            assertEquals(EntityLoadState.LOADING, AggregateLoad.of(comp, data(null, EntityLoadState.LOADING)))
            assertEquals(EntityLoadState.UNAVAILABLE, AggregateLoad.of(comp, data(emptyList(), EntityLoadState.UNAVAILABLE)))
            assertEquals(EntityLoadState.LOADED, AggregateLoad.of(comp, data(one, EntityLoadState.UNAVAILABLE)))
            assertEquals(EntityLoadState.LOADED, AggregateLoad.of(comp, data(emptyList(), EntityLoadState.LOADED)))
            // 状態を持たないホスト(プレビュー・適合テスト)は読めた扱い
            assertEquals(EntityLoadState.LOADED, AggregateLoad.of(comp, data(null, null)))
        }
        // value のリテラルは記録を読まない
        val literal = c("""{"type":"progressBar","entity":"shop","value":0.25}""")
        assertEquals(EntityLoadState.LOADED, AggregateLoad.of(literal, data(null, EntityLoadState.UNAVAILABLE)))
    }

    @Test
    fun calendarEmptyDayFollowsEntityState() {
        // 選んだ日に記録が無い時の表示は entity の状態(骨組み・取得失敗+再試行・「予定はありません」)
        val cal = c("""{"type":"calendar","entity":"shop","dateField":"visited"}""")
        assertEquals(RecordsPlaceholder.SKELETON, data(null, EntityLoadState.LOADING).placeholder(cal.entity))
        assertEquals(RecordsPlaceholder.UNAVAILABLE, data(one, EntityLoadState.UNAVAILABLE).placeholder(cal.entity))
        assertEquals(RecordsPlaceholder.EMPTY, data(emptyList(), EntityLoadState.LOADED).placeholder(cal.entity))
    }

    @Test
    fun gridTableNotice() {
        val grid = c("""{"type":"gridTable","entity":"shop","columnField":"area","rowField":"name"}""")
        assertEquals(RecordsPlaceholder.SKELETON, GridTableNotice.of(data(null, EntityLoadState.LOADING), grid))
        // 失敗からの再試行中でも手元の記録があればマス目のまま
        assertNull(GridTableNotice.of(data(one, EntityLoadState.LOADING), grid))
        // 取得失敗は手元の記録があっても知らせる(iOS と同じ)
        assertEquals(RecordsPlaceholder.UNAVAILABLE, GridTableNotice.of(data(one, EntityLoadState.UNAVAILABLE), grid))
        assertNull(GridTableNotice.of(data(emptyList(), EntityLoadState.LOADED), grid))
        assertNull(GridTableNotice.of(data(null, null), grid))
    }
}
