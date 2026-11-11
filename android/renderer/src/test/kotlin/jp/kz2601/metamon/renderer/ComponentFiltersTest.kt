package jp.kz2601.metamon.renderer

import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.ComponentType
import jp.kz2601.metamon.dsl.DslJson
import jp.kz2601.metamon.dsl.FilterOp
import jp.kz2601.metamon.dsl.records.Entity
import jp.kz2601.metamon.dsl.records.JSONValue
import jp.kz2601.metamon.dsl.records.Record
import jp.kz2601.metamon.dsl.records.filters
import jp.kz2601.metamon.renderer.components.lists.ChartBuckets
import jp.kz2601.metamon.renderer.components.lists.LeafData
import jp.kz2601.metamon.renderer.components.lists.ListRows
import jp.kz2601.metamon.renderer.components.lists.StatValue
import jp.kz2601.metamon.renderer.components.lists.TableCells
import jp.kz2601.metamon.renderer.components.play.FlashcardRules
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test
import jp.kz2601.metamon.dsl.Filter as DslFilter

/**
 * 部品の filter が配列でも絞り込む(iOS の `Component.filters`・`Filter.decodeOneOrMany` と同じ: 配列は AND)。
 * 以前は :dsl の Component が 1 件だけを読み、配列だと絞り込みが消えて全件が出ていた。
 */
class ComponentFiltersTest {
    private val shop = Entity.parse(
        """{"id":"shop","name":"店","titleField":"name","fields":[
            {"id":"name","name":"名前","type":"string"},
            {"id":"area","name":"地域","type":"enum","options":["金沢","富山"]},
            {"id":"price","name":"予算","type":"integer"},
            {"id":"done","name":"済み","type":"boolean"}]}""",
    )
    private val records = listOf(
        rec("s1", "name" to "金沢カレー", "area" to "金沢", "price" to 900, "done" to true),
        rec("s2", "name" to "富山ブラック", "area" to "富山", "price" to 1000),
        rec("s3", "name" to "金沢おでん", "area" to "金沢", "price" to 2500),
        rec("s4", "name" to "金沢寿司", "area" to "金沢", "price" to 4000, "done" to false),
    )
    private val all = mapOf("shop" to records)

    private class Runtime(override val entityModels: List<Entity>, records: Map<String, List<Record>>) : DSLRuntime {
        override val recordsFlow: StateFlow<Map<String, List<Record>>> = MutableStateFlow(records)
        override fun onVoiceInput(componentId: String?) = Unit
    }

    private val rt = Runtime(listOf(shop), all)
    private fun data() = LeafData(rt, all)
    private fun c(json: String): Component = DslJson.decodeFromString(Component.serializer(), json)
    private fun rec(id: String, vararg pairs: Pair<String, Any?>) = Record(id, pairs.associate { it.first to JSONValue.of(it.second) })

    /** 金沢 かつ 予算 3000 未満 → s1・s3。 */
    private val both = """[{"field":"area","op":"eq","value":"金沢"},{"field":"price","op":"lt","value":3000}]"""

    @Test
    fun componentReadsOneOrManyFilters() {
        assertEquals(2, c("""{"type":"list","entity":"shop","filter":$both}""").filters.size)
        assertEquals(1, c("""{"type":"list","entity":"shop","filter":{"field":"area","op":"eq","value":"金沢"}}""").filters.size)
        assertTrue(c("""{"type":"list","entity":"shop"}""").filters.isEmpty())
        // 壊れた要素だけを捨てる(iOS と同じ)
        assertEquals(1, c("""{"type":"list","entity":"shop","filter":[{"field":"area","op":"eq","value":"金沢"},{"op":"eq"}]}""").filters.size)
        // コードで作った部品は生成器の型 filter から
        val coded = Component(type = ComponentType.LIST, entity = "shop", filter = DslFilter(field = "area", op = FilterOp.EQ, value = kotlinx.serialization.json.JsonPrimitive("富山")))
        assertEquals(listOf("s2"), data().filtered(coded).map { it.id })
    }

    @Test
    fun listAndTableApplyEveryFilter() {
        val list = c("""{"type":"list","entity":"shop","filter":$both,"item":{"titleField":"name"}}""")
        assertEquals(listOf("s1", "s3"), ListRows.rows(list, data(), null).map { it.id }.sorted())
        val table = c("""{"type":"table","entity":"shop","filter":$both,"columns":[{"field":"name"}]}""")
        assertEquals(listOf("s1", "s3"), TableCells.rows(table, data()).map { it.id }.sorted())
    }

    @Test
    fun statAndChartApplyEveryFilter() {
        val stat = c("""{"type":"stat","entity":"shop","aggregate":"count","filter":$both}""")
        assertEquals("2 件", StatValue.resolve(stat, data(), java.util.Locale.JAPAN).text)
        val chart = c("""{"type":"chart","entity":"shop","kind":"bar","groupBy":"area","aggregate":"count","filter":$both}""")
        assertEquals(listOf(2.0), ChartBuckets.buckets(chart, data()).map { it.value })
    }

    @Test
    fun playLeavesApplyEveryFilter() {
        val picker = c("""{"type":"randomPicker","entity":"shop","titleField":"name","filter":$both}""")
        assertEquals(listOf("s1", "s3"), LeafData(rt, all).filtered(picker).map { it.id }.sorted())
        // 単語カード: toggle の filter(まだ)と他の filter を両方当てる → 金沢 かつ 未済み
        val cards = c(
            """{"type":"flashcards","entity":"shop","front":"name","toggle":"done",
                "filter":[{"field":"done","op":"eq","value":false},{"field":"area","op":"eq","value":"金沢"}]}""",
        )
        assertEquals(false, FlashcardRules.learnedFilter(cards))
        assertEquals(listOf("s3", "s4"), FlashcardRules.cards(cards, rt, all, emptyMap()).map { it.id }.sorted())
    }
}
