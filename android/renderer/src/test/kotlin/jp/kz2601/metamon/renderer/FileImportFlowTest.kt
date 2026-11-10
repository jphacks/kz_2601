package jp.kz2601.metamon.renderer

import jp.kz2601.metamon.dsl.Accept
import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.ComponentType
import jp.kz2601.metamon.dsl.decode
import jp.kz2601.metamon.dsl.logic.ImportRowFailure
import jp.kz2601.metamon.dsl.logic.RecordImport
import jp.kz2601.metamon.dsl.records.Entity
import jp.kz2601.metamon.dsl.records.JSONValue
import jp.kz2601.metamon.dsl.records.Record
import jp.kz2601.metamon.dsl.records.entityModels
import jp.kz2601.metamon.dsl.screen
import jp.kz2601.metamon.renderer.components.importing.FileImportFlow
import jp.kz2601.metamon.renderer.components.importing.ImportKind
import jp.kz2601.metamon.renderer.components.importing.ImportMessage
import jp.kz2601.metamon.renderer.components.importing.ImportProblem
import jp.kz2601.metamon.renderer.components.importing.ImportStep
import jp.kz2601.metamon.renderer.registry.ComponentRegistry
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.runBlocking
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/** fileImport(#865): 選んだファイル → 計画 → 記録の追加、上限、失敗の知らせ(iOS の FileImportLeaf の read / planCSV / planBody / add)。 */
class FileImportFlowTest {
    private val dsl = AppDSL.decode(
        """
        {"schemaVersion":1,"app":{"name":"t","voiceEntry":"none"},"initialScreen":"s",
         "entities":[
          {"id":"part","name":"パート","titleField":"name","fields":[{"id":"name","name":"名前","type":"string"}]},
          {"id":"member","name":"部員","titleField":"name","fields":[
            {"id":"name","name":"名前","type":"string","required":true},
            {"id":"fee","name":"部費","type":"currency","unit":"円"},
            {"id":"part","name":"パート","type":"ref","refEntity":"part"}]},
          {"id":"note","name":"メモ","titleField":"title","fields":[
            {"id":"title","name":"題","type":"string"},
            {"id":"body","name":"本文","type":"text"}]},
          {"id":"board","name":"掲示","scope":"public","fields":[{"id":"t","name":"t","type":"string"}]}
         ],
         "screens":[{"id":"s","title":"s","components":[
          {"type":"fileImport","entity":"member","accept":["csv"],"map":[{"column":"氏名","field":"name"}]},
          {"type":"fileImport","entity":"note","accept":["text","pdf","audio"],"field":"body"},
          {"type":"fileImport","entity":"board","accept":["csv"]},
          {"type":"fileImport","entity":"member","accept":["audio"]}
         ]}]}
        """.trimIndent(),
    )
    private val entities = dsl.entityModels()
    private val components: List<Component> = dsl.screen("s")!!.components
    private val member: Entity = entities.first { it.id == "member" }
    private val note: Entity = entities.first { it.id == "note" }

    private class FakeRuntime(
        override val entityModels: List<Entity>,
        records: Map<String, List<Record>> = emptyMap(),
        private val failIds: Set<String> = emptySet(),
    ) : DSLRuntime {
        val added = mutableListOf<Pair<String, Map<String, JSONValue>>>()
        val loaded = mutableListOf<Set<String>>()
        override val recordsFlow: StateFlow<Map<String, List<Record>>> = MutableStateFlow(records)
        override fun onVoiceInput(componentId: String?) = Unit
        override suspend fun addRecord(entity: String, data: Map<String, JSONValue>): Boolean {
            if ((data["name"] as? JSONValue.Str)?.value in failIds) return false
            added += entity to data
            return true
        }
        override suspend fun loadRecords(entities: Set<String>) {
            loaded += entities
        }
    }

    private val parts = listOf(Record("p1", mapOf("name" to JSONValue.Str("前衛"))), Record("p2", mapOf("name" to JSONValue.Str("後衛"))))

    private fun prepare(text: String, rt: FakeRuntime, existing: Int = 0, bytes: ByteArray = text.toByteArray()) =
        FileImportFlow.prepare(bytes, ImportKind.CSV, "members.csv", components[0], member, existing) { f, n -> FileImportFlow.refId(rt, f, n) }

