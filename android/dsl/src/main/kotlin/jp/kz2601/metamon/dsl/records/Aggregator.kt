package jp.kz2601.metamon.dsl.records

import jp.kz2601.metamon.dsl.expr.DSLClock
import jp.kz2601.metamon.dsl.expr.SwiftNumbers
import java.time.DayOfWeek
import java.time.Instant
import java.time.LocalDate
import java.time.ZoneId
import java.time.format.DateTimeFormatter
import java.time.temporal.TemporalAdjusters
import java.util.Locale

/**
 * stat / chart / list の filter・sort・集計(iOS の Records/Aggregator.swift)。v1 は端末で集計する。
 * 期間 filter(`today` / `thisWeek` / `thisMonth` / `thisYear`)は端末のタイムゾーン基準。
 */
object Aggregator {
    /** 解釈する `Filter.op`(schema の enum と一致させる)。 */
    val filterOps: Set<String> = setOf("eq", "neq", "gt", "gte", "lt", "lte", "today", "thisWeek", "thisMonth", "thisYear", "contains")

    fun apply(filter: Filter?, entity: Entity?, records: List<Record>): List<Record> {
        if (filter == null) return records
        val field = entity?.field(filter.field)
        return records.filter { matches(filter, it, field) }
    }

    fun matches(f: Filter, record: Record, field: Field?, now: Instant = DSLClock.now(), zone: ZoneId = FieldCoding.zone): Boolean {
        RecordValue.markId(f.field)?.let { mid ->
            // 自分が付けたか(#255)。value は me(resolvingMe 後は uid 文字列)
            if (f.value == null || f.value == JSONValue.Null) return false
            val mine = record.markState(mid).mine
            return if (f.op == "neq") !mine else mine
        }
        if (f.field == RecordValue.AUTHOR) {
            f.followedAuthors?.let { followed ->
                // フォローしている人(#270)
                val hit = (record.author ?: "") in followed
                return if (f.op == "neq") !hit else hit
            }
            // 投稿者の uid と比べる(#251)。uid に置き換わっていない me はどれにも一致しない
            val uid = f.value?.stringValue ?: return false
            val same = (record.author ?: "") == uid
            return if (f.op == "neq") !same else same
        }
        val v = record[f.field]
        return when (f.op) {
            "thisMonth" -> {
                val d = FieldCoding.date(v) ?: return false
                val a = LocalDate.ofInstant(d, zone)
                val b = LocalDate.ofInstant(now, zone)
                a.year == b.year && a.monthValue == b.monthValue
            }
            "today" -> {
                val d = FieldCoding.date(v) ?: return false
                LocalDate.ofInstant(d, zone) == LocalDate.ofInstant(now, zone)
            }
            "thisWeek" -> {
                // 月曜 0:00 ≤ d < 次の月曜 0:00
                val d = FieldCoding.date(v) ?: return false
                val (start, end) = week(now, zone)
                d >= start && d < end
            }
            "thisYear" -> {
                val d = FieldCoding.date(v) ?: return false
                LocalDate.ofInstant(d, zone).year == LocalDate.ofInstant(now, zone).year
            }
            "contains" -> {
                // type refs(#231): 未選択 = 全員なので常に一致
                val ids = refIds(v)
                ids.isEmpty() || (f.value?.stringValue ?: "") in ids
            }
            "eq" -> v == f.value || v.displayString == (f.value ?: JSONValue.Null).displayString
            "neq" -> !(v == f.value || v.displayString == (f.value ?: JSONValue.Null).displayString)
            "gt", "gte", "lt", "lte" -> {
                val order = filterOrder(v, f.value ?: JSONValue.Null) ?: return false
                when (f.op) {
                    "gt" -> order > 0
                    "gte" -> order >= 0
                    "lt" -> order < 0
                    else -> order <= 0
                }
            }
            else -> true
        }
    }

    /** 今週(月曜 0:00 から次の月曜 0:00)。週の始まりは月曜に固定(日本の「今週」は月〜日)。 */
    fun week(now: Instant, zone: ZoneId): Pair<Instant, Instant> {
        val monday = LocalDate.ofInstant(now, zone).with(TemporalAdjusters.previousOrSame(DayOfWeek.MONDAY))
        return monday.atStartOfDay(zone).toInstant() to monday.plusDays(7).atStartOfDay(zone).toInstant()
    }

