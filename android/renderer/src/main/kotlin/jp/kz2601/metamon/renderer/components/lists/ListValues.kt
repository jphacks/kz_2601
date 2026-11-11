package jp.kz2601.metamon.renderer.components.lists

import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.ExprNumber
import jp.kz2601.metamon.dsl.ListItem
import jp.kz2601.metamon.dsl.Resets
import jp.kz2601.metamon.dsl.TableColumn
import jp.kz2601.metamon.dsl.expr.DailyCheck
import jp.kz2601.metamon.dsl.expr.Expr
import jp.kz2601.metamon.dsl.expr.ExprValue
import jp.kz2601.metamon.dsl.records.Aggregator
import jp.kz2601.metamon.dsl.records.Entity
import jp.kz2601.metamon.dsl.records.Field
import jp.kz2601.metamon.dsl.records.FieldCoding
import jp.kz2601.metamon.dsl.records.FieldType
import jp.kz2601.metamon.dsl.records.JSONValue
import jp.kz2601.metamon.dsl.records.Record
import jp.kz2601.metamon.dsl.records.RecordQuery
import jp.kz2601.metamon.dsl.records.Sort
import jp.kz2601.metamon.dsl.records.TableGrouping
import jp.kz2601.metamon.dsl.records.Track
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonNull
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.doubleOrNull
import java.math.BigDecimal
import java.math.RoundingMode
import java.text.DecimalFormat
import java.text.DecimalFormatSymbols
import java.time.Instant
import java.util.Locale

// 一覧・集計・グラフの Leaf が出す値(iOS の各 Leaf の static な resolve)。View と適合テストの描画の木が同じ関数を使う。
// 値の文言(「件」「X の合計」など)は [ValueLabels] で外から渡す(画面は端末の言語・適合テストは iOS の日本語)。

/** `list`: 表示する行(filter → sort → 検索)。iOS の `ListLeaf.rows`。 */
object ListRows {
    fun rows(c: Component, data: LeafData, query: String?): List<Record> {
        val entity = data.entity(c.entity)
        val sorted = Aggregator.sorted(data.filtered(c), RecordQuery.sort(c.sort), entity)
        return searched(c, sorted, entity, data, query)
    }

    /** `searchField`(同画面の searchBar.field)の文字列で title / subtitle を部分一致(大小無視)で絞る。 */
    fun searched(c: Component, rows: List<Record>, entity: Entity?, data: LeafData, query: String?): List<Record> {
        if (c.searchField == null) return rows
        val q = query?.trim()?.takeIf { it.isNotEmpty() } ?: return rows
        val fields = listOfNotNull(c.item?.title ?: entity?.titleField, c.item?.subtitle)
        return rows.filter { r -> fields.any { data.display(r[it], entity?.field(it)).contains(q, ignoreCase = true) } }
    }

    data class Texts(val title: String, val subtitle: String?, val trailing: String?)

    /** 行に出す文字(title / subtitle / trailing・空は null)。iOS の `ListRow.texts`。 */
    fun texts(item: ListItem?, record: Record, entity: Entity?, data: LeafData): Texts {
        fun t(f: String?) = f?.let { data.display(record[it], entity?.field(it)) }
        return Texts(
            t(item?.title ?: entity?.displayTitleField) ?: "",
            t(item?.subtitle)?.takeIf { it.isNotEmpty() },
            t(item?.trailing)?.takeIf { it.isNotEmpty() },
        )
    }
}

/** `list.item.toggle`(#108)の判定。iOS の `ListToggle`。 */
object ListToggle {
    /** チェック丸に使う field(boolean・computed でない。daily なら date field)。条件を満たさなければ null。 */
    fun field(item: ListItem?, entity: Entity?): Field? {
        val id = item?.toggle ?: return null
        val f = entity?.field(id) ?: return null
        if (f.isComputed) return null
        return f.takeIf { it.type == if (isDaily(item)) FieldType.DATE else FieldType.BOOLEAN }
    }

    /** 毎日リセットのチェック(#132)。 */
    fun isDaily(item: ListItem?): Boolean = item?.resets == Resets.DAILY

