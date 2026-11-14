package jp.kz2601.metamon.dsl.records

import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.Direction
import jp.kz2601.metamon.dsl.TableColumnAggregate
import jp.kz2601.metamon.dsl.expr.SwiftNumbers

/**
 * `table.groupBy`(#261): 同じ値のレコードを 1 行にまとめ、集計列(count|sum|avg|min|max)を出す(iOS の Records/TableGrouping.swift)。
 * 例: 店ごとの平均点・件数のランキング。chart.groupBy と同じく端末で集計する。
 */
object TableGrouping {
    val aggregates: Set<String> = setOf("count", "sum", "avg", "min", "max")

    /**
     * @param key グループの値(文字列化・前後の空白は無視)
     * @param sample グループ先頭のレコード(グループ名の表示に使う)
     * @param values columns と同じ並び。グループ名の列・値の無い avg/min/max は null
     */
    data class Row(val key: String, val sample: Record, val values: List<Double?>)

    private val TableColumnAggregate?.op: String? get() = this?.wire?.takeIf { it in aggregates }

    /** groupBy があり集計列が 1 つ以上ある時だけグループ表示(それ以外はレコード 1 件 = 1 行の表)。 */
    fun isGrouped(c: Component): Boolean {
        if (c.groupBy.isNullOrEmpty()) return false
        return c.columns.any { it.aggregate.op != null }
    }

    /** グループ化 → 集計 → sort → limit。値が空のレコードはどのグループにも入れない。 */
    fun rows(c: Component, records: List<Record>): List<Row> {
        val groupBy = c.groupBy?.takeIf { it.isNotEmpty() } ?: return emptyList()
        val groups = LinkedHashMap<String, MutableList<Record>>()
        for (r in records) {
            val key = r[groupBy].displayString.trim()
            if (key.isEmpty()) continue
            groups.getOrPut(key) { mutableListOf() }.add(r)
        }
        val rows = groups.map { (key, members) ->
            Row(key, members[0], c.columns.map { col -> col.aggregate.op?.let { aggregate(it, col.field, members) } })
        }
        val (index, ascending) = sortKey(c)
        val cmp = Comparator<Row> { a, b ->
            if (index != null) {
                val x = a.values[index]
                val y = b.values[index]
                when {
                    x != null && y != null && x != y -> return@Comparator if (ascending) x.compareTo(y) else y.compareTo(x)
                    x != null && y == null -> return@Comparator -1 // 値なしは最後
                    x == null && y != null -> return@Comparator 1
                }
                return@Comparator nameCompare(a.key, b.key)
            }
            if (ascending) nameCompare(a.key, b.key) else nameCompare(b.key, a.key)
        }
        val sorted = rows.sortedWith(cmp)
        val limit = c.limit
        return if (limit != null && limit > 0 && sorted.size > limit) sorted.take(limit) else sorted
    }

    /**
     * 並べる列: sort.field と同じ field の集計列 → 無ければ groupBy の field ならグループ名。
     * sort 省略 = 最初の集計列の降順。index が null ならグループ名で並べる。
     */
    fun sortKey(c: Component): Pair<Int?, Boolean> {
        val firstAgg = c.columns.indexOfFirst { it.aggregate.op != null }.takeIf { it >= 0 }
        val sort = c.sort ?: return firstAgg to false
        val ascending = sort.direction == Direction.ASC
        val i = c.columns.indexOfFirst { it.field == sort.field && it.aggregate.op != null }
        if (i >= 0) return i to ascending
        if (sort.field == c.groupBy) return null to ascending
        return firstAgg to ascending
    }

    /** avg / min / max は数値が 1 つも無ければ null(0 と区別して「—」)。 */
    fun aggregate(op: String, field: String, records: List<Record>): Double? {
        if ((op == "avg" || op == "min" || op == "max") && records.none { it[field].doubleValue != null }) return null
        return Aggregator.aggregate(op, field, records)
    }

    /** 数同士は数値、それ以外は Finder 風(数字の並びは数として・大小無視)。iOS の `localizedStandardCompare`。 */
    fun nameCompare(a: String, b: String): Int {
        val x = SwiftNumbers.parseDouble(a)
        val y = SwiftNumbers.parseDouble(b)
        if (x != null && y != null) return x.compareTo(y)
        return naturalCompare(a, b)
    }

    internal fun naturalCompare(a: String, b: String): Int {
        var i = 0
        var j = 0
        while (i < a.length && j < b.length) {
            val ca = a[i]
            val cb = b[j]
            if (ca.isDigit() && cb.isDigit()) {
                var ei = i
                while (ei < a.length && a[ei].isDigit()) ei++
                var ej = j
                while (ej < b.length && b[ej].isDigit()) ej++
                val na = a.substring(i, ei).trimStart('0')
                val nb = b.substring(j, ej).trimStart('0')
                if (na.length != nb.length) return na.length.compareTo(nb.length)
                val c = na.compareTo(nb)
                if (c != 0) return c
                i = ei
                j = ej
            } else {
                val c = ca.lowercaseChar().compareTo(cb.lowercaseChar())
                if (c != 0) return c
                i++
                j++
            }
        }
        return (a.length - i).compareTo(b.length - j)
    }
}