    @Test
    fun registryHasFileImport() {
        assertNotNull(ComponentRegistry.standard.builder(ComponentType.FILE_IMPORT))
    }

    @Test
    fun pickerOffersOnlyKindsThisDeviceCanRead() {
        assertTrue("text/csv" in FileImportFlow.mimeTypes(listOf(Accept.CSV)))
        assertEquals(listOf("text/plain", "text/markdown", "application/pdf"), FileImportFlow.mimeTypes(listOf(Accept.TEXT, Accept.PDF, Accept.AUDIO)))
        // 録音はまだ読めない(#1180)。PDF は読める(#1181)
        assertTrue(FileImportFlow.mimeTypes(listOf(Accept.AUDIO)).isEmpty())
        assertEquals(setOf(Accept.AUDIO), FileImportFlow.unavailableKinds)
        // CSV は text/plain を名乗っても拡張子で CSV。accept に無い種類は null。
        assertEquals(ImportKind.CSV, FileImportFlow.kind("a.CSV", "text/plain", listOf(Accept.CSV, Accept.TEXT)))
        assertEquals(ImportKind.CSV, FileImportFlow.kind("download", "text/csv; charset=utf-8", listOf(Accept.CSV)))
        assertEquals(ImportKind.TEXT, FileImportFlow.kind("memo.txt", null, listOf(Accept.CSV, Accept.TEXT)))
        assertNull(FileImportFlow.kind("memo.txt", "text/plain", listOf(Accept.CSV)))
        assertNull(FileImportFlow.kind("a.csv", "text/csv", listOf(Accept.TEXT)))
        assertEquals(ImportKind.AUDIO, FileImportFlow.kind("rec.m4a", "audio/mp4", listOf(Accept.AUDIO)))
        assertEquals(ImportKind.PDF, FileImportFlow.kind("a.pdf", null, listOf(Accept.PDF)))
    }

    @Test
    fun usableOnlyForPrivateOrSharedEntityWithReadableKind() {
        assertTrue(FileImportFlow.usable(components[0], member))
        assertFalse(FileImportFlow.usable(components[2], entities.first { it.id == "board" }))
        assertFalse(FileImportFlow.usable(components[3], member))
        assertFalse(FileImportFlow.usable(components[0], null))
    }

    @Test
    fun csvBecomesRecordsWithMapAndRefsThenAdded() = runBlocking {
        val rt = FakeRuntime(entities, mapOf("part" to parts))
        val step = prepare("氏名,部費,パート,メモ\r\n山田,\"¥1,200\",前衛,x\r\n佐藤,800 円,後衛,y\r\n", rt)
        step as ImportStep.Confirm
        val m = step.message as ImportMessage.Csv
        assertEquals(2, m.count)
        assertEquals(listOf("氏名" to "名前", "部費" to "部費", "パート" to "パート"), m.used)
        assertEquals(listOf("メモ"), m.unused)
        assertEquals(JSONValue.Str("p1"), step.rows[0]["part"])
        assertEquals(JSONValue.Num(1200.0), step.rows[0]["fee"])

        var last = 0
        val added = FileImportFlow.add(rt, "member", step.rows) { last = it }
        assertEquals(2, added)
        assertEquals(2, last)
        assertEquals(listOf("山田", "佐藤"), rt.added.map { (it.second["name"] as JSONValue.Str).value })
        assertTrue(rt.added.all { it.first == "member" })
    }

    @Test
    fun shiftJisCsvIsRead() {
        val rt = FakeRuntime(entities)
        val bytes = "名前,部費\n田中,500\n".toByteArray(charset("windows-31j"))
        val step = prepare("", rt, bytes = bytes) as ImportStep.Confirm
        assertEquals(JSONValue.Str("田中"), step.rows[0]["name"])
    }

    @Test
    fun limitsAndUnreadableRowsAreReported() {
        val rt = FakeRuntime(entities)
        val step = prepare("名前,部費\n,1\nA,abc\nB,1\nC,2\n", rt, existing = RecordImport.MAX_RECORDS_PER_ENTITY - 1) as ImportStep.Confirm
        val m = step.message as ImportMessage.Csv
        assertEquals(1, step.rows.size)
        assertEquals(1, m.overLimit)
        assertEquals(listOf(ImportRowFailure.Kind.REQUIRED, ImportRowFailure.Kind.INVALID), m.failures.map { it.kind })
        assertEquals(listOf(2, 3), m.failures.map { it.line })
    }