    /** 表示上の ON/OFF(保存待ちの値を優先)。bool 以外の保存値("true" / 1)も寛容に読む。 */
    fun isOn(record: Record, field: String, daily: Boolean, overrides: Map<String, Boolean>, today: Instant): Boolean {
        overrides[record.id]?.let { return it }
        if (daily) return DailyCheck.isDone(record[field], today)
        return when (val v = record[field]) {
            is JSONValue.Bool -> v.value
            is JSONValue.Num -> v.value != 0.0
            is JSONValue.Str -> v.value == "true"
            else -> false
        }
    }

    /** 保存する差分。daily は「最後にやった日」+ 押した日の履歴(#172)、それ以外は bool 1 つ。 */
    fun patch(record: Record, field: String, on: Boolean, daily: Boolean, today: Instant): Map<String, JSONValue> =
        if (daily) DailyCheck.patch(record, field, on, today) else mapOf(field to JSONValue.Bool(on))
}

/** `table`: 見出しと行の文字。iOS の `TableLeaf.cellRows` / `header`。 */
object TableCells {
    data class Row(val id: String, val texts: List<String>)

    fun rows(c: Component, data: LeafData): List<Row> {
        val entity = data.entity(c.entity)
        if (TableGrouping.isGrouped(c)) {
            return TableGrouping.rows(c, data.filtered(c)).map { row ->
                Row(row.key, c.columns.mapIndexed { i, col ->
                    if (col.aggregate == null) {
                        data.display(row.sample[col.field], entity?.field(col.field))
                    } else {
                        aggregateText(row.values[i], col.aggregate?.wire, entity?.field(col.field), data)
                    }
                })
            }
        }
        val all = Aggregator.sorted(data.filtered(c), RecordQuery.sort(c.sort), entity)
        val limit = c.limit ?: 0
        val rows = if (limit > 0) all.take(limit) else all
        return rows.map { r -> Row(r.id, c.columns.map { data.display(r[it.field], entity?.field(it.field)) }) }
    }

    /** count は件数、sum/min/max は field の書式(¥・単位)、avg は小数 1 桁(currency は円の整数)。 */
    private fun aggregateText(value: Double?, op: String?, field: Field?, data: LeafData): String {
        value ?: return FieldCoding.UNAVAILABLE
        if (op == "count") return FieldCoding.plainNumber(value)
        val shown = if (op == "avg" && field?.type != FieldType.CURRENCY) swiftRounded(value * 10) / 10 else value
        field ?: return FieldCoding.plainNumber(shown)
        return data.display(JSONValue.Num(shown), field)
    }

    fun header(col: TableColumn, entity: Entity?, labels: ValueLabels): String {
        col.title?.let { return it }
        val name = entity?.field(col.field)?.name ?: col.field
        return when (col.aggregate?.wire) {
            "count" -> labels.count
            "sum" -> labels.sum.format(name)
            "avg" -> labels.avg.format(name)
            "min" -> labels.min.format(name)
            "max" -> labels.max.format(name)
            else -> name
        }
    }

    fun isNumericColumn(col: TableColumn, entity: Entity?): Boolean =
        col.aggregate != null || entity?.field(col.field)?.type?.isNumeric == true
}

/** `timeline`: 日付の新しい順・limit。iOS の `TimelineLeaf.rows`。 */
object TimelineRows {
    fun rows(c: Component, data: LeafData): List<Record> {
        val entity = data.entity(c.entity)
        val all = Aggregator.sorted(data.filtered(c), Sort(c.dateField ?: "", "desc"), entity)
        val limit = c.limit ?: 0
        return if (limit > 0) all.take(limit) else all
    }
}

/** `stat`: 表示する値(書式後の文字, 取得失敗)。iOS の `StatLeaf.resolve`。 */
object StatValue {
    data class Resolved(val text: String, val unavailable: Boolean)

    /** `value`(Expr)で値を出すモード(aggregate 指定が優先)。 */
    fun isValueMode(c: Component): Boolean = c.aggregate == null && c.value != null

    fun resolve(c: Component, data: LeafData, locale: Locale = Locale.getDefault()): Resolved =
        if (isValueMode(c)) exprValue(c, data, locale) else aggregateValue(c, data, locale)

