package jp.kz2601.metamon.dsl.records

import jp.kz2601.metamon.dsl.expr.DSLClock
import jp.kz2601.metamon.dsl.expr.SwiftNumbers
import java.math.RoundingMode
import java.text.DecimalFormat
import java.text.DecimalFormatSymbols
import java.time.DateTimeException
import java.time.Instant
import java.time.LocalDate
import java.time.LocalDateTime
import java.time.ZoneId
import java.time.ZoneOffset
import java.time.format.DateTimeFormatter
import java.util.Locale
import kotlin.math.abs

/**
 * Field.type ごとの値表現(appconfig `RecordData` の規約と一致。iOS の Records/FieldCoding.swift):
 * string/text→string, integer/currency→整数, number→数値, boolean→bool, date→"YYYY-MM-DD", datetime→RFC3339, enum→options の1つ,
 * photo→"data:image/jpeg;base64,..." or https URL, location→"lat,lng"。
 */
object FieldCoding {
    /**
     * 端末のタイムゾーン("YYYY-MM-DD" の読み書きと、式の評価器の既定)。iOS の `dateFormatter.timeZone`(= 端末の設定)。
     * テストでは固定する。
     */
    @Volatile
    var zone: ZoneId = ZoneId.systemDefault()

    // MARK: - 日付の読み書き

    /** "YYYY-MM-DD"(端末のタイムゾーンでの暦日)。iOS の `dateFormatter.string(from:)`。 */
    fun formatDay(d: Instant, zone: ZoneId = this.zone): String = dayString(LocalDate.ofInstant(d, zone))

    fun dayString(day: LocalDate): String {
        val y = if (day.year <= 0) 1 - day.year else day.year // 紀元(yyyy は年の数)
        return "%04d-%02d-%02d".format(Locale.ROOT, y, day.monthValue, day.dayOfMonth)
    }

    /** RFC 3339(UTC・秒まで)。iOS の `datetimeFormatter`(ISO8601DateFormatter の既定)。 */
    fun formatDatetime(d: Instant): String {
        val t = LocalDateTime.ofEpochSecond(d.epochSecond, 0, ZoneOffset.UTC)
        return "%04d-%02d-%02dT%02d:%02d:%02dZ".format(Locale.ROOT, t.year, t.monthValue, t.dayOfMonth, t.hour, t.minute, t.second)
    }

    fun defaultValue(f: Field, now: Instant = DSLClock.now()): JSONValue {
        // 選択肢から消えた既定値は使わない(#194)
        val d = f.defaultValue
        if (d != null && !(f.type == FieldType.ENUM && (f.options ?: emptyList()).contains(d.stringValue ?: "").not())) {
            if (f.type.isDate && d.stringValue == "now") return JSONValue.Str(encode(now, f))
            return d
        }
        return when (f.type) {
            FieldType.BOOLEAN -> JSONValue.Bool(false)
            FieldType.ENUM -> f.options?.firstOrNull()?.let { JSONValue.Str(it) } ?: JSONValue.Null
            FieldType.REF, FieldType.REFS -> JSONValue.Null
            FieldType.DATE, FieldType.DATETIME -> JSONValue.Str(encode(now, f))
            else -> JSONValue.Null
        }
    }

    fun encode(date: Instant, f: Field): String = if (f.type == FieldType.DATE) formatDay(date) else formatDatetime(date)

    /**
     * 日付の値を読む: RFC 3339(秒とオフセット必須・小数秒可)か "YYYY-MM-DD"(端末のタイムゾーンのその日の最初の時刻)。
     * 2/30・24:00 のようなあり得ない日時は読まない(#555)。
     */
    fun date(value: JSONValue, zone: ZoneId = this.zone): Instant? {
        val s = value.stringValue ?: return null
        return strictDatetime(s) ?: strictDay(s, zone)
    }

    private val datetimePattern =
        Regex("""^([0-9]{4})-([0-9]{2})-([0-9]{2})T([0-9]{2}):([0-9]{2}):([0-9]{2})(\.[0-9]+)?(Z|[+-][0-9]{2}:?[0-9]{2})$""")
    private val dayPattern = Regex("""^([0-9]{1,4})-([0-9]{1,2})-([0-9]{1,2})$""")

