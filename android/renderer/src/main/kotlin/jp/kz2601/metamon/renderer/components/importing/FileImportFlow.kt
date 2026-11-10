package jp.kz2601.metamon.renderer.components.importing

import jp.kz2601.metamon.dsl.Accept
import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.logic.CsvColumnMapping
import jp.kz2601.metamon.dsl.logic.CsvReader
import jp.kz2601.metamon.dsl.logic.ImportPlan
import jp.kz2601.metamon.dsl.logic.ImportRowFailure
import jp.kz2601.metamon.dsl.logic.RecordImport
import jp.kz2601.metamon.dsl.records.Entity
import jp.kz2601.metamon.dsl.records.Field
import jp.kz2601.metamon.dsl.records.JSONValue
import jp.kz2601.metamon.renderer.DSLRuntime
import jp.kz2601.metamon.renderer.form.FormChoices

/** 読むファイルの種類。Android でまだ読めないのは録音だけ([FileImportFlow.unavailableKinds])。 */
enum class ImportKind { CSV, TEXT, PDF, AUDIO }

/** 確認の文の材料(文言は [FileImportCopy])。 */
sealed interface ImportMessage {
    data class Csv(
        val entityName: String,
        val count: Int,
        /** 使う列(見出し → field 名) */
        val used: List<Pair<String, String>>,
        val unused: List<String>,
        val failures: List<ImportRowFailure>,
        val overLimit: Int,
    ) : ImportMessage

    data class Body(val fileName: String, val entityName: String, val characters: Int, val truncated: Boolean) : ImportMessage
}

/** 取り込めない理由(文言は [FileImportCopy])。 */
sealed interface ImportProblem {
    data object Unsupported : ImportProblem
    data object CannotOpen : ImportProblem
    data class TooLarge(val megabytes: Int) : ImportProblem
    data object Encoding : ImportProblem
    data class NoMatchingColumns(val entityName: String, val fieldNames: List<String>) : ImportProblem
    /** 当たる列はあるが、足せる行が 1 つも無い(読めない行・上限) */
    data class NothingToAdd(val summary: ImportMessage.Csv) : ImportProblem
    data object EmptyText : ImportProblem
    /** PDF から文字を 1 つも取り出せない(壊れている・暗号化・文字認識もできない) */
    data object PdfUnreadable : ImportProblem
    data object Full : ImportProblem
    /** 録音: この端末ではまだ読めない */
    data object KindUnavailable : ImportProblem
}

/** 選んだファイルを読んだ結果: 確かめてから足すもの、または取り込めない理由。 */
sealed interface ImportStep {
    data class Confirm(val fileName: String, val rows: List<Map<String, JSONValue>>, val message: ImportMessage) : ImportStep
    data class Problem(val problem: ImportProblem) : ImportStep
}

/**
 * `fileImport` の端末内の流れ(iOS の FileImportLeaf.swift の read / planCSV / planBody / add)。計算は :dsl の [RecordImport]。
 * ファイルは読むだけで、送らない・コピーしない・残さない。
 */
object FileImportFlow {
    /** Android ではまだ読めない種類(録音の文字起こし・#1180)。 */
    val unavailableKinds = setOf(Accept.AUDIO)

    private val csvMimes = listOf("text/csv", "text/comma-separated-values", "application/csv")
    private val textExtensions = setOf("txt", "text", "md", "markdown")

    /** OpenDocument に渡す MIME。CSV は「text/plain」「Excel」と名乗る保存先が多いのでそれも出す(読む前に [kind] で弾く)。 */
    fun mimeTypes(accept: List<Accept>): List<String> {
        val out = linkedSetOf<String>()
        if (Accept.CSV in accept) out += csvMimes + listOf("application/vnd.ms-excel", "text/plain")
        if (Accept.TEXT in accept) out += listOf("text/plain", "text/markdown")
        if (Accept.PDF in accept) out += "application/pdf"
        return out.toList()
    }

    /** 選ばれたファイルの種類(`accept` に無い・読めないものは null)。CSV は text/plain とも名乗るので先に見る。 */
    fun kind(fileName: String, mimeType: String?, accept: List<Accept>): ImportKind? {
        val ext = fileName.substringAfterLast('.', "").lowercase()
        val mime = mimeType?.lowercase()?.substringBefore(';')?.trim()
        if (Accept.CSV in accept && (ext == "csv" || mime in csvMimes)) return ImportKind.CSV
        if (Accept.PDF in accept && (ext == "pdf" || mime == "application/pdf")) return ImportKind.PDF
        if (Accept.TEXT in accept && (ext in textExtensions || mime == "text/plain" || mime == "text/markdown")) return ImportKind.TEXT
        if (Accept.AUDIO in accept && mime?.startsWith("audio/") == true) return ImportKind.AUDIO
        return null
    }

