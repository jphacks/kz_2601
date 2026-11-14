package jp.kz2601.metamon.dsl.expr

import jp.kz2601.metamon.dsl.records.Aggregator
import jp.kz2601.metamon.dsl.records.Entity
import jp.kz2601.metamon.dsl.records.Field
import jp.kz2601.metamon.dsl.records.FieldCoding
import jp.kz2601.metamon.dsl.records.FieldType
import jp.kz2601.metamon.dsl.records.Filter
import jp.kz2601.metamon.dsl.records.JSONValue
import jp.kz2601.metamon.dsl.records.Record
import jp.kz2601.metamon.dsl.records.SingleRecord
import java.time.Instant
import java.time.LocalDate
import java.time.ZoneId
import java.time.ZonedDateTime
import java.time.temporal.ChronoUnit
import kotlin.math.abs
import kotlin.math.floor
import kotlin.math.ceil
import kotlin.math.sqrt

/** 式の評価値。評価できない(ゼロ除算・型不一致・参照なし)は null(表示は「—」)。iOS の `ExprValue`。 */
sealed class ExprValue {
    class Number(val value: Double) : ExprValue() {
        override fun equals(other: Any?): Boolean = other is Number && value == other.value
        override fun hashCode(): Int = if (value == 0.0) 0 else value.hashCode()
        override fun toString(): String = "Number($value)"
    }

    data class Str(val value: String) : ExprValue()
    data class Bool(val value: Boolean) : ExprValue()
    data class Date(val value: Instant) : ExprValue()

    /** 数値として読む(数の文字列も・カンマは除く)。真偽値・日付は null。 */
    val number: Double?
        get() = when (this) {
            is Number -> value
            is Str -> Aggregator.parseNumber(value)
            else -> null
        }

    val truthy: Boolean
        get() = when (this) {
            is Number -> value != 0.0
            is Str -> value.isNotEmpty() && value != "false"
            is Bool -> value
            is Date -> true
        }

    /** concat / toString / len が見る文字列(日付は端末のタイムゾーンの "YYYY-MM-DD")。 */
    val text: String
        get() = when (this) {
            is Number -> JSONValue.numberText(value)
            is Str -> value
            is Bool -> if (value) "true" else "false"
            is Date -> FieldCoding.formatDay(value)
        }

    /** Record.data に載せる形(date は field 型に合わせて "YYYY-MM-DD" / RFC3339)。 */
    fun json(field: Field? = null): JSONValue = when (this) {
        is Number -> JSONValue.Num(value)
        is Str -> JSONValue.Str(value)
        is Bool -> JSONValue.Bool(value)
        is Date -> JSONValue.Str(if (field?.type == FieldType.DATETIME) FieldCoding.formatDatetime(value) else FieldCoding.formatDay(value))
    }
}

/**
 * [Expr] の評価器(#101)。iOS の Logic/ExprEvaluator.swift と同じ意味。
 * 1 画面の描画 1 回につき 1 つ作り、computed / agg の結果をメモする。
 * `{"ref": ...}` は行([Row])から読む。agg の filter.value の式は外側の行で評価する。
 *
 * @param zone 端末のタイムゾーン(iOS の Calendar.current)。既定は [FieldCoding.zone]。
 */
