package jp.kz2601.metamon.dsl.logic

import jp.kz2601.metamon.dsl.expr.DSLClock
import jp.kz2601.metamon.dsl.expr.SwiftNumbers
import jp.kz2601.metamon.dsl.records.Entity
import jp.kz2601.metamon.dsl.records.Field
import jp.kz2601.metamon.dsl.records.FieldCoding
import jp.kz2601.metamon.dsl.records.FieldType
import jp.kz2601.metamon.dsl.records.JSONValue
import java.nio.ByteBuffer
import java.nio.charset.CharacterCodingException
import java.nio.charset.Charset
import java.nio.charset.CodingErrorAction
import java.time.Instant
import java.time.LocalDate
import java.time.LocalDateTime
import java.time.LocalTime
import java.time.format.DateTimeFormatter
import java.time.format.DateTimeFormatterBuilder
import java.time.format.ResolverStyle
import java.time.temporal.ChronoField
import java.time.temporal.TemporalQueries
import java.util.Locale
import kotlin.math.abs

/** `fileImport.map` の 1 つ: CSV の 1 行目が `column` の列を `field` に入れる。iOS の `ImportColumn`。 */
data class CsvColumnMapping(val column: String, val field: String)

/** RFC 4180 の CSV を読む([CsvWriter] の逆)。引用符・`""`・セル内の改行・CRLF / LF / CR・BOM に対応。iOS の `CSVReader`。 */
object CsvReader {
    /** バイト列を文字にする。UTF-8(BOM の有無どちらも)→ UTF-16(BOM 付き)→ Shift_JIS(日本語の Excel の「CSV」)。 */
    fun decode(data: ByteArray): String? {
        val bom = CsvWriter.BOM
        val body = if (data.size >= 3 && data.copyOfRange(0, 3).contentEquals(bom)) data.copyOfRange(3, data.size) else data
        strict(body, Charsets.UTF_8)?.let { return it }
        if (data.size >= 2) {
            val b0 = data[0].toInt() and 0xFF
            val b1 = data[1].toInt() and 0xFF
            if ((b0 == 0xFF && b1 == 0xFE) || (b0 == 0xFE && b1 == 0xFF)) return strict(data, Charsets.UTF_16)
        }
        return strict(data, Charset.forName("windows-31j"))
    }

    private fun strict(bytes: ByteArray, cs: Charset): String? = try {
        cs.newDecoder()
            .onMalformedInput(CodingErrorAction.REPORT)
            .onUnmappableCharacter(CodingErrorAction.REPORT)
            .decode(ByteBuffer.wrap(bytes)).toString()
    } catch (_: CharacterCodingException) {
        null
    }

    /** 行 × セル。空行(セルが 1 つで空)は飛ばす。 */
    fun parse(text: String): List<List<String>> {
        val cps = text.codePoints().toArray()
        var i = 0
        if (cps.isNotEmpty() && cps[0] == 0xFEFF) i = 1
        val rows = mutableListOf<List<String>>()
        var row = mutableListOf<String>()
        val cell = StringBuilder()
        var quoted = false
        fun endRow() {
            row.add(cell.toString())
            cell.setLength(0)
            if (!(row.size == 1 && row[0].isEmpty())) rows.add(row)
            row = mutableListOf()
        }
        while (i < cps.size) {
            val c = cps[i++]
            val next = if (i < cps.size) cps[i] else null
            if (quoted) {
                if (c == '"'.code) {
                    if (next == '"'.code) {
                        cell.append('"')
                        i++
                    } else {
                        quoted = false
                    }
                } else {
                    cell.appendCodePoint(c)
                }
                continue
            }
            when {
                c == '"'.code && cell.isEmpty() -> quoted = true
                c == ','.code -> {
                    row.add(cell.toString())
                    cell.setLength(0)
                }
                c == '\r'.code -> {
                    if (next == '\n'.code) i++
                    endRow()
                }
                c == '\n'.code -> endRow()
                else -> cell.appendCodePoint(c)
            }
        }
        if (cell.isNotEmpty() || row.isNotEmpty()) endRow()
        return rows
    }
}