    /** RFC 3339。繰り上げ(2/30・24:00)は読まない。紀元前(年 0)も読まない。 */
    fun strictDatetime(s: String): Instant? {
        val m = datetimePattern.matchEntire(s) ?: return null
        val g = m.groupValues
        val y = g[1].toInt()
        if (y < 1) return null
        val local = try {
            LocalDateTime.of(y, g[2].toInt(), g[3].toInt(), g[4].toInt(), g[5].toInt(), g[6].toInt())
        } catch (_: DateTimeException) {
            return null
        }
        val offset = try {
            ZoneOffset.ofTotalSeconds(offsetSeconds(g[8]))
        } catch (_: DateTimeException) {
            return null
        }
        val nanos = if (g[7].isEmpty()) 0L else (g[7].substring(1) + "000000000").substring(0, 9).toLong()
        return local.toInstant(offset).plusNanos(nanos)
    }

    /** "YYYY-MM-DD"(月・日は 1 桁も可)。その日の最初の時刻(DST で 0:00 が無い日は 1:00・その日が丸ごと無ければ null)。 */
    fun strictDay(s: String, zone: ZoneId = this.zone): Instant? {
        val m = dayPattern.matchEntire(s) ?: return null
        val (y, mo, d) = m.destructured
        val day = try {
            LocalDate.of(y.toInt(), mo.toInt(), d.toInt())
        } catch (_: DateTimeException) {
            return null
        }
        if (day.year < 1) return null
        val start = day.atStartOfDay(zone)
        if (start.toLocalDate() != day) return null
        return start.toInstant()
    }

    private fun offsetSeconds(z: String): Int {
        if (z == "Z") return 0
        val digits = z.substring(1).replace(":", "")
        val secs = digits.take(2).toInt() * 3600 + digits.takeLast(2).toInt() * 60
        return if (z.startsWith("-")) -secs else secs
    }

    // MARK: - 送信前の正規化

    /** 送信前の正規化。空文字・未入力は null(キーごと落とす)。 */
    fun normalize(v: JSONValue, f: Field): JSONValue? = when (f.type) {
        FieldType.INTEGER, FieldType.CURRENCY -> {
            v.doubleValue?.let { JSONValue.Num(SwiftNumbers.rounded(it)) }
                ?: v.stringValue?.let { s -> SwiftNumbers.parseDouble(s.replace(",", ""))?.let { JSONValue.Num(SwiftNumbers.rounded(it)) } }
        }
        FieldType.NUMBER -> v.doubleValue?.let { JSONValue.Num(it) } ?: v.stringValue?.let { s -> SwiftNumbers.parseDouble(s)?.let { JSONValue.Num(it) } }
        FieldType.BOOLEAN -> JSONValue.Bool(v.truthy)
        FieldType.REFS, FieldType.MEMBERS -> {
            // id の配列(#231)・uid の配列(#271)。未選択はキーごと落とす
            val ids = Aggregator.refIds(v)
            if (ids.isEmpty()) null else JSONValue.Arr(ids.map { JSONValue.Str(it) })
        }
        else -> when {
            v.stringValue != null -> if (v.stringValue!!.isEmpty()) null else v
            v.isNull -> null
            else -> v
        }
    }

    // MARK: - 表示

    /** display が出す固定の語(#150)。描画側が文言カタログから引いた語を渡す。既定は日本語。 */
    data class Words(
        val yes: String,
        val no: String,
        val photo: String,
        val video: String = "動画",
        val audio: String = "録音",
    ) {
        companion object {
            val ja = Words(yes = "はい", no = "いいえ", photo = "写真", video = "動画", audio = "録音")
        }
    }

    /** 値が出せない時の表示。 */
    const val UNAVAILABLE = "—"

    /** 数値の表示(小数は 2 桁まで・末尾の 0 は落とす・#229)。 */
    fun plainNumber(n: Double): String {
        if (!n.isFinite()) return SwiftNumbers.description(n)
        val r = SwiftNumbers.rounded(n * 100) / 100
        if (r == SwiftNumbers.rounded(r) && abs(r) < 1e15) return r.toLong().toString()
        var s = Aggregator.cFormat(r, 2)
        while (s.endsWith("0")) s = s.dropLast(1)
        return s
    }

    /** 一覧・集計の表示用整形。`locale` / `now` は日付の書式にだけ使う。 */
    fun display(
        v: JSONValue,
        field: Field?,
        locale: Locale = Locale.getDefault(),
        now: Instant = Instant.now(),
        words: Words = Words.ja,
    ): String {
        if (field == null) return v.displayString
        // computed が評価できなかった = 「—」(#101)
        if (field.isComputed && v.isNull) return UNAVAILABLE
        return when (field.type) {
            FieldType.CURRENCY -> {
                val body = currency(v.doubleValue ?: 0.0, locale)
                val suffix = currencySuffix(field.unit) ?: return body
                body + (if (suffix.startsWith("/")) "" else " ") + suffix
            }
            FieldType.NUMBER, FieldType.INTEGER -> {
                val n = v.doubleValue ?: return ""
                val s = plainNumber(n)
                field.unit?.let { "$s $it" } ?: s
            }
            FieldType.DATE, FieldType.DATETIME -> {
                val d = date(v) ?: return v.displayString
                displayDate(d, field.type == FieldType.DATETIME, locale, now)
            }
            FieldType.BOOLEAN -> if (v.truthy) words.yes else words.no
            FieldType.PHOTO -> if (v.isNull) "" else words.photo
            FieldType.VIDEO -> if (v.isNull) "" else words.video
            FieldType.AUDIO -> AudioValue.parse(v)?.let { if (it.seconds > 0) words.audio + " " + AudioValue.elapsedText(it.seconds.toDouble()) else words.audio } ?: ""
            else -> v.displayString
        }
    }

