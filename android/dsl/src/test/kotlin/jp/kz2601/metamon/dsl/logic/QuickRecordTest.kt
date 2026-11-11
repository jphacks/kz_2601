package jp.kz2601.metamon.dsl.logic

import jp.kz2601.metamon.dsl.logic.QuickRecord.Parsed
import jp.kz2601.metamon.dsl.records.Entity
import jp.kz2601.metamon.dsl.records.Fixtures
import jp.kz2601.metamon.dsl.records.Fixtures.str
import jp.kz2601.metamon.dsl.records.JSONValue
import jp.kz2601.metamon.dsl.records.Record
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonObject
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import java.util.Locale

/** iOS の QuickRecordTests: ショートカット「記録を追加」(#80)のパラメータ解決。 */
class QuickRecordTest {
    private fun num(n: Int) = JSONValue.Num(n.toDouble())
    private fun str(s: String) = JSONValue.Str(s)

    @Test
    fun parse() {
        assertEquals(Parsed("ランチ", 800.0, "円"), QuickRecord.parse("ランチ 800円"))
        assertEquals(Parsed("ランチ", 800.0, "円"), QuickRecord.parse("ランチ８００円"))
        assertEquals(Parsed("ランチ", 1200.0, "円"), QuickRecord.parse("ランチを1,200円"))
        assertEquals(Parsed("コンビニ", 980.0, "円"), QuickRecord.parse("￥980 コンビニ"))
        assertEquals(Parsed("コンビニ", 980.0, "円"), QuickRecord.parse("¥980 コンビニ"))
        assertEquals(Parsed("体重", 62.5, "kg"), QuickRecord.parse("体重 62.5kg"))
        assertEquals(Parsed("スクワット", 20.0, "回"), QuickRecord.parse("スクワット20回"))
        assertEquals(Parsed("ランチ", 800.0, "円"), QuickRecord.parse("800円のランチ"))
        assertEquals(Parsed("散歩した", null), QuickRecord.parse("散歩した"))
        assertEquals(Parsed(null, null), QuickRecord.parse("  "))
    }

    private fun kakeibo() = Fixtures.load("kakeibo").entity("expense")!!

    private fun build(entity: Entity, text: String?, number: Double? = null, refChoices: Map<String, List<QuickRecord.RefChoice>> = emptyMap()) =
        QuickRecord.build(entity, text, number, refChoices, Locale.JAPAN)

    @Test
    fun kakeiboDraft() {
        val entity = kakeibo()
        assertEquals("memo", QuickRecord.titleField(entity)?.id)
        assertEquals("amount", QuickRecord.numberField(entity)?.id)
        val draft = build(entity, "ランチ 800円")
        assertEquals(str("ランチ"), draft.data["memo"])
        assertEquals(num(800), draft.data["amount"])
        assertEquals(str("食費"), draft.data["category"])
        assertEquals(10, draft.data["date"]?.stringValue?.length)
        assertFalse("receipt" in draft.data)
        assertEquals(emptyList<String>(), draft.missing)
        assertEquals("ランチ ¥800", draft.summary)
    }

    @Test
    fun enumMatchedFromText() {
        val entity = kakeibo()
        val option = entity.field("category")!!.options!!.last()
        val draft = build(entity, "$option 300円")
        assertEquals(option, draft.data["category"]?.stringValue)
        assertEquals(num(300), draft.data["amount"])
    }

    @Test
    fun explicitNumberWinsAndMissingRequired() {
        val entity = kakeibo()
        assertEquals(num(650), build(entity, "ランチ 800円", 650.0).data["amount"])
        assertEquals(listOf(entity.field("amount")!!.name), build(entity, "ランチ").missing)
    }

    @Test
    fun trainingEnumTitle() {
        val entity = Fixtures.load("training").entity("workout")!!
        assertEquals("exercise", QuickRecord.titleField(entity)?.id)
        assertEquals("reps", QuickRecord.numberField(entity)?.id)
        val option = entity.field("exercise")!!.options!!.first()
        val draft = build(entity, "$option 20回")
        assertEquals(option, draft.data["exercise"]?.stringValue)
        assertEquals(num(20), draft.data["reps"])
        assertEquals(emptyList<String>(), draft.missing)
    }

    private fun formEntityIds(root: JsonObject): Set<String> =
        (root["screens"] as JsonArray).mapNotNull { (it as JsonObject).str("entity") }.toSet()