/** 読めなかった 1 行(行番号は表計算と同じ: 見出しが 1 行目)。 */
data class ImportRowFailure(val line: Int, val fieldName: String, val kind: Kind, val cell: String) {
    enum class Kind {
        /** 型に合わない(数にできない・選択肢に無い・参照先が無い など) */
        INVALID,
        /** 必須の項目が空 */
        REQUIRED,
        /** 長すぎる(string 4000 字・text 20000 字) */
        TOO_LONG,
    }
}

/** CSV を記録に変える計画。確認ダイアログに件数を出し、OK なら `rows` を 1 件ずつ足す。 */
data class ImportPlan(
    /** 当てた列(見出し → field 名) */
    val matched: List<Pair<String, String>>,
    /** どの field にも当たらない列の見出し */
    val unused: List<String>,
    /** 足す記録(既定値を埋めた後) */
    val rows: List<Map<String, JSONValue>>,
    val failures: List<ImportRowFailure>,
    /** 上限(1 回 500 件・entity の合計)で入れない行の数 */
    val overLimit: Int,
)

/** `fileImport` の端末内の処理(ファイルは送らない・残さない)。iOS の Records/RecordImport.swift。 */
object RecordImport {
    /** 1 回で足す最大の件数 */
    const val MAX_ROWS = 500

    /** 1 entity の記録の上限(backend `store.MaxRecordsPerEntity`) */
    const val MAX_RECORDS_PER_ENTITY = 2000

    const val MAX_TEXT_BYTES = 5_000_000
    const val MAX_PDF_BYTES = 30_000_000
    const val MAX_OCR_PAGES = 10

    /** field に入る最大の文字数(数え方は Go の rune = コードポイント) */
    fun characterLimit(f: Field): Int = if (f.type == FieldType.TEXT) 20000 else 4000

    sealed interface CellValue {
        data object Empty : CellValue
        data class Value(val value: JSONValue) : CellValue
        data class Invalid(val kind: ImportRowFailure.Kind) : CellValue
    }

    private val unusableTypes = setOf(FieldType.PHOTO, FieldType.VIDEO, FieldType.AUDIO, FieldType.MEMBER, FieldType.MEMBERS)

    private fun key(s: String): String = s.trimWhitespace().lowercase(Locale.ROOT)

    /**
     * 見出し → 使える field(列の位置 → field)。`map` が先、残りは見出しが field の name か id と同じ列(前後の空白・大文字小文字は無視)。
     * 計算項目・写真・動画・録音・参加者には当てない。1 つの field に当てるのは最初の列だけ。
     */
    fun columns(headers: List<String>, entity: Entity, map: List<CsvColumnMapping>): Map<Int, Field> {
        val usable = entity.storedFields.filter { it.type !in unusableTypes }
        val out = linkedMapOf<Int, Field>()
        val used = mutableSetOf<String>()
        for (m in map) {
            val f = usable.firstOrNull { it.id == m.field } ?: continue
            if (f.id in used) continue
            val i = headers.indices.firstOrNull { out[it] == null && key(headers[it]) == key(m.column) } ?: continue
            out[i] = f
            used.add(f.id)
        }
        for ((i, h) in headers.withIndex()) {
            if (out[i] != null) continue
            val f = usable.firstOrNull { it.id !in used && (key(it.name) == key(h) || key(it.id) == key(h)) } ?: continue
            out[i] = f
            used.add(f.id)
        }
        return out
    }