    /** 円の表示(四捨五入は偶数丸め・負は「-¥500」・桁区切りは locale)。 */
    fun currency(n: Double, locale: Locale = Locale.getDefault()): String {
        val f = DecimalFormat("#,##0", DecimalFormatSymbols.getInstance(locale))
        f.roundingMode = RoundingMode.HALF_EVEN
        val body = "¥" + f.format(abs(n))
        return if (n < 0 && SwiftNumbers.rounded(n) != 0.0) "-$body" else body
    }

    /** 通貨そのものを表す unit(大小文字は無視)。 */
    internal val currencyUnitTokens = listOf("円", "¥", "￥", "jpy", "yen", "$", "usd", "dollar", "dollars")

    /** currency field の unit のうち、¥ の後ろに付ける残り(#150)。 */
    fun currencySuffix(unit: String?): String? {
        val raw = unit?.let { trimSpaces(it) }?.takeIf { it.isNotEmpty() } ?: return null
        val lower = raw.lowercase()
        if (lower in currencyUnitTokens) return null
        for (token in currencyUnitTokens.sortedByDescending { it.length }) {
            if (lower.startsWith(token)) {
                val rest = trimSpaces(raw.drop(token.length))
                return rest.ifEmpty { null }
            }
        }
        return raw
    }

    /** Swift の `.whitespaces`(改行は含まない)で前後を落とす。 */
    private fun trimSpaces(s: String): String = s.trim { it == '\t' || Character.isSpaceChar(it) }

    /**
     * 日付の表示(#140)。ja「10/24(金)」「10/24(金) 14:30」、それ以外は英語の並び「Fri, Oct 24」「Fri, Oct 24, 2:30 PM」。
     * 今年でなければ年を付ける。
     */
    fun displayDate(d: Instant, withTime: Boolean, locale: Locale, now: Instant = Instant.now()): String {
        val z = zone
        val sameYear = LocalDate.ofInstant(d, z).year == LocalDate.ofInstant(now, z).year
        val pattern = if (locale.language == "ja") {
            (if (sameYear) "M/d(EEE)" else "uuuu/M/d(EEE)") + (if (withTime) " H:mm" else "")
        } else {
            (if (sameYear) "EEE, MMM d" else "EEE, MMM d, uuuu") + (if (withTime) ", h:mm a" else "")
        }
        // 暦は西暦に固定(端末が和暦でも 2025 と出す)
        val formatLocale = Locale.Builder().setLanguage(locale.language).setRegion(locale.country).build()
        return DateTimeFormatter.ofPattern(pattern, formatLocale).withZone(z).format(d)
    }
}

/** 端末内の録音の値("audio:<id>|<秒>"・#420)。表示に要る分だけ。 */
data class AudioValue(val id: String, val seconds: Int) {
    companion object {
        const val PREFIX = "audio:"

        fun parse(v: JSONValue): AudioValue? {
            val s = v.stringValue ?: return null
            if (!s.startsWith(PREFIX)) return null
            val parts = s.substring(PREFIX.length).split("|", limit = 2)
            val id = parts[0]
            if (!isValidId(id)) return null
            return AudioValue(id, maxOf(0, if (parts.size > 1) parts[1].toIntOrNull() ?: 0 else 0))
        }

        fun isValidId(id: String): Boolean =
            id.isNotEmpty() && id.length <= 80 && id.all { (it in 'a'..'z') || (it in 'A'..'Z') || (it in '0'..'9') || it == '-' || it == '_' }

        /** 経過時間の表示("1:05"・"1:02:03")。 */
        fun elapsedText(seconds: Double): String {
            val t = maxOf(0, (if (seconds.isFinite()) seconds else 0.0).toInt())
            val h = t / 3600
            val m = (t % 3600) / 60
            val s = t % 60
            return if (h > 0) "%d:%02d:%02d".format(Locale.ROOT, h, m, s) else "%d:%02d".format(Locale.ROOT, m, s)
        }
    }
}
