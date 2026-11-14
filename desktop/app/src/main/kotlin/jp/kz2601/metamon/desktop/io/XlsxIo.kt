package jp.kz2601.metamon.desktop.io

import jp.kz2601.metamon.dsl.logic.RecordImport
import java.io.ByteArrayInputStream
import java.io.ByteArrayOutputStream
import java.io.InputStream
import java.math.BigDecimal
import java.math.MathContext
import java.time.LocalDate
import java.time.LocalDateTime
import java.time.LocalTime
import java.time.format.DateTimeFormatter
import java.util.zip.ZipInputStream
import javax.xml.stream.XMLInputFactory
import javax.xml.stream.XMLStreamConstants
import javax.xml.stream.XMLStreamReader
import kotlin.math.roundToLong

/**
 * .xlsx の 1 枚のシート → CSV の取り込みと同じ「行 × 列の文字」(#2302 の 1)。画面は持たない。
 * 依存を足さず JDK の zip と StAX で読む(Apache POI は 10 MB を超える)。
 * - 文字は共有文字列・その場の文字、数は Excel の見た目と同じ 15 桁、式は保存されている計算済みの値
 * - 日付の書式のセルは `yyyy-MM-dd`(時刻があれば `yyyy-MM-dd HH:mm[:ss]`、時刻だけなら `HH:mm[:ss]`)。1904 年始まりにも合わせる
 * - 真偽は TRUE / FALSE、エラーは `#DIV/0!` などそのまま。空の行は飛ばす(CsvReader と同じ)
 */
object XlsxIo {
    sealed interface Result {
        /** [sheetNames] はブックの全シートの名前(順番どおり)。[rows] は読んだシートの表。 */
        data class Table(val sheetNames: List<String>, val sheet: String, val rows: List<List<String>>) : Result

        /** 上限([RecordImport.MAX_TEXT_BYTES]・展開後の大きさ)を超える。 */
        data object TooLarge : Result

        /** .xlsx として読めない(壊れている・暗号化・.xls など)。 */
        data object Unreadable : Result
    }

    /** 展開した後の 1 つの部品の上限(zip 爆弾を断る)。 */
    internal const val MAX_PART_BYTES = 50_000_000L

    /** 展開した後の合計の上限。 */
    internal const val MAX_TOTAL_BYTES = 100_000_000L

    /** 列の上限(Excel の XFD = 16384)。 */
    private const val MAX_COLUMNS = 16_384

    private const val NS_REL = "http://schemas.openxmlformats.org/officeDocument/2006/relationships"

    /** [sheetIndex] 枚目(0 始まり・既定は最初)のシートを読む。 */
    fun read(bytes: ByteArray, sheetIndex: Int = 0): Result {
        if (bytes.size > RecordImport.MAX_TEXT_BYTES) return Result.TooLarge
        val parts = try {
            unzip(bytes) ?: return Result.TooLarge
        } catch (e: Exception) {
            return Result.Unreadable
        }
        return try {
            parse(parts, sheetIndex) ?: Result.Unreadable
        } catch (e: Exception) {
            Result.Unreadable
        }
    }

    private class TooLargeException : Exception()

    /** 要る部品(xl/ の XML と rels)だけを展開する。上限を超えれば null。 */
    private fun unzip(bytes: ByteArray): Map<String, ByteArray>? {
        val out = mutableMapOf<String, ByteArray>()
        var total = 0L
        ZipInputStream(ByteArrayInputStream(bytes)).use { zip ->
            while (true) {
                val e = zip.nextEntry ?: break
                val name = e.name.removePrefix("/")
                if (e.isDirectory || !(name.startsWith("xl/") && (name.endsWith(".xml") || name.endsWith(".rels")))) continue
                val buf = ByteArrayOutputStream()
                val chunk = ByteArray(64 * 1024)
                var size = 0L
                while (true) {
                    val n = zip.read(chunk)
                    if (n < 0) break
                    size += n
                    total += n
                    if (size > MAX_PART_BYTES || total > MAX_TOTAL_BYTES) return null
                    buf.write(chunk, 0, n)
                }
                out[name] = buf.toByteArray()
            }
        }
        return out
    }

    private fun parse(parts: Map<String, ByteArray>, sheetIndex: Int): Result? {
        val workbook = parts["xl/workbook.xml"] ?: return null
        val (sheets, date1904) = workbook(workbook)
        val sheet = sheets.getOrNull(sheetIndex) ?: return null
        val target = relationships(parts["xl/_rels/workbook.xml.rels"] ?: return null)[sheet.second] ?: return null
        val path = if (target.startsWith("/")) target.removePrefix("/") else "xl/" + target.removePrefix("./")
        val sheetXml = parts[path] ?: return null
        val shared = parts["xl/sharedStrings.xml"]?.let(::sharedStrings).orEmpty()
        val dateStyles = parts["xl/styles.xml"]?.let(::dateStyles).orEmpty()
        val rows = sheetRows(sheetXml, shared, dateStyles, date1904)
        return Result.Table(sheets.map { it.first }, sheet.first, rows)
    }