    /** CSV の表から足す記録を作る。`refId` は ref / refs の名前 → 参照先レコードの id。`existing` は今の件数(合計 2000 件まで)。 */
    fun plan(
        table: List<List<String>>,
        entity: Entity,
        map: List<CsvColumnMapping>,
        existing: Int,
        now: Instant = DSLClock.now(),
        refId: (Field, String) -> String?,
    ): ImportPlan {
        val headers = table.firstOrNull() ?: return ImportPlan(emptyList(), emptyList(), emptyList(), emptyList(), 0)
        val cols = columns(headers, entity, map).toSortedMap()
        val matched = cols.map { (i, f) -> headers[i] to f.name }
        val unused = headers.indices.filter { cols[it] == null }.map { headers[it] }.filter { it.isNotEmpty() }
        val room = maxOf(0, minOf(MAX_ROWS, MAX_RECORDS_PER_ENTITY - existing))
        if (cols.isEmpty()) return ImportPlan(emptyList(), unused, emptyList(), emptyList(), 0)
        val rows = mutableListOf<Map<String, JSONValue>>()
        val failures = mutableListOf<ImportRowFailure>()
        var over = 0
        for ((n, cells) in table.drop(1).withIndex()) {
            val line = n + 2
            val data = linkedMapOf<String, JSONValue>()
            var failure: ImportRowFailure? = null
            for ((i, f) in cols) {
                val cell = if (i < cells.size) cells[i] else ""
                when (val v = value(cell, f, now, refId)) {
                    CellValue.Empty -> Unit
                    is CellValue.Value -> data[f.id] = v.value
                    is CellValue.Invalid -> if (failure == null) failure = ImportRowFailure(line, f.name, v.kind, cell.graphemePrefix(40))
                }
            }
            if (failure == null) {
                for (f in entity.storedFields) {
                    if (data[f.id] != null) continue
                    val d = FieldCoding.defaultValue(f)
                    if (!d.isNull) data[f.id] = d
                }
                entity.storedFields.firstOrNull { it.required == true && (data[it.id]?.isNull ?: true) }?.let {
                    failure = ImportRowFailure(line, it.name, ImportRowFailure.Kind.REQUIRED, "")
                }
            }
            when {
                failure != null -> failures.add(failure!!)
                rows.size < room -> rows.add(data)
                else -> over += 1
            }
        }
        return ImportPlan(matched, unused, rows, failures, over)
    }

    private const val FORMULA_GUARDED = "=+-@\t\r"

    /** export の CSV インジェクション対策(先頭の ')を外す。 */
    private fun unguard(s: String): String =
        if (s.length >= 2 && s[0] == '\'' && FORMULA_GUARDED.indexOf(s[1]) >= 0) s.substring(1) else s

    private val trueWords = setOf("true", "yes", "y", "1", "on", "はい", "✓", "✔", "○", "〇", "済")
    private val falseWords = setOf("false", "no", "n", "0", "off", "いいえ", "×", "✗", "未")

