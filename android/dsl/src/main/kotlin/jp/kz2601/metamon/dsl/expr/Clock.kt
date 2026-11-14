package jp.kz2601.metamon.dsl.expr

import jp.kz2601.metamon.dsl.records.FieldCoding
import jp.kz2601.metamon.dsl.records.JSONValue
import jp.kz2601.metamon.dsl.records.Record
import java.time.Instant
import java.time.LocalDate
import java.time.ZoneId
import java.time.ZonedDateTime

// iOS の Logic/LogicV3.swift(DSLClock・DailyCheck)と Logic/RepeatDate.swift。
// iOS の Calendar は ZoneId(グレゴリオ暦)で表す。

/** 「今」の基準時刻。期間 filter・`today`/`now`・毎日リセットのチェックが使う。 */
object DSLClock {
    /** 固定の時刻(適合テスト・#352 の `device.now`)。null なら実時刻。アプリからは設定しない。 */
    @Volatile
    var fixed: Instant? = null

    fun now(): Instant = fixed ?: Instant.now()

    /**
     * `METAMON_DEBUG_TODAY=YYYY-MM-DD` の日付 + 実時刻の時:分:秒。未設定・形式外は null(iOS の DEBUG ビルドの日付差し替え)。
     * Android ではデバッグ用の設定から呼ぶ(ここでは環境変数を勝手に読まない)。
     */
    fun debugNow(real: Instant, env: Map<String, String>, zone: ZoneId = FieldCoding.zone): Instant? {
        val s = env["METAMON_DEBUG_TODAY"] ?: return null
        val day = FieldCoding.strictDay(s, zone) ?: return null
        val t = ZonedDateTime.ofInstant(real, zone)
        return ZonedDateTime.ofInstant(day, zone).withHour(t.hour).withMinute(t.minute).withSecond(t.second).toInstant()
    }
}

/** `list.item.resets: "daily"`(#132): toggle は date field。値が今日(端末ローカル)なら済み。押した日の履歴(#172)も持つ。 */
object DailyCheck {
    /** 履歴の保存キーの接尾辞(toggle field id + "__dates")。値は "YYYY-MM-DD" の昇順配列。 */
    const val HISTORY_SUFFIX = "__dates"

    /** 履歴の上限(古い方から捨てる)。appconfig の上限と揃える。 */
    const val MAX_HISTORY = 400

    fun historyKey(field: String): String = field + HISTORY_SUFFIX

    /** 保存値が `today` と同じ日か。"YYYY-MM-DD" も RFC3339 も読む。空・形式外は未チェック。 */
    fun isDone(value: JSONValue, today: Instant, zone: ZoneId = FieldCoding.zone): Boolean {
        val d = FieldCoding.date(value) ?: return false
        val s = value.stringValue
        if (s != null && !s.contains('T')) return s == FieldCoding.formatDay(today)
        return LocalDate.ofInstant(d, zone) == LocalDate.ofInstant(today, zone)
    }

    /** チェックを付けた/外した時に保存する値(付けたら今日の "YYYY-MM-DD"、外したら null)。 */
    fun stored(on: Boolean, today: Instant): JSONValue = if (on) JSONValue.Str(FieldCoding.formatDay(today)) else JSONValue.Null

    /** 押した日("YYYY-MM-DD")の昇順・重複なし。履歴の無い旧レコードは toggle の値 1 件として読む。 */
    fun dates(record: Record, field: String): List<String> {
        val set = HashSet<String>()
        (record[historyKey(field)] as? JSONValue.Arr)?.items?.forEach { x -> day(x)?.let { set.add(it) } }
        day(record[field])?.let { set.add(it) }
        return set.sorted()
    }