    private fun aggregateValue(c: Component, data: LeafData, locale: Locale): Resolved {
        val entity = data.entity(c.entity)
        val rows = data.filtered(c)
        val op = c.aggregate?.wire
        val fieldDef = c.field?.let { entity?.field(it) }
        if (Track.isTrackAggregate(op)) {
            // 位置の記録(#168)を時刻順に繋いだ距離・時間(10 分以上空いたら別の記録)
            return Resolved(Track.format(op, Track.aggregate(op, rows, c.field, c.by)), false)
        }
        if (Aggregator.isPick(op)) {
            return Resolved(Aggregator.pickText(op, c.field, c.by, c.format?.wire, entity, rows, locale) ?: FieldCoding.UNAVAILABLE, false)
        }
        val value = Aggregator.aggregate(op, c.field, rows)
        // format 省略時: count → 件数、currency 型の field → ¥ と桁区切り
        val format = c.format?.wire ?: if (op == "count") "plain" else if (fieldDef?.type == FieldType.CURRENCY) "currency" else null
        return Resolved(Aggregator.format(value, format, if (op == "count") data.labels.countUnit else fieldDef?.unit, locale), false)
    }

    /** value モード(#101): 行コンテキスト無しで Expr を評価。評価できなければ「—」。 */
    private fun exprValue(c: Component, data: LeafData, locale: Locale): Resolved {
        val expr = RecordQuery.expr(c.value) ?: return Resolved(FieldCoding.UNAVAILABLE, true)
        val v = data.evaluator.evaluate(expr) ?: return Resolved(FieldCoding.UNAVAILABLE, true)
        return when (v) {
            // percent の value は比率(0〜1)。100 倍は Aggregator.format が持つ
            is ExprValue.Number -> Resolved(Aggregator.format(v.value, c.format?.wire, c.unit, locale), false)
            else -> Resolved(v.text, false)
        }
    }
}

/** `progressBar`: 比率と表示。iOS の `ProgressBarLeaf`。 */
object ProgressValue {
    enum class Level { NORMAL, SUCCESS, OVER }

    /** 色の段階(#140)。tone 省略: 100% 以上を negative / positive: 100% 以上を成功色 / negative: 100% を超えたら警告色。 */
    fun level(raw: Double, tone: String?): Level = when (tone) {
        "positive" -> if (raw >= 1) Level.SUCCESS else Level.NORMAL
        "negative" -> if (raw > 1) Level.OVER else Level.NORMAL
        else -> if (raw >= 1) Level.OVER else Level.NORMAL
    }

    /** 「42%」。tone 指定時は実比率(112%)。999% で頭打ち。 */
    fun percentText(raw: Double, tone: String?): String {
        val shown = if (tone == null) raw.coerceIn(0.0, 1.0) else minOf(maxOf(0.0, raw), 9.99)
        return cFormat0(shown * 100) + "%" + if (tone != null && raw > 9.99) "+" else ""
    }

    /** `value` のリテラルの数(式なら null)。 */
    fun literal(c: Component): Double? = (c.value as? ExprNumber)?.value

    /** `target` が式({"setting"} など)か。 */
    fun targetExpr(c: Component): Expr? {
        val t = c.target ?: return null
        if (t is JsonPrimitive && !t.isString && t.doubleOrNull != null) return null
        if (t == JsonNull) return null
        return runCatching { Expr.from(t) }.getOrNull()
    }

    private fun targetLiteral(t: JsonElement?): Double? = (t as? JsonPrimitive)?.takeIf { !it.isString }?.doubleOrNull

    /** entity モードの分母: 数値の `target`、または式の今の値。0 以下・空は null。 */
    fun target(c: Component, data: LeafData): Double? {
        val t = targetLiteral(c.target) ?: targetExpr(c)?.let { data.evaluator.evaluate(it)?.number } ?: return null
        return t.takeIf { it.isFinite() && it > 0 }
    }

    /** 目標を式で読むのに、その値が空・0 以下。 */
    fun missingTarget(c: Component, data: LeafData): Boolean =
        literal(c) == null && c.entity != null && targetLiteral(c.target) == null && targetExpr(c) != null && target(c, data) == null

