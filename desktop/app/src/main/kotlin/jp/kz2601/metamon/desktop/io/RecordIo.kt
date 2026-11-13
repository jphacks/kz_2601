package jp.kz2601.metamon.desktop.io

import jp.kz2601.metamon.desktop.MockStore
import jp.kz2601.metamon.desktop.Row
import jp.kz2601.metamon.dsl.ImportColumn
import jp.kz2601.metamon.dsl.logic.CsvColumnMapping
import jp.kz2601.metamon.dsl.logic.CsvReader
import jp.kz2601.metamon.dsl.logic.CsvWriter
import jp.kz2601.metamon.dsl.logic.ExportTable
import jp.kz2601.metamon.dsl.logic.ImportPlan
import jp.kz2601.metamon.dsl.logic.RecordExport
import jp.kz2601.metamon.dsl.logic.RecordImport
import jp.kz2601.metamon.dsl.records.Entity
import jp.kz2601.metamon.dsl.records.Field
import jp.kz2601.metamon.dsl.records.FieldCoding
import jp.kz2601.metamon.dsl.records.FieldType
import jp.kz2601.metamon.dsl.records.JSONValue
import jp.kz2601.metamon.dsl.records.Record
import java.time.Instant
import java.util.Locale

/**
 * 取り込み・書き出し・クリップボードの純粋な部分(画面に触らないので JVM で確かめる)。
 * 規則は :dsl の RecordImport / RecordExport / CsvReader / CsvWriter をそのまま使い、ここではモックの文字列の行との行き来だけを足す。
 */
object RecordIo {
    /** モックの行(文字列)→ 記録の値。型に合わない値は文字列のまま(表示は FieldCoding に任せる)。 */
    fun toJson(raw: String?, f: Field): JSONValue {
        if (raw.isNullOrEmpty()) return JSONValue.Null
        return when (f.type) {
            FieldType.INTEGER, FieldType.NUMBER, FieldType.CURRENCY -> raw.toDoubleOrNull()?.let { JSONValue.Num(it) } ?: JSONValue.Str(raw)
            FieldType.BOOLEAN -> JSONValue.Bool(raw == "true")
            FieldType.REFS -> JSONValue.Arr(raw.split(',').map { it.trim() }.filter { it.isNotEmpty() }.map { JSONValue.Str(it) })
            else -> JSONValue.Str(raw)
        }
    }

    /** 記録の値 → モックの行の文字列(フォームの入力欄と同じ形)。 */
    fun toStore(v: JSONValue): String? = when (v) {
        JSONValue.Null -> null
        is JSONValue.Arr -> v.items.joinToString(",") { it.displayString }
        else -> v.displayString
    }

    fun record(row: Row, entity: Entity): Record =
        Record(row.id, entity.storedFields.mapNotNull { f -> toJson(row.data[f.id], f).takeUnless { it.isNull }?.let { f.id to it } }.toMap())

    /** ref / refs の参照先の見出し(titleField の値)→ id。 */
    fun refId(store: MockStore, entities: List<Entity>): (Field, String) -> String? = { f, name ->
        val target = entities.firstOrNull { it.id == f.refEntity }
        val title = target?.displayTitleField
        if (target == null || title == null) null else store.list(target.id).firstOrNull { it.data[title] == name }?.id
    }

    /** 表示の文字列(UI と同じ書式・ref は参照先の見出し)。 */
    fun display(store: MockStore, entities: List<Entity>, locale: Locale = Locale.JAPAN, now: Instant = Instant.now()): (JSONValue, Field) -> String = { v, f ->
        fun refName(id: String): String {
            val target = entities.firstOrNull { it.id == f.refEntity } ?: return id
            val title = target.displayTitleField ?: return id
            return store.find(target.id, id)?.data?.get(title) ?: id
        }
        when (f.type) {
            FieldType.REF -> v.stringValue?.let(::refName) ?: ""
            FieldType.REFS -> v.arrayValue.orEmpty().mapNotNull { it.stringValue }.joinToString("、") { refName(it) }
            else -> FieldCoding.display(v, f, locale, now)
        }
    }