    /** チェックを付けた/外した時に保存する差分(toggle field と履歴)。 */
    fun patch(record: Record, field: String, on: Boolean, today: Instant): Map<String, JSONValue> {
        val t = FieldCoding.formatDay(today)
        var ds = dates(record, field).filter { it != t }
        if (on) ds = ds + t
        ds = ds.sorted().takeLast(MAX_HISTORY)
        val last: JSONValue = if (on) JSONValue.Str(t) else ds.lastOrNull()?.let { JSONValue.Str(it) } ?: JSONValue.Null
        return mapOf(field to last, historyKey(field) to if (ds.isEmpty()) JSONValue.Null else JSONValue.Arr(ds.map { JSONValue.Str(it) }))
    }

    /** 連続日数。今日やっていれば今日から、まだなら昨日から遡って続いた日数。 */
    fun streak(days: List<String>, today: Instant, zone: ZoneId = FieldCoding.zone): Int {
        val set = days.toSet()
        var d = LocalDate.ofInstant(today, zone)
        if (key(d) !in set) d = d.minusDays(1)
        var n = 0
        while (key(d) in set && n <= MAX_HISTORY) {
            n++
            d = d.minusDays(1)
        }
        return n
    }

    /** 直近 n 日(今日を含む)にやった日数。n < 1 は 0。 */
    fun count(days: List<String>, lastDays: Int, today: Instant, zone: ZoneId = FieldCoding.zone): Int {
        if (lastDays < 1) return 0
        val start = LocalDate.ofInstant(today, zone)
        val window = (0 until minOf(lastDays, MAX_HISTORY)).map { key(start.minusDays(it.toLong())) }.toSet()
        return days.toSet().intersect(window).size
    }

    private fun key(d: LocalDate): String = FieldCoding.dayString(d)

    /** 保存値 → "YYYY-MM-DD"(RFC3339 は端末ローカルの日付)。空・形式外は null。 */
    private fun day(v: JSONValue): String? {
        val s = v.stringValue?.takeIf { it.isNotEmpty() } ?: return null
        val d = FieldCoding.date(v) ?: return null
        return if (s.contains('T')) FieldCoding.formatDay(d) else s.take(10)
    }
}

/**
 * 毎月 / 毎年くり返す日付(#266)。式の `nextOccurrence` と通知(`repeat: monthly|yearly`)で共通。
 * 日が無い月は月末に寄せる(1/31 の毎月 → 2/28)。寄せるのはその回だけ(1/31 → 2/28 → 3/31)。
 */
object RepeatDate {
    /** "monthly" = 1 か月、"yearly" = 12 か月。それ以外は null。 */
    fun months(unit: String): Int? = when (unit) {
        "monthly" -> 1
        "yearly" -> 12
        else -> null
    }

    /** 起点から k 回目(0 = 起点)。時刻はそのまま、日は月末に寄せる。 */
    fun occurrence(anchor: Instant, months: Int, index: Int, zone: ZoneId): Instant =
        ZonedDateTime.ofInstant(anchor, zone).plusMonths(months.toLong() * index).toInstant()

    /** anchor と date の暦の月の差(date の月 − anchor の月)。 */
    fun monthsBetween(anchor: Instant, date: Instant, zone: ZoneId): Int {
        val a = LocalDate.ofInstant(anchor, zone)
        val b = LocalDate.ofInstant(date, zone)
        return (b.year - a.year) * 12 + b.monthValue - a.monthValue
    }

    /** 今日以降で最も近い回の日付(その日の最初の時刻)。起点が今日以降なら起点の日。 */
    fun next(anchor: Instant, unit: String, today: Instant, zone: ZoneId): Instant? {
        val m = months(unit) ?: return null
        val a = startOfDay(anchor, zone)
        val t = startOfDay(today, zone)
        if (a >= t) return a
        var k = maxOf(monthsBetween(a, t, zone) / m, 0)
        repeat(4) {
            val day = startOfDay(occurrence(a, m, k, zone), zone)
            if (day >= t) return day
            k++
        }
        return null
    }

    internal fun startOfDay(d: Instant, zone: ZoneId): Instant = LocalDate.ofInstant(d, zone).atStartOfDay(zone).toInstant()
}