    /** 1 セル → field の値。`export` が書いた表示の書式(¥1,200・はい / いいえ・10/24(金)・参照先の名前)も読む。 */
    fun value(raw: String, field: Field, now: Instant = DSLClock.now(), refId: (Field, String) -> String?): CellValue {
        val s = unguard(raw.trimWhitespace())
        if (s.isEmpty() || s == FieldCoding.UNAVAILABLE) return CellValue.Empty
        val invalid = CellValue.Invalid(ImportRowFailure.Kind.INVALID)
        return when (field.type) {
            FieldType.STRING, FieldType.TEXT -> {
                // 文字はセルのまま(前後の空白も残す)
                val text = unguard(raw)
                if (text.scalarCount() > characterLimit(field)) CellValue.Invalid(ImportRowFailure.Kind.TOO_LONG) else CellValue.Value(JSONValue.Str(text))
            }
            FieldType.INTEGER, FieldType.CURRENCY -> number(s, field.unit)?.let { CellValue.Value(JSONValue.Num(SwiftNumbers.rounded(it))) } ?: invalid
            FieldType.NUMBER -> number(s, field.unit)?.let { CellValue.Value(JSONValue.Num(it)) } ?: invalid
            FieldType.BOOLEAN -> when (s.lowercase(Locale.ROOT)) {
                in trueWords -> CellValue.Value(JSONValue.Bool(true))
                in falseWords -> CellValue.Value(JSONValue.Bool(false))
                else -> invalid
            }
            FieldType.DATE, FieldType.DATETIME -> date(s, now)?.let { CellValue.Value(JSONValue.Str(FieldCoding.encode(it, field))) } ?: invalid
            FieldType.ENUM -> {
                val options = field.options ?: emptyList()
                when {
                    s in options -> CellValue.Value(JSONValue.Str(s))
                    else -> options.firstOrNull { it.lowercase(Locale.ROOT) == s.lowercase(Locale.ROOT) }?.let { CellValue.Value(JSONValue.Str(it)) } ?: invalid
                }
            }
            FieldType.REF -> refId(field, s)?.let { CellValue.Value(JSONValue.Str(it)) } ?: invalid
            FieldType.REFS -> {
                if (s == "全員" || s.lowercase(Locale.ROOT) == "everyone") return CellValue.Empty
                val names = s.replace(", and ", ",").replace(" and ", ",")
                    .split('、', ',', '，').map { it.trimSpaces() }.filter { it.isNotEmpty() }
                val ids = mutableListOf<JSONValue>()
                for (name in names) ids.add(JSONValue.Str(refId(field, name) ?: return invalid))
                if (ids.isEmpty()) CellValue.Empty else CellValue.Value(JSONValue.Arr(ids))
            }
            FieldType.LOCATION -> {
                // Swift の split は空の部分を落とす
                val parts = s.split(',').filter { it.isNotEmpty() }.map { SwiftNumbers.parseDouble(it.trimSpaces()) }
                val lat = parts.getOrNull(0)
                val lng = parts.getOrNull(1)
                if (parts.size != 2 || lat == null || lng == null || abs(lat) > 90 || abs(lng) > 180) invalid
                else CellValue.Value(JSONValue.Str(s.replace(" ", "")))
            }
            FieldType.PHOTO, FieldType.VIDEO, FieldType.AUDIO, FieldType.MEMBER, FieldType.MEMBERS -> CellValue.Empty
        }
    }

    /** 全角の英数記号・全角空白・￥ を半角に(ICU の Fullwidth-Halfwidth のうち数値の読み取りに効く分)。 */
    internal fun fullwidthToHalfwidth(s: String): String {
        val sb = StringBuilder(s.length)
        for (c in s) {
            sb.append(
                when (c.code) {
                    in 0xFF01..0xFF5E -> (c.code - 0xFEE0).toChar()
                    0x3000 -> ' '
                    0xFFE5 -> '¥'
                    else -> c
                },
            )
        }
        return sb.toString()
    }

    /** 「¥1,200」「1,200 円」「-¥500」「5 km」「１２」→ 数。単位・通貨記号・桁区切りを外す。 */
    internal fun number(s: String, unit: String?): Double? {
        var t = fullwidthToHalfwidth(s)
        if (!unit.isNullOrEmpty() && t.endsWith(unit)) t = t.dropLast(unit.length)
        FieldCoding.currencySuffix(unit)?.let { suffix -> if (t.endsWith(suffix)) t = t.dropLast(suffix.length) }
        for (token in listOf("¥", "円", "$", ",", " ")) t = t.replace(token, "")
        if (t.isEmpty()) return null
        val n = SwiftNumbers.parseDouble(t) ?: return null
        return if (n.isFinite()) n else null
    }

    private val weekdayPattern = Regex("[(（][^)）]*[)）]")

    private val dateFormats = listOf(
        "yyyy/M/d H:mm:ss", "yyyy/M/d H:mm", "yyyy/M/d", "yyyy-M-d H:mm:ss", "yyyy-M-d H:mm", "yyyy-M-d'T'H:mm",
        "yyyy年M月d日 H:mm", "yyyy年M月d日", "M/d H:mm", "M/d", "M月d日 H:mm", "M月d日",
        "EEE, MMM d, yyyy, h:mm a", "EEE, MMM d, h:mm a", "EEE, MMM d, yyyy", "EEE, MMM d", "MMM d, yyyy", "MMM d",
    )