    /** 比率(1 を超えてもよい)。 */
    fun ratio(c: Component, data: LeafData): Double {
        literal(c)?.let { return it }
        if (c.entity == null) return 0.0
        val target = target(c, data) ?: return 0.0
        return Aggregator.aggregate(c.aggregate?.wire ?: "sum", c.field, data.filtered(c)) / target
    }
}

/** `gauge`: (値, 単位)。iOS の `GaugeLeaf.value`。 */
object GaugeValue {
    fun value(c: Component, data: LeafData): Pair<Double, String?> {
        ProgressValue.literal(c)?.let { return it to null }
        val entityId = c.entity ?: return 0.0 to null
        val entity = data.entity(entityId)
        val op = c.aggregate?.wire ?: "sum"
        val unit = if (op == "count") data.labels.countUnit else c.field?.let { entity?.field(it)?.unit }
        return Aggregator.aggregate(op, c.field, data.filtered(c)) to unit
    }
}

/** `chart`: 棒・点・扇の元。iOS の `ChartLeaf.buckets`。 */
object ChartBuckets {
    fun buckets(c: Component, data: LeafData, locale: Locale? = null): List<Aggregator.Bucket> {
        val entity = data.entity(c.entity)
        val groupField = c.groupBy?.let { entity?.field(it) }
        // ref の groupBy は id で束ねるので、軸・凡例は参照先の見出しにする(#235)
        val label: ((String) -> String)? = if (groupField?.type == FieldType.REF) { id -> data.display(JSONValue.Str(id), groupField) } else null
        return Aggregator.buckets(c.groupBy, c.bucket?.wire, c.aggregate?.wire, c.field, entity, data.filtered(c), label, locale)
    }

    /** x 軸ラベル: 6 文字を超えたら 5 文字 + …(読み上げは元の文字列のまま)。 */
    fun axisLabel(s: String): String = if (s.codePointCount(0, s.length) > 6) s.take(5) + "…" else s
}

/** グラフの値の読み取り(押した所の値・読み上げの要約)。iOS の `ChartReadout`。 */
object ChartReadout {
    /** 整数はそのまま(3 桁区切り)、小数は 1 桁まで。 */
    fun format(value: Double, locale: Locale = Locale.getDefault()): String {
        val f = DecimalFormat("#,##0.#", DecimalFormatSymbols.getInstance(locale))
        f.roundingMode = RoundingMode.HALF_EVEN
        return f.format(value)
    }

    /** 「3月: 12」 */
    fun line(label: String, value: Double, locale: Locale = Locale.getDefault()): String = "$label: ${format(value, locale)}"

    /** 扇の累積値 → その扇の位置。 */
    fun index(cumulative: Double, values: List<Double>): Int? {
        var sum = 0.0
        values.forEachIndexed { i, v ->
            sum += v
            if (cumulative <= sum) return i
        }
        return if (values.isEmpty()) null else values.size - 1
    }

    /** 「4 項目。最大は 3月 の 12、最小は 1月 の 2」。1 項目なら「1 項目。3月 の 12」。 */
    fun summary(buckets: List<Aggregator.Bucket>, labels: ValueLabels, locale: Locale = Locale.getDefault()): String {
        val maxB = buckets.maxByOrNull { it.value } ?: return labels.noData
        val minB = buckets.minByOrNull { it.value } ?: return labels.noData
        val count = buckets.size
        if (count == 1) return labels.chartOne.format(locale, count, maxB.label, format(maxB.value, locale))
        return labels.chartMany.format(locale, count, maxB.label, format(maxB.value, locale), minB.label, format(minB.value, locale))
    }
}

/** Swift の `rounded()`(0.5 は 0 から遠い方)。 */
internal fun swiftRounded(x: Double): Double = if (x >= 0) kotlin.math.floor(x + 0.5) else -kotlin.math.floor(-x + 0.5)

/** C の printf("%.0f")(2 進の値を偶数丸め)。 */
internal fun cFormat0(x: Double): String {
    if (!x.isFinite()) return x.toString()
    return BigDecimal(x).setScale(0, RoundingMode.HALF_EVEN).toPlainString()
}