    /** 大小比較の filter: 数値(数の文字列も)同士は数値、日付同士は日付で比べる。それ以外は比べない(null)。 */
    fun filterOrder(a: JSONValue, b: JSONValue): Int? {
        fun num(v: JSONValue): Double? = v.doubleValue ?: v.stringValue?.let { parseNumber(it) }
        val x = num(a)
        val y = num(b)
        if (x != null && y != null) return order(x, y)
        val p = FieldCoding.date(a)
        val q = FieldCoding.date(b)
        if (p != null && q != null) return p.compareTo(q).coerceIn(-1, 1)
        return null
    }

    /** -1 / 0 / 1。NaN を含む時は「大きい」(Swift の `x == y ? same : x < y ? asc : desc` と同じ)。 */
    fun order(x: Double, y: Double): Int = if (x == y) 0 else if (x < y) -1 else 1

    /** 文字列 → 数値。桁区切りのカンマは除く("1,200" = 1200)。 */
    fun parseNumber(s: String): Double? = SwiftNumbers.parseDouble(s.replace(",", ""))

    /** type refs の保存値 → id の配列(文字列 1 つは 1 件として読む)。 */
    fun refIds(v: JSONValue): List<String> = when (v) {
        is JSONValue.Arr -> v.items.mapNotNull { it.stringValue }.filter { it.isNotEmpty() }
        is JSONValue.Str -> if (v.value.isEmpty()) emptyList() else listOf(v.value)
        else -> emptyList()
    }

    /** 並べ替え。sort 無しは作成日時の新しい順。数値の field は数値、それ以外は表示文字列で比べる。 */
    fun sorted(records: List<Record>, sort: Sort?, entity: Entity?): List<Record> {
        if (sort == null) return records.sortedWith { a, b -> (b.createdAt ?: "").compareTo(a.createdAt ?: "") }
        val asc = sort.direction != "desc"
        val field = entity?.field(sort.field)
        // 数値の field は数値、それ以外は表示文字列で比べる。同じ値は元の順(安定)
        val cmp = Comparator<Record> { a, b ->
            val x = a[sort.field]
            val y = b[sort.field]
            val p = x.doubleValue
            val q = y.doubleValue
            if (field != null && field.type.isNumeric && p != null && q != null) p.compareTo(q) else x.displayString.compareTo(y.displayString)
        }
        return records.sortedWith(if (asc) cmp else cmp.reversed())
    }

    /** count|sum|avg|min|max(既定 count)。数値でない値は数えない。空の avg/min/max は 0。 */
    fun aggregate(op: String?, field: String?, records: List<Record>): Double {
        val nums = field?.let { f -> records.mapNotNull { it[f].doubleValue } } ?: emptyList()
        return when (op ?: "count") {
            "sum" -> nums.fold(0.0) { a, b -> a + b }
            "avg" -> if (nums.isEmpty()) 0.0 else nums.fold(0.0) { a, b -> a + b } / nums.size
            "min" -> nums.minOrNull() ?: 0.0
            "max" -> nums.maxOrNull() ?: 0.0
            else -> records.size.toDouble()
        }
    }

    /** percent は比率(0〜1)を 100 倍で出す(0.5 →「50%」)。 */
    fun format(value: Double, format: String?, unit: String? = null, locale: Locale = Locale.getDefault()): String = when (format) {
        "currency" -> FieldCoding.currency(value, locale)
        "percent" -> cFormat(value * 100, 0) + "%"
        else -> {
            val s = FieldCoding.plainNumber(value)
            unit?.let { "$s $it" } ?: s
        }
    }

    /** C の printf("%.Nf") と同じ丸め(2 進の値を偶数丸め)。 */
    internal fun cFormat(x: Double, digits: Int): String {
        if (!x.isFinite()) return SwiftNumbers.description(x)
        val s = java.math.BigDecimal(x).setScale(digits, java.math.RoundingMode.HALF_EVEN).toPlainString()
        // 負の値が 0 に丸まった時も C は「-0」と書く
        return if ((x < 0 || 1.0 / x < 0) && !s.startsWith("-")) "-$s" else s
    }

    // MARK: - latest / earliest(#133)

    /** 1 件の値を返す集計(latest / earliest)か。 */
    fun isPick(op: String?): Boolean = op == "latest" || op == "earliest"

    /**
     * latest / earliest の対象レコード。`by`(date/datetime field)で並べ、省略時は entity の最初の日付 field、
     * それも無ければ作成順。同じ日は作成順で後の方を新しいとみなす。by が空のレコードは除く。0 件は null。
     */
    fun pick(op: String, by: String?, entity: Entity?, records: List<Record>): Record? {
        val byField = by ?: entity?.fields?.firstOrNull { it.type.isDate }?.id
        data class Key(val date: Instant?, val created: String, val index: Int)
        val keyed = records.mapIndexedNotNull { i, r ->
            val d = byField?.let { FieldCoding.date(r[it]) }
            if (byField != null && d == null) null else Key(d, r.createdAt ?: "", i) to r
        }
        val sorted = keyed.sortedWith { (a, _), (b, _) ->
            if (a.date != null && b.date != null && a.date != b.date) a.date.compareTo(b.date)
            else if (a.created != b.created) a.created.compareTo(b.created)
            else a.index.compareTo(b.index)
        }
        return if (op == "earliest") sorted.firstOrNull()?.second else sorted.lastOrNull()?.second
    }

