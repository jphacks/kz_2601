package jp.kz2601.metamon.dsl.records

import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.GridTableRowRange
import java.time.DayOfWeek
import java.time.Instant
import java.time.LocalDate
import java.time.ZoneId
import kotlin.math.abs

/**
 * `gridTable`(#234)の配置計算(iOS の Records/GridTableLayout.swift)。列・行の見出しを作り、各レコードを (列, 行) のマスに置く。
 * 描画(GridTableLeaf)から切り離してテストする。
 */
class GridTableLayout(c: Component, entity: Entity?, records: List<Record>) {
    /** 見出し 1 つ。[value] は columnField / rowField に保存する値(空マスのタップで初期値になる)。 */
    data class Header(val value: JSONValue, val label: String)

    data class Cell(val column: Int, val row: Int)

    val columnField: String = c.columnField ?: ""
    val rowField: String = c.rowField ?: ""
    val columns: List<Header>
    val rows: List<Header>

    /** マス → そのマスのレコード(元の並び順)。見出しに無い値のレコードは載らない。 */
    val cells: Map<Cell, List<Record>>

    init {
        val colDef = entity?.field(columnField)
        val rowDef = entity?.field(rowField)
        columns = headers(colDef, c.columnValues.takeIf { it.isNotEmpty() }, null, records, columnField).take(MAX_COLUMNS)
        rows = headers(rowDef, c.rowValues.takeIf { it.isNotEmpty() }, c.rowRange, records, rowField).take(MAX_ROWS)
        val colIndex = HashMap<String, Int>()
        columns.forEachIndexed { i, h -> matchKey(h.value)?.let { colIndex.putIfAbsent(it, i) } }
        val rowIndex = HashMap<String, Int>()
        rows.forEachIndexed { i, h -> matchKey(h.value)?.let { rowIndex.putIfAbsent(it, i) } }
        val out = LinkedHashMap<Cell, MutableList<Record>>()
        for (r in records) {
            val ci = matchKey(r[columnField])?.let(colIndex::get) ?: continue
            val ri = matchKey(r[rowField])?.let(rowIndex::get) ?: continue
            out.getOrPut(Cell(ci, ri)) { mutableListOf() }.add(r)
        }
        cells = out
    }

    fun records(at: Cell): List<Record> = cells[at].orEmpty()

    /** 空のマスをタップした時に新規フォームへ渡す初期値(列・行の値)。 */
    fun prefill(at: Cell): Map<String, JSONValue> = buildMap {
        if (at.column in columns.indices && columnField.isNotEmpty()) put(columnField, columns[at.column].value)
        if (at.row in rows.indices && rowField.isNotEmpty()) put(rowField, rows[at.row].value)
    }

    companion object {
        /** integer の rowField で rowRange が無い時の既定(1〜6 限)と、行・列の上限。 */
        val defaultRows = 1..6
        const val MAX_ROWS = 24
        const val MAX_COLUMNS = 14

        /** 整数として扱う値の上限(これより大きい記録は行・マスに置かない)。 */
        const val MAX_MAGNITUDE = 1_000_000_000_000_000L

        internal fun headers(field: Field?, explicit: List<String>?, range: GridTableRowRange?, records: List<Record>, key: String): List<Header> {
            if (field?.type == FieldType.INTEGER || field?.type == FieldType.NUMBER) {
                val unit = field.unit ?: ""
                return integerRange(range, records, key).map { Header(JSONValue.Num(it.toDouble()), "$it$unit") }
            }
            val values = explicit?.filter { it.isNotEmpty() } ?: field?.options.orEmpty()
            return values.distinct().map { Header(JSONValue.Str(it), it) }
        }

        /** rowRange があればそれ、無ければ 1〜6(記録がそれより外ならそこまで広げる)。最大 24 行。 */
        internal fun integerRange(range: GridTableRowRange?, records: List<Record>, key: String): List<Long> {
            if (range != null && range.to >= range.from) {
                val end = minOf(range.to.toLong(), range.from.toLong() + MAX_ROWS - 1)
                return (range.from.toLong()..end).toList()
            }
            val present = records.mapNotNull { intValue(it[key]) }
            val lo = minOf(defaultRows.first.toLong(), present.minOrNull() ?: defaultRows.first.toLong())
            val hi = maxOf(defaultRows.last.toLong(), present.maxOrNull() ?: defaultRows.last.toLong())
            return (lo..minOf(hi, lo + MAX_ROWS - 1)).toList()
        }

        internal fun intValue(v: JSONValue): Long? {
            val i: Long? = when (v) {
                is JSONValue.Num -> if (v.value.isFinite() && abs(v.value) < MAX_MAGNITUDE.toDouble()) swiftRound(v.value) else null
                is JSONValue.Str -> v.value.trim { it == ' ' || it == '\t' }.let { s -> if (s.matches(Regex("[+-]?\\d+"))) s.toLongOrNull() else null }
                else -> null
            }
            return i?.takeIf { abs(it) < MAX_MAGNITUDE }
        }

        /** Swift の `rounded()`(0.5 は 0 から遠い方)。 */
        private fun swiftRound(x: Double): Long = if (x >= 0) kotlin.math.floor(x + 0.5).toLong() else -kotlin.math.floor(-x + 0.5).toLong()

        /** 値の照合キー。数値は整数に丸め、"2" と 2 を同じマスに置く。 */
        internal fun matchKey(v: JSONValue): String? = when (v) {
            is JSONValue.Num -> intValue(v)?.let { "n:$it" }
            is JSONValue.Str -> intValue(v)?.let { "n:$it" } ?: if (v.value.isEmpty()) null else "s:${v.value}"
            is JSONValue.Bool -> "s:${v.value}"
            else -> null
        }

        private val jaWeekdays = listOf("日", "月", "火", "水", "木", "金", "土")
        private val enWeekdays = listOf("sun", "mon", "tue", "wed", "thu", "fri", "sat")

        /**
         * 列見出しが曜日なら今日の列の index(「月」「月曜」「月曜日」「Mon」「Monday」)。
         * 曜日の列が 3 つ未満なら null(曜日でない表を誤って強調しない)。
         */
        fun todayColumn(labels: List<String>, now: Instant, zone: ZoneId = FieldCoding.zone): Int? {
            if (labels.isEmpty()) return null
            val weekdays = labels.map(::weekdayIndex)
            if (weekdays.count { it != null } < minOf(3, labels.size)) return null
            val today = LocalDate.ofInstant(now, zone).dayOfWeek.let { if (it == DayOfWeek.SUNDAY) 1 else it.value + 1 }
            return weekdays.indexOf(today).takeIf { it >= 0 }
        }

        /** 曜日らしい見出し → 1 = 日曜 … 7 = 土曜。 */
        internal fun weekdayIndex(label: String): Int? {
            val t = label.trim { it == ' ' || it == '\t' }
            jaWeekdays.indexOfFirst { t == it || t.startsWith(it + "曜") || t == "($it)" }.takeIf { it >= 0 }?.let { return it + 1 }
            val l = t.lowercase()
            if (l.length >= 3) enWeekdays.indexOfFirst { l.startsWith(it) }.takeIf { it >= 0 }?.let { return it + 1 }
            return null
        }
    }
}
