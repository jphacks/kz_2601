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
import java.io.ByteArrayOutputStream
import java.io.InputStream

/** 読むファイルの種類。録音は文字起こしの口([AudioFileTranscriptionHost])がある時だけ読む。 */
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

    /** [note] は録音の長さなど、確認の文の最後に足す一言 */
    data class Body(
        val fileName: String,
        val entityName: String,
        val characters: Int,
        val truncated: Boolean,
        val note: String? = null,
    ) : ImportMessage
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
    /** 録音: この画面・端末では文字にできない */
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
    private val csvMimes = listOf("text/csv", "text/comma-separated-values", "application/csv")
    private val textExtensions = setOf("txt", "text", "md", "markdown")

    /**
     * OpenDocument に渡す MIME。CSV は「text/plain」「Excel」と名乗る保存先が多いのでそれも出す(読む前に [kind] で弾く)。
     * 録音は [audio](文字起こしの口がある)の時だけ。
     */
    fun mimeTypes(accept: List<Accept>, audio: Boolean = false): List<String> {
        val out = linkedSetOf<String>()
        if (Accept.CSV in accept) out += csvMimes + listOf("application/vnd.ms-excel", "text/plain")
        if (Accept.TEXT in accept) out += listOf("text/plain", "text/markdown")
        if (Accept.PDF in accept) out += "application/pdf"
        if (audio && Accept.AUDIO in accept) out += "audio/*"
        return out.toList()
    }

    /** 選ばれたファイルの種類(`accept` に無い・読めないものは null)。CSV は text/plain とも名乗るので先に見る。 */
    fun kind(fileName: String, mimeType: String?, accept: List<Accept>): ImportKind? {
        val ext = fileName.substringAfterLast('.', "").lowercase()
        val mime = mimeType?.lowercase()?.substringBefore(';')?.trim()
        if (Accept.CSV in accept && (ext == "csv" || mime in csvMimes)) return ImportKind.CSV
        if (Accept.PDF in accept && (ext == "pdf" || mime == "application/pdf")) return ImportKind.PDF
        if (Accept.TEXT in accept && (ext in textExtensions || mime == "text/plain" || mime == "text/markdown")) return ImportKind.TEXT
        if (Accept.AUDIO in accept && AudioFileImport.isAudio(fileName, mime)) return ImportKind.AUDIO
        return null
    }

    /** 読むバイト数の上限(超えたら読まずに断る)。 */
    fun byteLimit(kind: ImportKind): Int = if (kind == ImportKind.PDF) RecordImport.MAX_PDF_BYTES else RecordImport.MAX_TEXT_BYTES

    /** 先頭から最大 [max] バイトを読む(`InputStream.readNBytes(Int)` は API 33 からなので minSdk 31 では自前)。 */
    fun readUpTo(input: InputStream, max: Int): ByteArray {
        val out = ByteArrayOutputStream(minOf(max, 8192))
        val buf = ByteArray(8192)
        var left = max
        while (left > 0) {
            val n = input.read(buf, 0, minOf(buf.size, left))
            if (n < 0) break
            out.write(buf, 0, n)
            left -= n
        }
        return out.toByteArray()
    }

    /** 使える部品か: entity があり、public でも single でもなく、この端末で読める種類が 1 つはある。 */
    fun usable(c: Component, entity: Entity?, audio: Boolean = false): Boolean =
        entity != null && entity.scope != "public" && !entity.single && mimeTypes(c.accept, audio).isNotEmpty()

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
            // 録音はバイト列でなく [AudioFileTranscriptionHost] で読む(部品が先に分ける)
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

    /** 本文を `field` に入れた 1 件。[note] は確認の文の最後に足す一言(録音の長さ)。 */
    fun planText(text: String, fileName: String, c: Component, entity: Entity, existing: Int, note: String? = null): ImportStep {
        val field = c.field
        val made = if (existing < RecordImport.MAX_RECORDS_PER_ENTITY && field != null) {
            RecordImport.bodyRecord(text, fileName, entity, field)
        } else {
            null
        }
        if (made == null) return ImportStep.Problem(ImportProblem.Full)
        val body = (made.data[field] as? JSONValue.Str)?.value.orEmpty()
        val chars = body.codePointCount(0, body.length)
        return ImportStep.Confirm(fileName, listOf(made.data), ImportMessage.Body(fileName, entity.name, chars, made.truncated, note))
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
