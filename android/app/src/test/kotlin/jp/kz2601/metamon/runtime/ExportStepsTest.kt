package jp.kz2601.metamon.runtime

import jp.kz2601.metamon.dsl.Action
import jp.kz2601.metamon.dsl.ActionFormat
import jp.kz2601.metamon.dsl.ActionType
import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.Filter
import jp.kz2601.metamon.dsl.FilterOp
import jp.kz2601.metamon.dsl.decode
import jp.kz2601.metamon.dsl.logic.CsvWriter
import jp.kz2601.metamon.dsl.records.JSONValue
import jp.kz2601.metamon.renderer.ActionContext
import jp.kz2601.metamon.renderer.navigation.Route
import jp.kz2601.metamon.runtime.RuntimeFixture.runtime
import kotlinx.coroutines.test.runTest
import kotlinx.serialization.json.JsonPrimitive
import org.junit.Assert.assertArrayEquals
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.rules.TemporaryFolder

/** export(#911): 表は list と同じ経路・CSV は iOS の CSVWriter と同じ中身。ファイルは次の書き出しで消す。 */
class ExportStepsTest {
    @get:Rule
    val tmp = TemporaryFolder()

    private val dsl = AppDSL.decode(
        """
        {"schemaVersion":1,"app":{"name":"家計簿"},"initialScreen":"list",
         "entities":[{"id":"expense","name":"支出","fields":[
           {"id":"day","name":"日付","type":"date"},
           {"id":"item","name":"品目","type":"string"},
           {"id":"shop","name":"店","type":"string"}
         ]}],
         "screens":[{"id":"list","title":"一覧","components":[{"type":"list","entity":"expense"}]}]}
        """.trimIndent(),
    )

    private fun export(format: ActionFormat? = ActionFormat.CSV, entity: String? = "expense", filter: Filter? = null, fields: List<String> = emptyList()) =
        Action(type = ActionType.EXPORT, entity = entity, format = format, filter = filter, fields = fields)

    private suspend fun AppRuntime.seed() {
        createRecord("expense", mapOf("day" to JSONValue.Str("2026-11-02"), "item" to JSONValue.Str("=SUM(A1)"), "shop" to JSONValue.Str("A, B")))
        createRecord("expense", mapOf("day" to JSONValue.Str("2026-11-01"), "item" to JSONValue.Str("牛乳"), "shop" to JSONValue.Str("スーパー")))
    }

    @Test
    fun csvIsChronologicalAndEscaped() = runTest {
        val rt = runtime(dsl)
        rt.seed()
        val ready = ExportSteps.prepare(export(), rt) { listOf("$it 件") } as ExportSteps.Prepared.Ready
        assertEquals("支出", ready.table.title)
        assertEquals(listOf("2 件"), ready.table.notes)
        val (bytes, ext) = ExportSteps.encode(ready.table, ready.format) { error("csv は PDF を描かない") }
        assertEquals("csv", ext)
        val bom = CsvWriter.BOM
        assertArrayEquals("UTF-8 の BOM", bom, bytes.copyOfRange(0, 3))
        val text = String(bytes, 3, bytes.size - 3, Charsets.UTF_8)
        val lines = text.split("\r\n")
        assertEquals("日付,品目,店", lines[0])
        assertTrue("古い順: ${lines[1]}", lines[1].contains("牛乳"))
        assertTrue("式に見える先頭に ' ・カンマは囲む: ${lines[2]}", lines[2].endsWith(",'=SUM(A1),\"A, B\""))
    }

    @Test
    fun filterAndFieldsAreApplied() = runTest {
        val rt = runtime(dsl)
        rt.seed()
        val action = export(filter = Filter(field = "shop", op = FilterOp.EQ, value = JsonPrimitive("スーパー")), fields = listOf("item", "nope"))
        val ready = ExportSteps.prepare(action, rt) as ExportSteps.Prepared.Ready
        assertEquals(listOf("品目"), ready.table.headers)
        assertEquals(listOf(listOf("牛乳")), ready.table.rows)
    }

    @Test
    fun unknownEntityOrFormatIsAToast() = runTest {
        val rt = runtime(dsl)
        assertEquals(ExportSteps.Prepared.Invalid(ExportSteps.NO_ENTITY), ExportSteps.prepare(export(entity = "nope"), rt))
        assertEquals(ExportSteps.Prepared.Invalid(ExportSteps.NO_ENTITY), ExportSteps.prepare(export(entity = null), rt))
        assertEquals(ExportSteps.Prepared.Invalid(ExportSteps.FAILED), ExportSteps.prepare(export(format = null), rt))
    }

    @Test
    fun writeFileRemovesPreviousExports() {
        val first = ExportSteps.writeFile(tmp.root, "a.csv", byteArrayOf(1))
        val second = ExportSteps.writeFile(tmp.root, "a.csv", byteArrayOf(2))
        assertFalse("前の書き出しは残さない", first.exists())
        assertArrayEquals(byteArrayOf(2), second.readBytes())
        assertTrue(second.path.contains(ExportSteps.DIR))
        ExportSteps.removeAll(tmp.root)
        assertFalse(second.exists())
    }

    /** ホストまで: AppRuntime の export がホストに届く(未対応のトーストにならない)。 */
    @Test
    fun runtimeHandsExportToHost() = runTest {
        var called = 0
        val host = object : RuntimeHost {
            override suspend fun export(action: Action, context: ActionContext, runtime: AppRuntime) {
                called += 1
            }
        }
        val rt = runtime(dsl, host = host)
        rt.run(export(), ActionContext(Route("list")))
        assertEquals(1, called)
    }
}
