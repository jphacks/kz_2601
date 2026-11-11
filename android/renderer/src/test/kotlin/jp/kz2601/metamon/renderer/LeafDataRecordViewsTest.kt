package jp.kz2601.metamon.renderer

import jp.kz2601.metamon.dsl.records.Entity
import jp.kz2601.metamon.dsl.records.JSONValue
import jp.kz2601.metamon.dsl.records.Record
import jp.kz2601.metamon.renderer.components.lists.LeafData
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import org.junit.Assert.assertEquals
import org.junit.Test

/**
 * 部品が記録を読む関数は LeafData の 1 か所(#1129・iOS の `ComputedValues.swift`)。
 * 一覧・集計系の [LeafData.viewRecords] は hidden を残し、計算・遊び系の [LeafData.visibleRecords] は落とす(まとめる前の PlayRecords と同じ)。
 */
class LeafDataRecordViewsTest {
    private val member = Entity.parse(
        """{"id":"member","name":"M","titleField":"name","fields":[
          {"id":"name","name":"名前","type":"string"},
          {"id":"paid","name":"払った","type":"currency"},
          {"id":"share","name":"負担","type":"currency"},
          {"id":"memo","name":"控え","type":"string","hidden":true},
          {"id":"balance","name":"差額","type":"currency","computed":{"op":"sub","args":[{"ref":"paid"},{"ref":"share"}]}}
        ]}""",
    )
    private val expense = Entity.parse(
        """{"id":"expense","name":"E","fields":[
          {"id":"who","name":"誰","type":"ref","refEntity":"member"}
        ]}""",
    )

    private class FakeRuntime(override val entityModels: List<Entity>, records: Map<String, List<Record>>) : DSLRuntime {
        override val recordsFlow: StateFlow<Map<String, List<Record>>> = MutableStateFlow(records)
        override fun onVoiceInput(componentId: String?) = Unit
    }

    private val all = mapOf(
        "member" to listOf(
            Record("a", mapOf("name" to JSONValue.Str("A"), "paid" to JSONValue.Num(3000.0), "share" to JSONValue.Num(1000.0), "memo" to JSONValue.Str("x"))),
        ),
        "expense" to listOf(Record("e1", mapOf("who" to JSONValue.Str("a")))),
    )
    private val data = LeafData(FakeRuntime(listOf(member, expense), all), all)

    @Test
    fun viewRecordsComposeComputedAndKeepHidden() {
        val r = data.viewRecords("member").single()
        assertEquals(JSONValue.Num(2000.0), r["balance"])
        assertEquals(JSONValue.Str("x"), r["memo"])
    }

    @Test
    fun visibleRecordsComposeComputedAndDropHidden() {
        val r = data.visibleRecords("member").single()
        assertEquals(JSONValue.Num(2000.0), r["balance"])
        assertEquals(setOf("name", "paid", "share", "balance"), r.data.keys)
    }

    @Test
    fun displayResolvesRefTitle() {
        val e = data.records("expense").single()
        assertEquals("A", data.display(e["who"], expense.field("who")))
        assertEquals("", data.display(JSONValue.Str(""), expense.field("who")))
    }
}