    // MARK: - XML

    private val factory: XMLInputFactory = XMLInputFactory.newInstance().apply {
        setProperty(XMLInputFactory.SUPPORT_DTD, false)
        setProperty(XMLInputFactory.IS_SUPPORTING_EXTERNAL_ENTITIES, false)
        setProperty(XMLInputFactory.IS_NAMESPACE_AWARE, true)
    }

    private inline fun xml(bytes: ByteArray, block: (XMLStreamReader) -> Unit) {
        val input: InputStream = ByteArrayInputStream(bytes)
        val r = factory.createXMLStreamReader(input)
        try {
            block(r)
        } finally {
            r.close()
        }
    }

    private fun XMLStreamReader.attr(name: String): String? = getAttributeValue(null, name)

    /** シートの (名前, r:id) の並びと、1904 年始まりか。 */
    private fun workbook(bytes: ByteArray): Pair<List<Pair<String, String>>, Boolean> {
        val sheets = mutableListOf<Pair<String, String>>()
        var date1904 = false
        xml(bytes) { r ->
            while (r.hasNext()) {
                if (r.next() != XMLStreamConstants.START_ELEMENT) continue
                when (r.localName) {
                    "workbookPr" -> date1904 = r.attr("date1904").let { it == "1" || it == "true" }
                    "sheet" -> {
                        val id = r.getAttributeValue(NS_REL, "id") ?: continue
                        sheets += (r.attr("name") ?: "") to id
                    }
                }
            }
        }
        return sheets to date1904
    }

    private fun relationships(bytes: ByteArray): Map<String, String> {
        val out = mutableMapOf<String, String>()
        xml(bytes) { r ->
            while (r.hasNext()) {
                if (r.next() == XMLStreamConstants.START_ELEMENT && r.localName == "Relationship") {
                    val id = r.attr("Id") ?: continue
                    out[id] = r.attr("Target") ?: continue
                }
            }
        }
        return out
    }

    /** `<si>` ごとの文字。ふりがな(`<rPh>`)は入れない。 */
    private fun sharedStrings(bytes: ByteArray): List<String> {
        val out = mutableListOf<String>()
        xml(bytes) { r ->
            val sb = StringBuilder()
            var phonetic = 0
            var inText = false
            while (r.hasNext()) {
                when (r.next()) {
                    XMLStreamConstants.START_ELEMENT -> when (r.localName) {
                        "si" -> sb.setLength(0)
                        "rPh" -> phonetic++
                        "t" -> inText = phonetic == 0
                    }
                    XMLStreamConstants.CHARACTERS, XMLStreamConstants.CDATA, XMLStreamConstants.SPACE -> if (inText) sb.append(r.text)
                    XMLStreamConstants.END_ELEMENT -> when (r.localName) {
                        "si" -> out += sb.toString()
                        "rPh" -> phonetic--
                        "t" -> inText = false
                    }
                }
            }
        }
        return out
    }

    /** 日付・時刻の書式の、`cellXfs` の並びの番号。 */
    private fun dateStyles(bytes: ByteArray): Set<Int> {
        val custom = mutableMapOf<Int, String>()
        val xfs = mutableListOf<Int>()
        xml(bytes) { r ->
            var inCellXfs = false
            while (r.hasNext()) {
                when (r.next()) {
                    XMLStreamConstants.START_ELEMENT -> when (r.localName) {
                        "numFmt" -> r.attr("numFmtId")?.toIntOrNull()?.let { custom[it] = r.attr("formatCode").orEmpty() }
                        "cellXfs" -> inCellXfs = true
                        "xf" -> if (inCellXfs) xfs += r.attr("numFmtId")?.toIntOrNull() ?: 0
                    }
                    XMLStreamConstants.END_ELEMENT -> if (r.localName == "cellXfs") inCellXfs = false
                }
            }
        }
        return xfs.withIndex().filter { (_, id) -> custom[id]?.let(::isDateFormat) ?: isBuiltInDate(id) }.map { it.index }.toSet()
    }

    /** 組み込みの日付・時刻の書式(14〜22・45〜47・日本語の 27〜36・50〜58)。 */
    internal fun isBuiltInDate(id: Int): Boolean = id in 14..22 || id in 27..36 || id in 45..47 || id in 50..58

    private val quoted = Regex(""""[^"]*"|\\.|_.|\*.""")
    private val brackets = Regex("""\[[^]]*]""")
    private val elapsed = Regex("""\[(h+|m+|s+)]""", RegexOption.IGNORE_CASE)

    /** 自前の書式が日付・時刻か(引用・色・条件を除いて y m d h s があるか)。 */
    internal fun isDateFormat(code: String): Boolean {
        if (elapsed.containsMatchIn(code)) return true
        val plain = brackets.replace(quoted.replace(code.substringBefore(';'), ""), "")
        return plain.any { it.lowercaseChar() in "ymdhs" }
    }

    // MARK: - セル