    private fun formatter(pattern: String, year: Int): DateTimeFormatter =
        DateTimeFormatterBuilder()
            .parseCaseInsensitive()
            // 年は uuuu(STRICT で yyyy は紀元が要る)
            .appendPattern(pattern.replace("yyyy", "uuuu"))
            .parseDefaulting(ChronoField.YEAR, year.toLong())
            .toFormatter(Locale.US)
            .withResolverStyle(ResolverStyle.STRICT)

    /**
     * 日付・日時を読む。RFC 3339・YYYY-MM-DD、export の「2026/10/24(金) 14:30」「10/24(金)」(年が無ければ `now` の年)、
     * 「2026年10月24日」、英語 UI の「Fri, Oct 24, 2:30 PM」。時刻は端末のタイムゾーン([FieldCoding.zone])。
     */
    fun date(raw: String, now: Instant = DSLClock.now()): Instant? {
        FieldCoding.date(JSONValue.Str(raw))?.let { return it }
        val s = weekdayPattern.replace(raw, "").replace("  ", " ").trimSpaces()
        val zone = FieldCoding.zone
        val year = LocalDate.ofInstant(now, zone).year
        for (pattern in dateFormats) {
            val parsed = try {
                formatter(pattern, year).parse(s)
            } catch (_: Exception) {
                continue
            }
            val day = parsed.query(TemporalQueries.localDate()) ?: continue
            val time = parsed.query(TemporalQueries.localTime()) ?: LocalTime.MIDNIGHT
            return LocalDateTime.of(day, time).atZone(zone).toInstant()
        }
        return null
    }

    /** テキストファイルの中身(UTF-8 / UTF-16 / Shift_JIS)。 */
    fun plainText(data: ByteArray): String? = CsvReader.decode(data)

    data class BodyRecord(val data: Map<String, JSONValue>, val truncated: Boolean)

    /**
     * 本文を 1 件の記録にする。本文は `field` の上限で切り(切ったら `truncated`)、
     * entity の titleField(string / text)にはファイル名(拡張子なし)。
     */
    fun bodyRecord(text: String, fileName: String, entity: Entity, field: String): BodyRecord? {
        val f = entity.storedFields.firstOrNull { it.id == field && (it.type == FieldType.TEXT || it.type == FieldType.STRING) } ?: return null
        val body = text.replace("\r\n", "\n").trimWhitespace()
        val limit = characterLimit(f)
        val data = linkedMapOf<String, JSONValue>()
        val truncated = body.scalarCount() > limit
        data[f.id] = JSONValue.Str(if (truncated) body.substring(0, body.offsetByCodePoints(0, limit)) else body)
        val tid = entity.titleField
        if (tid != null && tid != f.id) {
            val t = entity.storedFields.firstOrNull { it.id == tid }
            if (t != null && (t.type == FieldType.STRING || t.type == FieldType.TEXT)) {
                val name = deletingPathExtension(fileName).trimSpaces()
                if (name.isNotEmpty()) data[t.id] = JSONValue.Str(name.graphemePrefix(200))
            }
        }
        for (x in entity.storedFields) {
            if (data[x.id] != null) continue
            val d = FieldCoding.defaultValue(x)
            if (!d.isNull) data[x.id] = d
        }
        return BodyRecord(data, truncated)
    }

    /** NSString の `deletingPathExtension`(最後の「.」以降を落とす。先頭の「.」だけの名前はそのまま)。 */
    private fun deletingPathExtension(name: String): String {
        val last = name.substringAfterLast('/')
        val dot = last.lastIndexOf('.')
        if (dot <= 0) return name
        return name.substring(0, name.length - (last.length - dot))
    }
}
