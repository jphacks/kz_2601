package jp.kz2601.metamon.desktop

import jp.kz2601.metamon.desktop.i18n.Strings
import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.expr.ExprEvaluator
import jp.kz2601.metamon.dsl.expr.ExprValue
import jp.kz2601.metamon.dsl.ExprNumber
import jp.kz2601.metamon.dsl.records.Aggregator
import jp.kz2601.metamon.dsl.records.Entity
import jp.kz2601.metamon.dsl.records.FieldCoding
import jp.kz2601.metamon.dsl.records.FieldType
import jp.kz2601.metamon.dsl.records.JSONValue
import jp.kz2601.metamon.dsl.records.Record
import jp.kz2601.metamon.dsl.records.RecordQuery
import jp.kz2601.metamon.dsl.records.Track
import jp.kz2601.metamon.dsl.records.entityModels
import jp.kz2601.metamon.dsl.targetExpr
import jp.kz2601.metamon.dsl.targetNumber
import java.math.BigDecimal
import java.math.RoundingMode
import java.util.Locale

// stat / progressBar / gauge / chart の値。Android の renderer/components/lists/ListValues.kt と同じ規則を :dsl の
// Aggregator・ExprEvaluator・RecordQuery で出す(読み込み中・取得失敗の状態はモックなので無い)。:renderer-core(#1739)で共有にする。

/** 1 画面ぶんの記録と評価器。 */
class LeafData(val entities: List<Entity>, source: (String) -> List<Record>, val uid: String? = "me", val labels: DataLabels = DataLabels.JAPANESE) {
    val evaluator = ExprEvaluator(entities, source)
    val locale: Locale = Locale.JAPAN

    fun entity(id: String?): Entity? = id?.let { i -> entities.firstOrNull { it.id == i } }

    fun filtered(c: Component): List<Record> = RecordQuery.filtered(c, entity(c.entity), evaluator, uid)

    companion object {
        fun of(dsl: AppDSL, store: MockStore): LeafData {
            val models = dsl.entityModels()
            return LeafData(models, { id -> store.records(id, models.firstOrNull { it.id == id }) })
        }
    }
}

/** `stat`: (書式後の文字, 評価できない)。 */
object StatValue {
    /** count の単位(「2 件」/「2 items」)。iOS の `L("件")`・Android の `lists_count_unit` と同じ文言。 */
    val countUnit: String get() = Strings["lists_count_unit"]

    fun isValueMode(c: Component): Boolean = c.aggregate == null && c.value != null

    fun resolve(c: Component, data: LeafData): Pair<String, Boolean> {
        if (isValueMode(c)) {
            val expr = RecordQuery.expr(c.value) ?: return FieldCoding.UNAVAILABLE to true
            val v = data.evaluator.evaluate(expr) ?: return FieldCoding.UNAVAILABLE to true
            return when (v) {
                is ExprValue.Number -> Aggregator.format(v.value, c.format?.wire, c.unit, data.locale) to false
                else -> v.text to false
            }
        }
        val entity = data.entity(c.entity)
        val rows = data.filtered(c)
        val op = c.aggregate?.wire
        if (Track.isTrackAggregate(op)) return Track.format(op, Track.aggregate(op, rows, c.field, c.by)) to false
        if (Aggregator.isPick(op)) {
            return (Aggregator.pickText(op, c.field, c.by, c.format?.wire, entity, rows, data.locale) ?: FieldCoding.UNAVAILABLE) to false
        }
        val fieldDef = c.field?.let { entity?.field(it) }
        val value = Aggregator.aggregate(op, c.field, rows)
        val format = c.format?.wire ?: if (op == "count") "plain" else if (fieldDef?.type == FieldType.CURRENCY) "currency" else null
        return Aggregator.format(value, format, if (op == "count") countUnit else fieldDef?.unit, data.locale) to false
    }
}

/** `progressBar`: 比率(1 を超えてもよい)と「42%」。 */
object ProgressValue {
    fun literal(c: Component): Double? = (c.value as? ExprNumber)?.value

    fun target(c: Component, data: LeafData): Double? {
        val t = c.targetNumber ?: c.targetExpr?.let { data.evaluator.evaluate(it)?.number } ?: return null
        return t.takeIf { it.isFinite() && it > 0 }
    }

    fun missingTarget(c: Component, data: LeafData): Boolean =
        literal(c) == null && c.entity != null && c.targetNumber == null && c.targetExpr != null && target(c, data) == null

    fun ratio(c: Component, data: LeafData): Double {
        literal(c)?.let { return it }
        if (c.entity == null) return 0.0
        val target = target(c, data) ?: return 0.0
        return Aggregator.aggregate(c.aggregate?.wire ?: "sum", c.field, data.filtered(c)) / target
    }

    /** tone 省略時は 0〜100% に収める。tone 指定時は実比率(999% で頭打ち)。 */
    fun percentText(raw: Double, tone: String?): String {
        val shown = if (tone == null) raw.coerceIn(0.0, 1.0) else minOf(maxOf(0.0, raw), 9.99)
        return round0(shown * 100) + "%" + if (tone != null && raw > 9.99) "+" else ""
    }

    fun percent(c: Component, data: LeafData): String =
        if (missingTarget(c, data)) FieldCoding.UNAVAILABLE else percentText(ratio(c, data), c.tone?.wire)

    private fun round0(x: Double): String = if (!x.isFinite()) x.toString() else BigDecimal(x).setScale(0, RoundingMode.HALF_EVEN).toPlainString()
}

/** `gauge`: (値, 単位)。 */
object GaugeValue {
    fun value(c: Component, data: LeafData): Pair<Double, String?> {
        ProgressValue.literal(c)?.let { return it to null }
        val entityId = c.entity ?: return 0.0 to null
        val entity = data.entity(entityId)
        val op = c.aggregate?.wire ?: "sum"
        val unit = if (op == "count") StatValue.countUnit else c.field?.let { entity?.field(it)?.unit }
        return Aggregator.aggregate(op, c.field, data.filtered(c)) to unit
    }
}

/** `chart`: 棒・線・扇の元。ref の groupBy の見出しはまだ id のまま。 */
object ChartBuckets {
    fun buckets(c: Component, data: LeafData): List<Aggregator.Bucket> =
        Aggregator.buckets(c.groupBy, c.bucket?.wire, c.aggregate?.wire, c.field, data.entity(c.entity), data.filtered(c), null, data.locale)
}

/** モックの文字列の値 → 記録の値(数の型は数・真偽は真偽)。 */
fun typedValue(raw: String, type: FieldType?): JSONValue = when {
    type?.isNumeric == true -> raw.replace(",", "").toDoubleOrNull()?.let { JSONValue.Num(it) } ?: JSONValue.Str(raw)
    type == FieldType.BOOLEAN -> JSONValue.Bool(raw == "true")
    else -> JSONValue.Str(raw)
}