    /**
     * stat の aggregate latest / earliest の表示文字列(数値は format・unit、それ以外は field の表示形)。0 件・空は null。
     * iOS は Component を受けるが、ここでは使う props(aggregate / field / by / format)だけを受ける。
     */
    fun pickText(
        aggregate: String?,
        fieldId: String?,
        by: String?,
        format: String?,
        entity: Entity?,
        records: List<Record>,
        locale: Locale = Locale.getDefault(),
    ): String? {
        val op = aggregate ?: return null
        val id = fieldId ?: return null
        val r = pick(op, by, entity, records) ?: return null
        val v = r[id]
        if (v.isNull) return null
        val f = entity?.field(id)
        val n = v.doubleValue
        if (n != null && (f?.type?.isNumeric ?: true)) {
            val fmt = format ?: if (f?.type == FieldType.CURRENCY) "currency" else null
            return format(n, fmt, f?.unit, locale)
        }
        return FieldCoding.display(v, f, locale)
    }

    // MARK: - chart のグループ

    data class Bucket(val key: String, val label: String, val value: Double)

    /**
     * chart: groupBy(enum or 日付 field)でグループ化して集計。日付は `bucket`(day|week|month)で丸める。
     * `label` は日付以外の値 → 表示名(ref の groupBy の見出し・#235)。`locale` が null・日本語なら「3月」「3/4週」「3/4」。
     * iOS は Component を受けるが、ここでは使う props(groupBy / bucket / aggregate / field)だけを受ける。
     * 週の始まりは端末の設定(`firstDayOfWeek`・既定は日曜)。
     */
    fun buckets(
        groupBy: String?,
        bucket: String?,
        aggregate: String?,
        field: String?,
        entity: Entity?,
        records: List<Record>,
        label: ((String) -> String)? = null,
        locale: Locale? = null,
        firstDayOfWeek: DayOfWeek = DayOfWeek.SUNDAY,
    ): List<Bucket> {
        val g = groupBy ?: return emptyList()
        val f = entity?.field(g)
        val groups = LinkedHashMap<String, Pair<String, MutableList<Record>>>()
        var order = mutableListOf<String>()
        for (r in records) {
            val v = r[g]
            val d = if (f != null && f.type.isDate) FieldCoding.date(v) else null
            val (key, lab) = if (d != null) {
                dateBucket(d, bucket ?: "day", locale, firstDayOfWeek)
            } else {
                val k = v.displayString
                k to (label?.invoke(k) ?: k)
            }
            if (key !in groups) {
                order.add(key)
                groups[key] = lab to mutableListOf()
            }
            groups.getValue(key).second.add(r)
        }
        val options = f?.options
        if (f != null && f.type == FieldType.ENUM && options != null) {
            order = (options.filter { it in groups } + order.filter { it !in options }).toMutableList()
        } else if (f?.type?.isDate == true) {
            order.sort()
        }
        return order.mapNotNull { k -> groups[k]?.let { Bucket(k, it.first, aggregate(aggregate, field, it.second)) } }
    }

    internal fun dateBucket(d: Instant, bucket: String, locale: Locale?, firstDayOfWeek: DayOfWeek): Pair<String, String> {
        val day = LocalDate.ofInstant(d, FieldCoding.zone)
        val ja = locale?.let { it.language == "ja" } ?: true
        fun formatted(date: LocalDate, pattern: String) = DateTimeFormatter.ofPattern(pattern, locale ?: Locale.getDefault()).format(date)
        return when (bucket) {
            "month" -> {
                val key = "%04d-%02d".format(Locale.ROOT, day.year, day.monthValue)
                key to if (ja) "${day.monthValue}月" else formatted(day, "MMM")
            }
            "week" -> {
                val start = day.with(TemporalAdjusters.previousOrSame(firstDayOfWeek))
                FieldCoding.dayString(start) to if (ja) "${start.monthValue}/${start.dayOfMonth}週" else formatted(start, "M/d")
            }
            else -> FieldCoding.dayString(day) to if (ja) "${day.monthValue}/${day.dayOfMonth}" else formatted(day, "M/d")
        }
    }
}