    @Test
    fun problemsAreReported() {
        val rt = FakeRuntime(entities)
        val none = prepare("foo,bar\n1,2\n", rt) as ImportStep.Problem
        assertEquals(ImportProblem.NoMatchingColumns("部員", listOf("名前", "部費", "パート")), none.problem)

        val full = prepare("名前\nA\n", rt, existing = RecordImport.MAX_RECORDS_PER_ENTITY) as ImportStep.Problem
        assertTrue(full.problem is ImportProblem.NothingToAdd)
        assertEquals(1, (full.problem as ImportProblem.NothingToAdd).summary.overLimit)

        val big = prepare("", rt, bytes = ByteArray(RecordImport.MAX_TEXT_BYTES + 1)) as ImportStep.Problem
        assertEquals(ImportProblem.TooLarge(5), big.problem)

        val audio = FileImportFlow.prepare(ByteArray(4), ImportKind.AUDIO, "rec.m4a", components[1], note, 0) { _, _ -> null }
        assertEquals(ImportStep.Problem(ImportProblem.KindUnavailable), audio)
    }

    @Test
    fun textFileBecomesOneBodyRecord() {
        val step = FileImportFlow.prepare("こんにちは\r\n世界\n".toByteArray(), ImportKind.TEXT, "日記.txt", components[1], note, 0) { _, _ -> null }
        step as ImportStep.Confirm
        assertEquals(JSONValue.Str("こんにちは\n世界"), step.rows.single()["body"])
        assertEquals(JSONValue.Str("日記"), step.rows.single()["title"])
        assertEquals(ImportMessage.Body("日記.txt", "メモ", 8, false), step.message)

        val long = FileImportFlow.planText("あ".repeat(20001), "a.txt", components[1], note, 0) as ImportStep.Confirm
        assertTrue((long.message as ImportMessage.Body).truncated)
        assertEquals(20000, (long.message as ImportMessage.Body).characters)

        val empty = FileImportFlow.prepare("  \n".toByteArray(), ImportKind.TEXT, "a.txt", components[1], note, 0) { _, _ -> null }
        assertEquals(ImportStep.Problem(ImportProblem.EmptyText), empty)
        val full = FileImportFlow.planText("x", "a.txt", components[1], note, RecordImport.MAX_RECORDS_PER_ENTITY)
        assertEquals(ImportStep.Problem(ImportProblem.Full), full)
    }

    @Test
    fun pdfBodyBecomesOneRecordOrUnreadable() {
        val ok = FileImportFlow.prepare(ByteArray(4), ImportKind.PDF, "議事録.pdf", components[1], note, 0, pdfText = { "1 ページ目\n\n2 ページ目" }) { _, _ -> null }
        ok as ImportStep.Confirm
        assertEquals(JSONValue.Str("1 ページ目\n\n2 ページ目"), ok.rows.single()["body"])
        assertEquals(JSONValue.Str("議事録"), ok.rows.single()["title"])

        for (text in listOf(null, " \n")) {
            val bad = FileImportFlow.prepare(ByteArray(4), ImportKind.PDF, "a.pdf", components[1], note, 0, pdfText = { text }) { _, _ -> null }
            assertEquals(ImportStep.Problem(ImportProblem.PdfUnreadable), bad)
        }
        var called = false
        val big = FileImportFlow.prepare(ByteArray(RecordImport.MAX_PDF_BYTES + 1), ImportKind.PDF, "a.pdf", components[1], note, 0, pdfText = { called = true; "x" }) { _, _ -> null }
        assertEquals(ImportStep.Problem(ImportProblem.TooLarge(30)), big)
        assertFalse(called)
    }

    @Test
    fun addCountsFailuresAndKeepsGoing() = runBlocking {
        val rt = FakeRuntime(entities, failIds = setOf("B"))
        val rows = listOf("A", "B", "C").map { mapOf("name" to JSONValue.Str(it)) }
        val seen = mutableListOf<Int>()
        assertEquals(2, FileImportFlow.add(rt, "member", rows) { seen += it })
        assertEquals(listOf(1, 1, 2), seen)
        assertEquals(listOf("A", "C"), rt.added.map { (it.second["name"] as JSONValue.Str).value })
    }
}