    @Test
    fun recordableEntitiesPreferFormScreens() {
        val k = Fixtures.load("kakeibo")
        assertEquals("expense", QuickRecord.recordableEntities(k.entities, formEntityIds(k.root)).first().id)
        val initial = Fixtures.load("initial")
        assertTrue(QuickRecord.recordableEntities(initial.entities, formEntityIds(initial.root)).isEmpty())
    }

    @Test
    fun numberFieldByUnit() {
        val entity = Entity.parse(
            """{"id":"workout","name":"筋トレ","titleField":"exercise","fields":[{"id":"exercise","name":"種目","type":"string","required":true},{"id":"weight","name":"重量","type":"number","unit":"kg"},{"id":"reps","name":"回数","type":"integer","unit":"回"},{"id":"sets","name":"セット数","type":"integer","defaultValue":3}]}""",
        )
        assertEquals(num(20), build(entity, "スクワット 20回").data["reps"])
        assertEquals(num(60), build(entity, "ベンチ 60kg").data["weight"])
        assertEquals(num(60), build(entity, "ベンチ 60").data["weight"])
        assertEquals(num(3), build(entity, "スクワット 20回").data["sets"])
    }

    // MARK: - ref

    private fun refEntities(categoryDefault: String? = null): List<Entity> {
        val def = categoryDefault?.let { ""","defaultValue":"$it"""" } ?: ""
        return listOf(
            Entity.parse(
                """{"id":"expense","name":"支出","titleField":"memo","fields":[
                  {"id":"memo","name":"メモ","type":"string"},
                  {"id":"amount","name":"金額","type":"currency","required":true},
                  {"id":"category","name":"カテゴリ","type":"ref","refEntity":"categories"$def},
                  {"id":"payment","name":"支払い","type":"enum","options":["現金","カード"]}]}""",
            ),
            Entity.parse("""{"id":"categories","name":"カテゴリ","titleField":"name","fields":[{"id":"name","name":"名前","type":"string"}]}"""),
        )
    }

    private val categories = listOf(
        Record("c1", mapOf("name" to str("食費"))),
        Record("c2", mapOf("name" to str("食費(外食)"))),
        Record("c3", mapOf("name" to str("交通費"))),
    )

    private fun refBuild(text: String, categoryDefault: String? = null, records: List<Record>? = null): QuickRecord.Draft {
        val entities = refEntities(categoryDefault)
        val entity = entities[0]
        val choices = QuickRecord.refChoices(entity, entities, mapOf("categories" to (records ?: categories)), Locale.JAPAN)
        return build(entity, text, refChoices = choices)
    }

    @Test
    fun refShortTitleNeedsWholeWord() {
        val short = categories + listOf(Record("c4", mapOf("name" to str("車"))), Record("c5", mapOf("name" to str("1"))))
        assertNull(refBuild("電車 300円", records = short).data["category"])
        assertNull(refBuild("ランチ 1200円", records = short).data["category"])
        assertEquals(str("c4"), refBuild("車 ガソリン 3000円", records = short).data["category"])
        assertEquals(str("c1"), refBuild("食費 電車 300円", records = short).data["category"])
    }

    @Test
    fun refMatchIsReported() {
        assertEquals(listOf("カテゴリ: 食費"), refBuild("食費 コンビニ 480円").refs)
        assertEquals(emptyList<String>(), refBuild("コンビニ 480円").refs)
        assertEquals(emptyList<String>(), refBuild("コンビニ 480円", categoryDefault = "c3").refs)
    }

    @Test
    fun refMatchedByRecordTitle() {
        val draft = refBuild("食費 コンビニ 480円")
        assertEquals(str("c1"), draft.data["category"])
        assertEquals(num(480), draft.data["amount"])
        assertEquals(str("食費 コンビニ"), draft.data["memo"])
        assertEquals(emptyList<String>(), draft.missing)
    }

    @Test
    fun refLongestTitleWins() {
        assertEquals(str("c2"), refBuild("食費(外食) ラーメン 900円").data["category"])
    }

    @Test
    fun refNoMatchLeavesEmptyOrDefault() {
        assertNull(refBuild("コンビニ 480円").data["category"])
        assertEquals(str("c3"), refBuild("コンビニ 480円", categoryDefault = "c3").data["category"])
        assertNull(build(refEntities()[0], "食費 480円").data["category"])
    }

    @Test
    fun enumAndRefBothMatched() {
        val draft = refBuild("交通費 カード 300円")
        assertEquals(str("c3"), draft.data["category"])
        assertEquals(str("カード"), draft.data["payment"])
        assertEquals(num(300), draft.data["amount"])
    }
}
