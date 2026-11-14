package jp.kz2601.metamon.dsl.logic

import jp.kz2601.metamon.dsl.records.Entity
import jp.kz2601.metamon.dsl.records.Field
import jp.kz2601.metamon.dsl.records.FieldCoding
import jp.kz2601.metamon.dsl.records.FieldType
import jp.kz2601.metamon.dsl.records.HiddenFields
import jp.kz2601.metamon.dsl.records.JSONValue
import jp.kz2601.metamon.dsl.records.Record

/** `export` アクション(#421)で書き出す表。見出し行 + 文字列の行(UI と同じ書式に整形済み)。iOS の `ExportTable`。 */
data class ExportTable(
    val title: String,
    val headers: List<String>,
    val rows: List<List<String>>,
    /** PDF の見出し下に出す補足(絞り込み・件数など) */
    val notes: List<String> = emptyList(),
    /** 列ごとに数値か(PDF で右寄せ)。空なら全部左寄せ */
    val numericColumns: List<Boolean> = emptyList(),
)

/** 書き出しの列選び・行の整形。iOS の Records/RecordExport.swift。表示文字列への変換は呼び出し側が渡す。 */
object RecordExport {
    /** 書き出す列。`fields` 指定ならその順(写真・動画・存在しない id は飛ばす)、省略で entity の全項目(計算項目を含む)。 */
    fun columns(entity: Entity, fields: List<String>?, includeHidden: Boolean): List<Field> {
        val picked = if (!fields.isNullOrEmpty()) fields.mapNotNull { id -> entity.fields.firstOrNull { it.id == id } } else entity.fields
        return picked.filter { f -> f.type != FieldType.PHOTO && f.type != FieldType.VIDEO && (includeHidden || !f.hidden) }
    }

    /** 1 レコード = 1 行。hidden の値は自分が書いたレコードの行だけ埋め、他は空欄。 */
    fun table(
        title: String,
        entity: Entity,
        records: List<Record>,
        fields: List<String>?,
        currentUid: String?,
        notes: List<String> = emptyList(),
        display: (JSONValue, Field) -> String,
    ): ExportTable {
        val showHidden = records.any { HiddenFields.canEdit(it, currentUid) }
        val cols = columns(entity, fields, showHidden)
        val rows = records.map { r ->
            val canSeeHidden = HiddenFields.canEdit(r, currentUid)
            cols.map { f ->
                val v = r[f.id]
                when {
                    f.hidden && !canSeeHidden -> ""
                    v.isNull && !f.isComputed -> ""
                    else -> display(v, f)
                }
            }
        }
        return ExportTable(title, cols.map { it.name }, rows, notes, cols.map { it.type.isNumeric })
    }

    /** 日付の列があれば古い順(報告は時系列)。無ければ元の順。 */
    fun chronological(records: List<Record>, entity: Entity): List<Record> {
        val df = entity.fields.firstOrNull { it.type.isDate && !it.hidden } ?: return records
        // sortedWith は安定なので、比べられない組は元の順のまま
        return records.sortedWith { a, b ->
            val da = FieldCoding.date(a[df.id])
            val db = FieldCoding.date(b[df.id])
            when {
                da != null && db != null -> da.compareTo(db)
                da == null && db != null -> 1
                da != null && db == null -> -1
                else -> 0
            }
        }
    }

    private const val BAD_FILE_CHARS = "/\\:?%*|\"<>"

    /** 共有シートに出るファイル名。パス区切りや制御文字を落とし、空なら `fallback`(画面の言語の「書き出し」)。 */
    fun fileName(title: String, ext: String, fallback: String = "書き出し"): String {
        val sb = StringBuilder()
        title.codePoints().forEach { cp ->
            if (isBadFileScalar(cp)) sb.append('_') else sb.appendCodePoint(cp)
        }
        var base = sb.toString().trimSpaces()
        while (base.startsWith(".")) base = base.substring(1)
        if (base.isEmpty()) base = fallback
        return base.graphemePrefix(80) + "." + ext
    }

    /** Foundation の `controlCharacters`(Cc・Cf)と `newlines` と記号。 */
    private fun isBadFileScalar(cp: Int): Boolean {
        if (cp < 0x10000 && BAD_FILE_CHARS.indexOf(cp.toChar()) >= 0) return true
        val t = Character.getType(cp)
        if (t == Character.CONTROL.toInt() || t == Character.FORMAT.toInt()) return true
        return cp == 0x2028 || cp == 0x2029 || cp == 0x85
    }
}

/** RFC 4180 の CSV。Excel が日本語を化かさないよう UTF-8 BOM を付け、改行は CRLF。iOS の `CSVWriter`。 */
object CsvWriter {
    val BOM: ByteArray get() = byteArrayOf(0xEF.toByte(), 0xBB.toByte(), 0xBF.toByte())

    /** 1 セル。`,` `"` 改行を含めば `"` で囲み `"` は 2 つに。式に見える先頭には `'` を付ける(CSV インジェクション対策)。 */
    fun escape(cell: String): String {
        var s = cell
        if (s.isNotEmpty()) {
            val first = s.codePointAt(0)
            val nextIndex = Character.charCount(first)
            val next = if (nextIndex < s.length) s.codePointAt(nextIndex) else null
            if (isFormulaLead(first, next)) s = "'$s"
        }
        if (s.none { it == ',' || it == '"' || it == '\n' || it == '\r' }) return s
        return "\"" + s.replace("\"", "\"\"") + "\""
    }

    /** `-500` / `-¥500` のような負の数は式扱いしない(先頭 `-` は後ろが数字・¥・. の時だけ許す)。 */
    private fun isFormulaLead(c: Int, next: Int?): Boolean = when (c) {
        '='.code, '+'.code, '@'.code, '\t'.code, '\r'.code -> true
        '-'.code -> next != null && !(Character.getType(next) == Character.DECIMAL_DIGIT_NUMBER.toInt() || next == '¥'.code || next == '.'.code)
        else -> false
    }

    fun string(table: ExportTable): String =
        (listOf(table.headers) + table.rows).joinToString("\r\n") { row -> row.joinToString(",") { escape(it) } } + "\r\n"

    fun data(table: ExportTable): ByteArray = BOM + string(table).toByteArray(Charsets.UTF_8)
}