    /** `export` の表(古い順・hidden は RecordExport が落とす)。[ids] を渡すとその記録だけ・[ids] の並び(画面の並び)。 */
    fun exportTable(
        title: String,
        entity: Entity,
        store: MockStore,
        entities: List<Entity>,
        fields: List<String>?,
        notes: List<String> = emptyList(),
        locale: Locale = Locale.JAPAN,
        now: Instant = Instant.now(),
        ids: List<String>? = null,
    ): ExportTable {
        // ids があればその行だけ・その並び(一覧で選んだ行のコピー・引き出し #1766)。書式は全部の時と同じ
        val records = if (ids == null) {
            RecordExport.chronological(store.list(entity.id).map { record(it, entity) }, entity)
        } else {
            val byId = store.list(entity.id).associateBy { it.id }
            ids.distinct().mapNotNull { byId[it] }.map { record(it, entity) }
        }
        return RecordExport.table(title, entity, records, fields?.takeIf { it.isNotEmpty() }, currentUid = null, notes = notes, display = display(store, entities, locale, now))
    }

    fun csv(table: ExportTable): ByteArray = CsvWriter.data(table)

    fun mapping(map: List<ImportColumn>): List<CsvColumnMapping> = map.map { CsvColumnMapping(it.column, it.field) }

    /** CSV / TSV の表 → 取り込みの計画。 */
    fun plan(table: List<List<String>>, entity: Entity, map: List<ImportColumn>, store: MockStore, entities: List<Entity>, now: Instant = Instant.now()): ImportPlan =
        RecordImport.plan(table, entity, mapping(map), store.list(entity.id).size, now, refId(store, entities))

    /** CSV ファイルのバイト列 → 表。文字コードが読めなければ null。 */
    fun csvTable(bytes: ByteArray): List<List<String>>? = CsvReader.decode(bytes)?.let(CsvReader::parse)

    /** 貼り付けた文字 → 表。タブがあれば表計算の TSV、無ければ CSV。 */
    fun pastedTable(text: String): List<List<String>> = if ('\t' in text) Tsv.parse(text) else CsvReader.parse(text)

    /** 計画の行をモックに足す。 */
    fun apply(store: MockStore, entityId: String, rows: List<Map<String, JSONValue>>): MockStore =
        rows.fold(store) { s, data -> s.save(entityId, null, data.mapNotNull { (k, v) -> toStore(v)?.let { k to it } }.toMap()) }

    /** 確認のダイアログの文(iOS の取り込み確認と同じ情報: 足す件数・読めない行・上限・当たらない列)。 */
    fun summary(plan: ImportPlan): List<String> = buildList {
        add("${plan.rows.size} 件を追加します")
        if (plan.matched.isNotEmpty()) add("当てた列: " + plan.matched.joinToString("、") { (h, f) -> if (h == f) h else "$h → $f" })
        if (plan.unused.isNotEmpty()) add("使わない列: " + plan.unused.joinToString("、"))
        if (plan.failures.isNotEmpty()) {
            add("読めない行 ${plan.failures.size} 件(飛ばします)")
            plan.failures.take(5).forEach { add("・${it.line} 行目 ${it.fieldName}: ${failureText(it.kind)}${if (it.cell.isEmpty()) "" else "「${it.cell}」"}") }
        }
        if (plan.overLimit > 0) add("上限を超える ${plan.overLimit} 件は入りません")
    }

    private fun failureText(k: jp.kz2601.metamon.dsl.logic.ImportRowFailure.Kind): String = when (k) {
        jp.kz2601.metamon.dsl.logic.ImportRowFailure.Kind.INVALID -> "読めない値"
        jp.kz2601.metamon.dsl.logic.ImportRowFailure.Kind.REQUIRED -> "必須が空"
        jp.kz2601.metamon.dsl.logic.ImportRowFailure.Kind.TOO_LONG -> "長すぎる"
    }
}