class ExprEvaluator(
    entities: List<Entity>,
    private val source: (String) -> List<Record>,
    private val now: Instant = DSLClock.now(),
    private val zone: ZoneId = FieldCoding.zone,
) {
    data class Row(
        val entity: Entity?,
        val record: Record,
        /** 保存済みレコード(id でメモしてよい)か。フォームのドラフトは false。 */
        val memoizable: Boolean = true,
    )

    private val entities: Map<String, Entity>
    private val singles: List<Entity>

    init {
        val map = LinkedHashMap<String, Entity>()
        val s = mutableListOf<Entity>()
        for (e in entities) {
            if (e.id in map) continue
            map[e.id] = e
            if (e.single) s.add(e)
        }
        this.entities = map
        this.singles = s
    }

    private val computedMemo = HashMap<String, ExprValue?>()
    private val aggMemo = HashMap<AggExpr, ExprValue?>()
    private val viewMemo = HashMap<String, List<Record>>()
    private val inProgress = HashSet<String>()
    private var depth = 0

    companion object {
        /** 入れ子・computed の連鎖の上限(循環参照の防波堤)。 */
        const val MAX_DEPTH = 48

        /** addDays の日数の上限(約 273 年)。超えたら null。 */
        const val MAX_ADD_DAYS = 100_000

        /** addMinutes の分数の上限(MAX_ADD_DAYS 日ぶん)。 */
        const val MAX_ADD_MINUTES = MAX_ADD_DAYS * 1440

        /** 評価できる op(schema の `ExprOp.op` enum と一致させる)。 */
        val supportedOps: Set<String> = setOf(
            "add", "sub", "mul", "div", "mod", "neg", "abs", "sqrt", "pow", "round", "floor", "ceil", "min", "max", "clamp",
            "eq", "neq", "gt", "gte", "lt", "lte", "and", "or", "not", "concat", "len",
            "now", "today", "daysBetween", "daysUntil", "addDays", "addMinutes", "nextOccurrence", "hoursBetween", "year", "month", "day", "weekday",
            "toNumber", "toString", "coalesce", "streak", "doneInDays",
        )

        /** 保存値 → 評価値。date/datetime field の文字列は日付にする。null・配列・object は null。 */
        fun value(v: JSONValue, field: Field?): ExprValue? = when (v) {
            is JSONValue.Num -> ExprValue.Number(v.value)
            is JSONValue.Bool -> ExprValue.Bool(v.value)
            is JSONValue.Str -> {
                val d = if (field?.type?.isDate == true) FieldCoding.date(v) else null
                if (d != null) ExprValue.Date(d) else ExprValue.Str(v.value)
            }
            else -> null
        }

        /** "HH:mm" / "H:mm" / "HH:mm:ss" → 0:00 からの秒(00:00〜23:59:59)。形式外は null。 */
        fun timeOfDay(s: String): Double? {
            val parts = s.trim { it == '\t' || Character.isSpaceChar(it) }.split(":")
            if (parts.size !in 2..3) return null
            if (!parts.all { p -> p.isNotEmpty() && p.length <= 2 && p.all { it in '0'..'9' } }) return null
            val h = parts[0].toInt()
            val m = parts[1].toInt()
            val sec = if (parts.size == 3) parts[2].toInt() else 0
            if (h !in 0..23 || m !in 0..59 || sec !in 0..59) return null
            return (h * 3600 + m * 60 + sec).toDouble()
        }

        /** 0:00 からの秒(24 時間で一周)→ "HH:mm"(秒が 0 でなければ "HH:mm:ss")。 */
        fun clockString(secs: Double): String {
            val t = ((secs % 86_400) + 86_400) % 86_400
            val i = t.toInt()
            return if (i % 60 == 0) "%02d:%02d".format(java.util.Locale.ROOT, i / 3600, i % 3600 / 60)
            else "%02d:%02d:%02d".format(java.util.Locale.ROOT, i / 3600, i % 3600 / 60, i % 60)
        }

        fun equal(a: ExprValue?, b: ExprValue?): Boolean {
            if (a == null || b == null) return a == null && b == null
            if (a is ExprValue.Number && b is ExprValue.Number) return a.value == b.value
            if (a is ExprValue.Date && b is ExprValue.Date) return a.value == b.value
            if (a is ExprValue.Date && b is ExprValue.Str) return dateEqualsString(a.value, b.value, a, b)
            if (a is ExprValue.Str && b is ExprValue.Date) return dateEqualsString(b.value, a.value, a, b)
            val x = a.number
            val y = b.number
            if (x != null && y != null) return x == y
            return a.text == b.text
        }

        private fun dateEqualsString(d: Instant, s: String, a: ExprValue, b: ExprValue): Boolean {
            // 日付と日付の文字列は日付として比べる(読めない文字列は文字列として)
            FieldCoding.date(JSONValue.Str(s))?.let { return d == it }
            return a.text == b.text
        }

        /** -1 / 0 / 1。比べられなければ null。 */
        fun compare(a: ExprValue, b: ExprValue): Int? {
            if (a is ExprValue.Date && b is ExprValue.Date) return a.value.compareTo(b.value).coerceIn(-1, 1)
            if (a is ExprValue.Date && b is ExprValue.Str) return FieldCoding.date(JSONValue.Str(b.value))?.let { a.value.compareTo(it).coerceIn(-1, 1) }
            if (a is ExprValue.Str && b is ExprValue.Date) return FieldCoding.date(JSONValue.Str(a.value))?.let { it.compareTo(b.value).coerceIn(-1, 1) }
            if (a is ExprValue.Str && b is ExprValue.Str && (Aggregator.parseNumber(a.value) == null || Aggregator.parseNumber(b.value) == null)) {
                // NSString.compare(オプション無し)= UTF-16 の並び
                return a.value.compareTo(b.value).coerceIn(-1, 1)
            }
            val x = a.number ?: return null
            val y = b.number ?: return null
            return Aggregator.order(x, y)
        }
    }

    // MARK: - Public

    fun evaluate(expr: Expr, row: Row? = null): ExprValue? {
        if (depth >= MAX_DEPTH) return null
        depth++
        try {
            return when (expr) {
                is Expr.Literal -> value(expr.value, null)
                is Expr.Ref -> row?.let { fieldValue(expr.id, it) }
                is Expr.Agg -> aggregate(expr.agg, row)
                is Expr.Op -> apply(expr.name, expr.args, row)
                is Expr.Cond -> if (evaluate(expr.cond, row)?.truthy == true) evaluate(expr.then, row) else evaluate(expr.otherwise, row)
                is Expr.Setting -> setting(expr.id)
            }
        } finally {
            depth--
        }
    }

    /** `{"setting": id}`(#267): field を持つ最初の single の entity の 1 件の値。空なら defaultValue。 */
    private fun setting(id: String): ExprValue? {
        if (id.startsWith("$") || id == "id") return null
        for (entity in singles) {
            val field = entity.fields.firstOrNull { it.id == id } ?: continue
            val record = SingleRecord.pick(source(entity.id)) ?: Record(id = "", data = SingleRecord.defaults(entity))
            val v = fieldValue(id, Row(entity, record))
            if (v == null && !field.isComputed) SingleRecord.defaultValue(field)?.let { return value(it, field) }
            return v
        }
        return null
    }

    /** 行のフィールド値。"id" はレコード id、computed は評価(メモ)、それ以外は保存値。"<refField>.<fieldId>" は参照先の値(1 段)。 */
    fun fieldValue(fieldId: String, row: Row): ExprValue? {
        if (fieldId == "id") return ExprValue.Str(row.record.id)
        val dot = fieldId.indexOf('.')
        if (dot >= 0) return lookup(fieldId.substring(0, dot), fieldId.substring(dot + 1), row)
        val field = row.entity?.field(fieldId)
        val expr = field?.computed ?: return value(row.record[fieldId], field)
        val key = "${row.entity?.id ?: ""}/${row.record.id}/$fieldId"
        if (row.memoizable && computedMemo.containsKey(key)) return computedMemo[key]
        if (key in inProgress) return null // 循環
        inProgress.add(key)
        val v = evaluate(expr, row)
        inProgress.remove(key)
        if (row.memoizable) computedMemo[key] = v
        return v
    }

    private fun lookup(refField: String, target: String, row: Row): ExprValue? {
        if (target.contains('.')) return null
        val f = row.entity?.field(refField) ?: return null
        if (f.type != FieldType.REF) return null
        val refEntity = f.refEntity ?: return null
        val id = row.record[refField].stringValue?.takeIf { it.isNotEmpty() } ?: return null
        val record = source(refEntity).firstOrNull { it.id == id } ?: return null
        return fieldValue(target, Row(entities[refEntity], record))
    }

    /** computed を data に合成した表示用レコード(保存はしない)。評価できない computed はキーを落とす。 */
    fun viewRecords(entityId: String): List<Record> {
        viewMemo[entityId]?.let { return it }
        val raw = source(entityId)
        val entity = entities[entityId]
        if (entity == null || entity.computedFields.isEmpty()) return raw
        val out = raw.map { r ->
            val row = Row(entity, r)
            val data = LinkedHashMap(r.data)
            for (f in entity.computedFields) {
                val v = fieldValue(f.id, row)?.json(f)
                if (v == null) data.remove(f.id) else data[f.id] = v
            }
            r.copy(data = data)
        }
        viewMemo[entityId] = out
        return out
    }

    /** filter.value の式を外側の行で評価してリテラルにする。評価できないものがあれば null(= どのレコードにも一致しない)。 */
    fun resolve(filters: List<Filter>, row: Row?): List<Filter>? {
        val out = mutableListOf<Filter>()
        for (f in filters) {
            val e = f.valueExpr
            if (e == null) {
                out.add(f)
                continue
            }
            val v = evaluate(e, row) ?: return null
            out.add(Filter(f.field, f.op, v.json()))
        }
        return out
    }

    /** 解決済み filter の AND。 */
    fun apply(filters: List<Filter>, entity: Entity?, records: List<Record>, row: Row? = null): List<Record> {
        val resolved = resolve(filters, row) ?: return emptyList()
        if (resolved.isEmpty()) return records
        return records.filter { r -> resolved.all { matches(it, r, entity?.field(it.field)) } }
    }

    /** Aggregator.matches + type refs の contains は削除済みの id を除いて判定(残りが 0 件なら全員に一致・#231)。 */
    private fun matches(f: Filter, record: Record, field: Field?): Boolean {
        if (f.op != "contains" || field == null || field.type != FieldType.REFS) {
            return Aggregator.matches(f, record, field, now, zone)
        }
        val ids = liveRefIds(record[f.field], field)
        return ids.isEmpty() || (f.value?.stringValue ?: "") in ids
    }

    /** type refs の保存値のうち、参照先にまだあるレコードの id。 */
    internal fun liveRefIds(v: JSONValue, field: Field): List<String> {
        val ids = Aggregator.refIds(v)
        val target = field.refEntity
        if (ids.isEmpty() || target == null) return ids
        val live = source(target).map { it.id }.toSet()
        return ids.filter { it in live }
    }

    // MARK: - agg

    private fun aggregate(a: AggExpr, row: Row?): ExprValue? {
        val filters = resolve(a.filters, row)
        val resolved = a.copy(filters = filters ?: a.filters)
        if (aggMemo.containsKey(resolved)) return aggMemo[resolved]
        val entity = entities[a.entity]
        // computed field を集計・絞り込みに使う時だけ合成済みレコードを使う
        val usesComputed = (listOfNotNull(a.field, a.by) + a.filters.map { it.field }).any { entity?.field(it)?.isComputed == true }
        val base = if (usesComputed) viewRecords(a.entity) else source(a.entity)
        val rows = when {
            filters == null -> emptyList()
            resolved.filters.isEmpty() -> base
            else -> base.filter { r -> resolved.filters.all { matches(it, r, entity?.field(it.field)) } }
        }
        val nums = a.field?.let { f -> rows.mapNotNull { it[f].doubleValue ?: it[f].stringValue?.let { s -> Aggregator.parseNumber(s) } } } ?: emptyList()
        val v: ExprValue? = when (a.op) {
            "count" -> ExprValue.Number(rows.size.toDouble())
            "sum" -> ExprValue.Number(nums.fold(0.0) { x, y -> x + y })
            "avg" -> if (nums.isEmpty()) null else ExprValue.Number(nums.fold(0.0) { x, y -> x + y } / nums.size)
            "min" -> swiftMin(nums)?.let { ExprValue.Number(it) }
            "max" -> swiftMax(nums)?.let { ExprValue.Number(it) }
            "latest", "earliest" -> a.field?.let { f ->
                Aggregator.pick(a.op, a.by, entity, rows)?.let { value(it[f], entity?.field(f)) }
            }
            else -> null
        }
        aggMemo[resolved] = v
        return v
    }

    // MARK: - op

    private fun apply(name: String, args: List<Expr>, row: Row?): ExprValue? {
        // 遅延評価が要るもの(and / or / coalesce)を先に
        when (name) {
            "and" -> return ExprValue.Bool(args.all { evaluate(it, row)?.truthy == true })
            "or" -> return ExprValue.Bool(args.any { evaluate(it, row)?.truthy == true })
            "coalesce" -> {
                for (a in args) {
                    val v = evaluate(a, row)
                    if (v != null && v != ExprValue.Str("")) return v
                }
                return null
            }
            "len" -> {
                // type refs(#231): 選んだ件数。削除済みの id は数えず、残りが 0 件なら参照先の全件数
                val first = args.firstOrNull()
                if (first is Expr.Ref && !first.id.contains('.') && row != null) {
                    val field = row.entity?.field(first.id)
                    if (field != null && field.type == FieldType.REFS) {
                        val ids = liveRefIds(row.record[first.id], field)
                        return ExprValue.Number((if (ids.isEmpty()) source(field.refEntity ?: "").size else ids.size).toDouble())
                    }
                }
            }
            "streak", "doneInDays" -> {
                // 毎日チェック(#172): 第 1 引数は resets daily の toggle field の ref。値ではなく行の履歴を読む
                val first = args.firstOrNull()
                if (first !is Expr.Ref || first.id.contains('.') || row == null) return null
                val days = DailyCheck.dates(row.record, first.id)
                if (name == "streak") return ExprValue.Number(DailyCheck.streak(days, now, zone).toDouble())
                if (args.size < 2) return null
                val n = evaluate(args[1], row)?.number ?: return null
                if (!n.isFinite()) return null
                val lastDays = SwiftNumbers.rounded(minOf(maxOf(n, -1.0), DailyCheck.MAX_HISTORY.toDouble())).toInt()
                return ExprValue.Number(DailyCheck.count(days, lastDays, now, zone).toDouble())
            }
        }
        val vals = args.map { evaluate(it, row) }
        fun num(i: Int): Double? = vals.getOrNull(i)?.number
        fun date(i: Int): Instant? = when (val v = vals.getOrNull(i)) {
            is ExprValue.Date -> v.value
            is ExprValue.Str -> FieldCoding.date(JSONValue.Str(v.value))
            else -> null
        }
        fun clockSeconds(d: Instant): Double {
            val t = ZonedDateTime.ofInstant(d, zone)
            return (t.hour * 3600 + t.minute * 60 + t.second).toDouble()
        }
        /** 時刻引数 → 0:00 からの秒。"HH:mm" 文字列、または日時の時刻部分。 */
        fun secondsOfDay(i: Int): Double? = when (val v = vals.getOrNull(i)) {
            is ExprValue.Str -> timeOfDay(v.value) ?: run {
                val d = FieldCoding.date(JSONValue.Str(v.value))
                if (d != null && v.value.contains('T')) clockSeconds(d) else null
            }
            is ExprValue.Date -> clockSeconds(v.value)
            else -> null
        }
        fun allNums(): List<Double>? {
            if (vals.isEmpty()) return null
            val ns = vals.mapNotNull { it?.number }
            return if (ns.size == vals.size) ns else null
        }
        fun finite(x: Double?): ExprValue? = x?.takeIf { it.isFinite() }?.let { ExprValue.Number(it) }

        return when (name) {
            // 算術
            "add" -> allNums()?.let { ExprValue.Number(it.fold(0.0) { a, b -> a + b }) }
            "mul" -> allNums()?.let { ExprValue.Number(it.fold(1.0) { a, b -> a * b }) }
            "sub" -> { val a = num(0); val b = num(1); if (a == null || b == null) null else finite(a - b) }
            "div" -> { val a = num(0); val b = num(1); if (a == null || b == null || b == 0.0) null else finite(a / b) }
            // truncatingRemainder = Kotlin の %(IEEE の fmod)
            "mod" -> { val a = num(0); val b = num(1); if (a == null || b == null || b == 0.0) null else finite(a % b) }
            "neg" -> num(0)?.let { ExprValue.Number(-it) }
            "abs" -> num(0)?.let { ExprValue.Number(abs(it)) }
            "sqrt" -> num(0)?.takeIf { it >= 0 }?.let { ExprValue.Number(sqrt(it)) }
            "pow" -> { val a = num(0); val b = num(1); if (a == null || b == null) null else finite(SwiftNumbers.pow(a, b)) }
            "round", "floor", "ceil" -> {
                val a = num(0) ?: return null
                val digits = num(1) ?: 0.0
                val scale = SwiftNumbers.pow(10.0, SwiftNumbers.rounded(digits))
                val x = a * scale
                val r = when (name) {
                    "floor" -> floor(x)
                    "ceil" -> ceil(x)
                    else -> SwiftNumbers.rounded(x)
                }
                finite(r / scale)
            }
            "min" -> allNums()?.let { swiftMin(it) }?.let { ExprValue.Number(it) }
            "max" -> allNums()?.let { swiftMax(it) }?.let { ExprValue.Number(it) }
            "clamp" -> {
                val x = num(0); val lo = num(1); val hi = num(2)
                if (x == null || lo == null || hi == null || !(lo <= hi)) null
                else ExprValue.Number(swiftMin2(swiftMax2(x, lo), hi))
            }
            // 比較
            "eq", "neq" -> {
                if (vals.size < 2) return null
                val same = equal(vals[0], vals[1])
                ExprValue.Bool(if (name == "eq") same else !same)
            }
            "gt", "gte", "lt", "lte" -> {
                if (vals.size < 2) return null
                val a = vals[0] ?: return null
                val b = vals[1] ?: return null
                val order = compare(a, b) ?: return null
                ExprValue.Bool(
                    when (name) {
                        "gt" -> order > 0
                        "gte" -> order >= 0
                        "lt" -> order < 0
                        else -> order <= 0
                    },
                )
            }
            // 論理
            "not" -> ExprValue.Bool(!(vals.firstOrNull()?.truthy ?: false))
            // 文字列
            "concat" -> ExprValue.Str(vals.joinToString("") { it?.text ?: "" })
            "len" -> vals.firstOrNull()?.let { ExprValue.Number(SwiftNumbers.graphemeCount(it.text).toDouble()) }
            // 日付
            "now" -> ExprValue.Date(now)
            "today" -> ExprValue.Date(RepeatDate.startOfDay(now, zone))
            "daysBetween" -> { val a = date(0); val b = date(1); if (a == null || b == null) null else days(a, b) }
            "daysUntil" -> date(0)?.let { days(now, it) }
            "addDays" -> {
                val d = date(0) ?: return null
                val n = num(1)?.let { SwiftNumbers.rounded(it) } ?: return null
                if (!(abs(n) <= MAX_ADD_DAYS)) return null
                ExprValue.Date(ZonedDateTime.ofInstant(d, zone).plusDays(n.toLong()).toInstant())
            }
            "addMinutes" -> {
                // "HH:mm" → "HH:mm"(24 時間で一周)、日付・日時 → その時点の n 分後(経過時間)
                val n = num(1)?.let { SwiftNumbers.rounded(it) } ?: return null
                if (!(abs(n) <= MAX_ADD_MINUTES)) return null
                val first = vals.firstOrNull()
                if (first is ExprValue.Str) timeOfDay(first.value)?.let { return ExprValue.Str(clockString(it + n * 60)) }
                date(0)?.let { ExprValue.Date(it.plusSeconds(n.toLong() * 60)) }
            }
            "nextOccurrence" -> {
                // #266: 毎月 / 毎年の次の日(今日以降・月末に寄せる)。第 2 引数は "monthly" | "yearly"
                val d = date(0) ?: return null
                val unit = (vals.getOrNull(1) as? ExprValue.Str)?.value ?: return null
                RepeatDate.next(d, unit, now, zone)?.let { ExprValue.Date(it) }
            }
            "hoursBetween" -> {
                // end − start(時間・小数)。end < start は日付またぎとして +24h
                val a = secondsOfDay(0) ?: return null
                val b = secondsOfDay(1) ?: return null
                val diff = b - a
                ExprValue.Number((if (diff < 0) diff + 86_400 else diff) / 3600)
            }
            "year", "month", "day", "weekday" -> {
                val d = date(0) ?: return null
                val t = ZonedDateTime.ofInstant(d, zone)
                val v = when (name) {
                    "year" -> t.year
                    "month" -> t.monthValue
                    "day" -> t.dayOfMonth
                    else -> t.dayOfWeek.value % 7 + 1 // Calendar 準拠: 日=1 … 土=7
                }
                ExprValue.Number(v.toDouble())
            }
            // 変換
            "toNumber" -> when (val v = vals.firstOrNull()) {
                null -> null
                is ExprValue.Bool -> ExprValue.Number(if (v.value) 1.0 else 0.0)
                else -> v.number?.let { ExprValue.Number(it) }
            }
            "toString" -> vals.firstOrNull()?.let { ExprValue.Str(it.text) }
            else -> null
        }
    }

    /** 暦日の差(DST で 23・25 時間の日も 1 日・#556)。 */
    private fun days(a: Instant, b: Instant): ExprValue =
        ExprValue.Number(ChronoUnit.DAYS.between(LocalDate.ofInstant(a, zone), LocalDate.ofInstant(b, zone)).toDouble())

    // Swift の Array.min()/max() と Swift.min/max(NaN の扱いを合わせる)
    private fun swiftMin2(x: Double, y: Double): Double = if (y < x) y else x
    private fun swiftMax2(x: Double, y: Double): Double = if (y >= x) y else x
    private fun swiftMin(xs: List<Double>): Double? = xs.reduceOrNull { m, x -> if (x < m) x else m }
    private fun swiftMax(xs: List<Double>): Double? = xs.reduceOrNull { m, x -> if (m < x) x else m }
}