    /** 読むバイト数の上限(超えたら読まずに断る)。 */
    fun byteLimit(kind: ImportKind): Int = if (kind == ImportKind.PDF) RecordImport.MAX_PDF_BYTES else RecordImport.MAX_TEXT_BYTES

    /** 使える部品か: entity があり、public でも single でもなく、この端末で読める種類が 1 つはある。 */
    fun usable(c: Component, entity: Entity?): Boolean =
        entity != null && entity.scope != "public" && !entity.single && mimeTypes(c.accept).isNotEmpty()

    /** 読んだバイト列 → 確認する中身(または理由)。`existing` は今の件数。`pdfText` は PDF の本文の取り出し(端末では [PdfTextReader])。 */
    fun prepare(
        bytes: ByteArray,
        kind: ImportKind,
        fileName: String,
        c: Component,
        entity: Entity,
        existing: Int,
        pdfText: (ByteArray) -> String? = { null },
        refId: (Field, String) -> String?,
    ): ImportStep {
        if (bytes.size > byteLimit(kind)) return ImportStep.Problem(ImportProblem.TooLarge(byteLimit(kind) / 1_000_000))
        return when (kind) {
            ImportKind.CSV -> planCsv(bytes, fileName, c, entity, existing, refId)
            ImportKind.TEXT -> {
                val text = RecordImport.plainText(bytes)
                if (text.isNullOrBlank()) ImportStep.Problem(ImportProblem.EmptyText) else planText(text, fileName, c, entity, existing)
            }
            ImportKind.PDF -> {
                val text = pdfText(bytes)
                if (text.isNullOrBlank()) ImportStep.Problem(ImportProblem.PdfUnreadable) else planText(text, fileName, c, entity, existing)
            }
            ImportKind.AUDIO -> ImportStep.Problem(ImportProblem.KindUnavailable)
        }
    }

    private fun planCsv(
        bytes: ByteArray,
        fileName: String,
        c: Component,
        entity: Entity,
        existing: Int,
        refId: (Field, String) -> String?,
    ): ImportStep {
        val text = CsvReader.decode(bytes) ?: return ImportStep.Problem(ImportProblem.Encoding)
        val map = c.map.map { CsvColumnMapping(it.column, it.field) }
        val plan = RecordImport.plan(CsvReader.parse(text), entity, map, existing, refId = refId)
        if (plan.matched.isEmpty()) {
            return ImportStep.Problem(ImportProblem.NoMatchingColumns(entity.name, entity.storedFields.map { it.name }))
        }
        val summary = summary(plan, entity)
        if (plan.rows.isEmpty()) return ImportStep.Problem(ImportProblem.NothingToAdd(summary))
        return ImportStep.Confirm(fileName, plan.rows, summary)
    }

    /** 本文を `field` に入れた 1 件。 */
    fun planText(text: String, fileName: String, c: Component, entity: Entity, existing: Int): ImportStep {
        val field = c.field
        val made = if (existing < RecordImport.MAX_RECORDS_PER_ENTITY && field != null) {
            RecordImport.bodyRecord(text, fileName, entity, field)
        } else {
            null
        }
        if (made == null) return ImportStep.Problem(ImportProblem.Full)
        val body = (made.data[field] as? JSONValue.Str)?.value.orEmpty()
        val chars = body.codePointCount(0, body.length)
        return ImportStep.Confirm(fileName, listOf(made.data), ImportMessage.Body(fileName, entity.name, chars, made.truncated))
    }

    private fun summary(plan: ImportPlan, entity: Entity) =
        ImportMessage.Csv(entity.name, plan.rows.size, plan.matched, plan.unused, plan.failures, plan.overLimit)

    /** ref / refs の名前 → 参照先の記録 id(見出しが同じもの)。 */
    fun refId(runtime: DSLRuntime, field: Field, name: String): String? {
        val target = field.refEntity ?: return null
        return FormChoices.records(runtime, target).firstOrNull { FormChoices.refTitle(it, target, runtime) == name }?.id
    }

    /** 1 件ずつ足す。`progress(足せた数)`。足せた件数を返す(途中で失敗しても残りは続ける・iOS と同じ)。 */
    suspend fun add(host: DSLRuntime,entity: String, rows: List<Map<String, JSONValue>>, progress: (Int) -> Unit = {}): Int {
        var added = 0
        for (data in rows) {
            if (host.addRecord(entity, data)) added += 1
            progress(added)
        }
        return added
    }
}