    private fun sheetRows(bytes: ByteArray, shared: List<String>, dateStyles: Set<Int>, date1904: Boolean): List<List<String>> {
        val rows = mutableListOf<List<String>>()
        xml(bytes) { r ->
            var cells: MutableList<String>? = null
            var nextCol = 0
            var col = 0
            var type = "n"
            var style = 0
            var value: String? = null
            val text = StringBuilder()
            var capture = false
            var inline = false
            var phonetic = 0
            while (r.hasNext()) {
                when (r.next()) {
                    XMLStreamConstants.START_ELEMENT -> when (r.localName) {
                        "row" -> {
                            cells = mutableListOf()
                            nextCol = 0
                        }
                        "c" -> {
                            col = r.attr("r")?.let(::column) ?: nextCol
                            type = r.attr("t") ?: "n"
                            style = r.attr("s")?.toIntOrNull() ?: 0
                            value = null
                            inline = false
                        }
                        "v" -> {
                            text.setLength(0)
                            capture = true
                        }
                        "is" -> {
                            inline = true
                            text.setLength(0)
                        }
                        "rPh" -> phonetic++
                        "t" -> if (inline && phonetic == 0) capture = true
                    }
                    XMLStreamConstants.CHARACTERS, XMLStreamConstants.CDATA, XMLStreamConstants.SPACE -> if (capture) text.append(r.text)
                    XMLStreamConstants.END_ELEMENT -> when (r.localName) {
                        "v" -> {
                            value = text.toString()
                            capture = false
                        }
                        "t" -> capture = false
                        "rPh" -> phonetic--
                        "is" -> {
                            value = text.toString()
                            inline = false
                        }
                        "c" -> {
                            val row = cells ?: continue
                            require(col < MAX_COLUMNS) { "column out of range" }
                            val s = cellText(type, value, style in dateStyles, shared, date1904)
                            while (row.size < col) row += ""
                            if (row.size == col) row += s else row[col] = s
                            nextCol = col + 1
                        }
                        "row" -> {
                            val row = cells ?: continue
                            while (row.isNotEmpty() && row.last().isEmpty()) row.removeAt(row.lastIndex)
                            if (row.isNotEmpty()) rows += row.toList()
                            cells = null
                        }
                    }
                }
            }
        }
        return rows
    }

    /** "C12" → 2(A = 0)。 */
    internal fun column(ref: String): Int? {
        var n = 0
        var letters = 0
        for (ch in ref) {
            if (ch !in 'A'..'Z') break
            n = n * 26 + (ch - 'A' + 1)
            letters++
            if (letters > 3) return null
        }
        return if (letters == 0) null else n - 1
    }

    private fun cellText(type: String, value: String?, isDate: Boolean, shared: List<String>, date1904: Boolean): String {
        val v = value ?: return ""
        return when (type) {
            "s" -> v.trim().toIntOrNull()?.let { shared.getOrNull(it) }.orEmpty()
            "inlineStr", "str", "e" -> v
            "b" -> if (v.trim() == "1") "TRUE" else "FALSE"
            else -> {
                val n = v.trim().toBigDecimalOrNull() ?: return v
                if (isDate) date(n.toDouble(), date1904) ?: number(n) else number(n)
            }
        }
    }

    /** Excel の見た目と同じく 15 桁に丸め、末尾の 0 と指数を出さない。 */
    internal fun number(n: BigDecimal): String {
        val s = n.round(MathContext(15)).stripTrailingZeros().toPlainString()
        return if (s == "-0") "0" else s
    }

    private val dayFormat = DateTimeFormatter.ofPattern("yyyy-MM-dd")
    private val minuteFormat = DateTimeFormatter.ofPattern("yyyy-MM-dd HH:mm")
    private val secondFormat = DateTimeFormatter.ofPattern("yyyy-MM-dd HH:mm:ss")

    /** 日付のシリアル値 → 文字。範囲外(負・9999 年より後)は null(数として出す)。 */
    internal fun date(serial: Double, date1904: Boolean): String? {
        if (serial < 0 || serial >= 2_958_466) return null
        val seconds = (serial * 86_400).roundToLong()
        val days = seconds / 86_400
        val secOfDay = seconds % 86_400
        val time = "%02d:%02d".format(secOfDay / 3600, secOfDay % 3600 / 60) + if (secOfDay % 60 != 0L) ":%02d".format(secOfDay % 60) else ""
        // 1900 年始まりの 0 日目は時刻だけの値
        if (days == 0L && !date1904) return time
        // 1900 年始まりは 1900-02-29(実在しない日・60)を数えるので、61 以降は 1 日戻す
        val base = if (date1904) LocalDate.of(1904, 1, 1) else LocalDate.of(1899, 12, 31)
        val day = base.plusDays(if (!date1904 && days >= 60) days - 1 else days)
        val at = LocalDateTime.of(day, LocalTime.ofSecondOfDay(secOfDay))
        return when {
            secOfDay == 0L -> at.format(dayFormat)
            secOfDay % 60 == 0L -> at.format(minuteFormat)
            else -> at.format(secondFormat)
        }
    }
}