/**
 * 表計算(Excel・LibreOffice)のクリップボードの形: タブ区切り・CRLF・タブ / 改行 / `"` を含むセルは `"` で囲む。
 * 式に見える先頭には CSV(:dsl の CsvWriter)と同じく `'` を付け、読む時に外す。
 */
object Tsv {
    fun escape(cell: String): String {
        val s = guard(cell)
        return if (s.none { it == '\t' || it == '\n' || it == '\r' || it == '"' }) s else "\"" + s.replace("\"", "\"\"") + "\""
    }

    /** CsvWriter.escape と同じ判定(`-500` / `-¥500` は数のまま)。 */
    fun guard(cell: String): String {
        if (cell.isEmpty()) return cell
        val first = cell.codePointAt(0)
        val nextIndex = Character.charCount(first)
        val next = if (nextIndex < cell.length) cell.codePointAt(nextIndex) else null
        return if (isFormulaLead(first, next)) "'$cell" else cell
    }

    /** [guard] が付けた `'` を外す(RecordImport と同じ規則)。 */
    fun unguard(cell: String): String =
        if (cell.length >= 2 && cell[0] == '\'' && FORMULA_GUARDED.indexOf(cell[1]) >= 0) cell.substring(1) else cell

    private const val FORMULA_GUARDED = "=+-@\t\r"

    internal fun isFormulaLead(c: Int, next: Int?): Boolean = when (c) {
        '='.code, '+'.code, '@'.code, '\t'.code, '\r'.code -> true
        '-'.code -> next != null && !(Character.getType(next) == Character.DECIMAL_DIGIT_NUMBER.toInt() || next == '¥'.code || next == '.'.code)
        else -> false
    }

    fun string(headers: List<String>?, rows: List<List<String>>): String =
        (listOfNotNull(headers) + rows).joinToString("\r\n") { r -> r.joinToString("\t") { escape(it) } } + "\r\n"

    fun string(table: ExportTable, includeHeaders: Boolean = true): String = string(table.headers.takeIf { includeHeaders }, table.rows)

    /** TSV を読む(CsvReader と同じ規則で区切りだけタブ)。空行は飛ばす。 */
    fun parse(text: String): List<List<String>> {
        val rows = mutableListOf<List<String>>()
        var row = mutableListOf<String>()
        val cell = StringBuilder()
        var quoted = false
        var i = if (text.startsWith("﻿")) 1 else 0
        fun endRow() {
            row.add(unguard(cell.toString()))
            cell.setLength(0)
            if (!(row.size == 1 && row[0].isEmpty())) rows.add(row)
            row = mutableListOf()
        }
        while (i < text.length) {
            val c = text[i++]
            val next = text.getOrNull(i)
            if (quoted) {
                if (c == '"') {
                    if (next == '"') {
                        cell.append('"')
                        i++
                    } else {
                        quoted = false
                    }
                } else {
                    cell.append(c)
                }
                continue
            }
            when {
                c == '"' && cell.isEmpty() -> quoted = true
                c == '\t' -> {
                    row.add(unguard(cell.toString()))
                    cell.setLength(0)
                }
                c == '\r' -> {
                    if (next == '\n') i++
                    endRow()
                }
                c == '\n' -> endRow()
                else -> cell.append(c)
            }
        }
        if (cell.isNotEmpty() || row.isNotEmpty()) endRow()
        return rows
    }
}

/** 落とした・選んだファイルの振り分け(拡張子で見る)。 */
enum class FileKind {
    CSV, TEXT, PDF, IMAGE, VIDEO, AUDIO, OTHER;

    companion object {
        fun of(name: String): FileKind = when (name.substringAfterLast('.', "").lowercase(Locale.ROOT)) {
            "csv", "tsv" -> CSV
            "txt", "text", "md", "markdown", "log" -> TEXT
            "pdf" -> PDF
            "png", "jpg", "jpeg", "gif", "bmp", "webp", "heic" -> IMAGE
            "mp4", "mov", "m4v", "webm", "avi", "mkv" -> VIDEO
            "m4a", "mp3", "wav", "aac", "ogg", "flac" -> AUDIO
            else -> OTHER
        }
    }
}
