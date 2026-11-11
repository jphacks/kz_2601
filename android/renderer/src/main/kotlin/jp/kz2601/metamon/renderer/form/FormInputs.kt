package jp.kz2601.metamon.renderer.form

import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.ComponentStyle
import jp.kz2601.metamon.dsl.expr.DSLClock
import jp.kz2601.metamon.dsl.expr.SwiftNumbers
import jp.kz2601.metamon.dsl.records.Aggregator
import jp.kz2601.metamon.dsl.records.Field
import jp.kz2601.metamon.dsl.records.FieldCoding
import jp.kz2601.metamon.dsl.records.FieldType
import jp.kz2601.metamon.dsl.records.JSONValue
import java.time.Instant
import java.time.LocalTime
import java.time.ZoneOffset
import java.time.ZonedDateTime
import java.util.Locale

// 入力部品が FormDraft に読み書きする形(iOS の FormDraft の Binding と FormDraft+Inputs.swift)。
// 値の形は iOS と同じにする(保存した値が OS で変わらない)。Compose に依らないので JVM で試せる。

/** 文字の欄(textField / textEditor / secureField / picker)。 */
fun FormDraft.text(field: String): String = (values[field] ?: JSONValue.Null).displayString

fun FormDraft.setText(field: String, text: String) {
    values[field] = JSONValue.Str(text)
}

/** numberField の文字。読めない文字はそのまま残し、保存の時に [FieldCoding.normalize] が落とす。 */
fun FormDraft.numberText(field: String): String = text(field)

/**
 * 数として読めれば数で書く(iOS と同じ)。ただし表示に戻すと打った文字と変わるもの("1." "0.50")は打ちかけなので文字のまま持つ
 * (数にすると「.」が消えて小数を打てない)。保存の時に [FieldCoding.normalize] が数にする。
 */
fun FormDraft.setNumberText(field: String, text: String) {
    val cleaned = text.replace(",", "")
    val n = cleaned.toDoubleOrNull()?.takeIf { it.isFinite() }?.let { JSONValue.Num(it) }
    values[field] = when {
        cleaned.isEmpty() -> JSONValue.Null
        n != null && n.displayString == cleaned -> n
        else -> JSONValue.Str(cleaned)
    }
}

fun FormDraft.bool(field: String): Boolean = (values[field] ?: JSONValue.Null).truthy

fun FormDraft.setBool(field: String, on: Boolean) {
    values[field] = JSONValue.Bool(on)
}

/** 日付の欄。読めなければ今。 */
fun FormDraft.date(field: String): Instant = FieldCoding.date(values[field] ?: JSONValue.Null) ?: DSLClock.now()

/** date は "YYYY-MM-DD"、それ以外(datetime)は RFC 3339。 */
fun FormDraft.setDate(field: String, at: Instant) {
    val f = entity.field(field) ?: return
    values[field] = JSONValue.Str(FieldCoding.encode(at, f))
}

/** slider / stepper / rating。integer 系(number 以外)は書く時に丸める。 */
fun FormDraft.double(field: String, fallback: Double): Double {
    val v = values[field] ?: JSONValue.Null
    return v.doubleValue ?: v.stringValue?.toDoubleOrNull() ?: fallback
}

fun FormDraft.setDouble(field: String, value: Double) {
    val isInteger = entity.field(field)?.type != FieldType.NUMBER
    values[field] = JSONValue.Num(if (isInteger) SwiftNumbers.rounded(value) else value)
}

/** timePicker: string/text には "HH:mm"(読めなければ今の時刻)。 */
fun FormDraft.time(field: String): LocalTime {
    val s = (values[field] ?: JSONValue.Null).stringValue
    return s?.let(TimeText::parse) ?: ZonedDateTime.ofInstant(DSLClock.now(), FieldCoding.zone).toLocalTime().withSecond(0).withNano(0)
}

/** timePicker の書き込み。datetime field は今日の日付のその時刻(RFC 3339)、それ以外は "HH:mm"。 */
fun FormDraft.setTime(field: String, hour: Int, minute: Int) {
    val f = entity.field(field) ?: return
    if (f.type == FieldType.DATETIME) {
        val base = ZonedDateTime.ofInstant(date(field), FieldCoding.zone)
        setDate(field, base.withHour(hour).withMinute(minute).withSecond(0).withNano(0).toInstant())
    } else {
        values[field] = JSONValue.Str(TimeText.format(hour, minute))
    }
}

/** timePicker は未入力でも今の時刻を出す。見えている時刻をそのまま保存値にする(#229)。入力済みなら何もしない。 */
fun FormDraft.seedTimeIfEmpty(field: String, now: Instant = DSLClock.now()) {
    val f = entity.field(field) ?: return
    if (FieldCoding.normalize(values[field] ?: JSONValue.Null, f) != null) return
    if (f.type == FieldType.DATETIME) {
        values[field] = JSONValue.Str(FieldCoding.encode(now, f))
    } else {
        val t = ZonedDateTime.ofInstant(now, FieldCoding.zone)
        values[field] = JSONValue.Str(TimeText.format(t.hour, t.minute))
    }
}

/** colorPicker: "#RRGGBB"。読めなければ null(既定の色で出す)。 */
fun FormDraft.color(field: String): Int? = ColorHex.parse((values[field] ?: JSONValue.Null).stringValue)

fun FormDraft.setColor(field: String, rgb: Int) {
    values[field] = JSONValue.Str(ColorHex.format(rgb))
}

/** refs / members の ON・OFF。並びは [listed](参照先・参加者の順)に保ち、一覧に無い id(抜けた人)は後ろに残す。 */
fun FormDraft.toggleId(field: String, id: String, listed: List<String>) {
    val current = Aggregator.refIds(values[field] ?: JSONValue.Null)
    val ids = current.toMutableSet()
    if (!ids.add(id)) ids.remove(id)
    val ordered = listed.filter { it in ids } + current.filter { it in ids && it !in listed }
    values[field] = JSONValue.Arr(ordered.map { JSONValue.Str(it) })
}

fun FormDraft.selectedIds(field: String): List<String> = Aggregator.refIds(values[field] ?: JSONValue.Null)

/** "HH:mm"(iOS の `timeFormatter`・en_US_POSIX)。 */
object TimeText {
    private val pattern = Regex("""^([0-9]{1,2}):([0-9]{2})$""")

    fun parse(s: String): LocalTime? {
        val m = pattern.matchEntire(s) ?: return null
        val h = m.groupValues[1].toInt()
        val min = m.groupValues[2].toInt()
        if (h > 23 || min > 59) return null
        return LocalTime.of(h, min)
    }

    fun format(hour: Int, minute: Int): String = "%02d:%02d".format(Locale.ROOT, hour, minute)
}

/** "#RRGGBB" と 0xRRGGBB(iOS の Color+Hex.swift)。 */
object ColorHex {
    fun parse(s: String?): Int? {
        val t = s?.trim()?.removePrefix("#") ?: return null
        if (t.length != 6) return null
        return t.toIntOrNull(16)
    }

    fun format(rgb: Int): String = "#%06X".format(Locale.ROOT, rgb and 0xFFFFFF)

    /** 自前の色の一覧(システムの色選びが無いので並べる)。 */
    val palette: List<Int> = listOf(
        0xFF3B30, 0xFF9500, 0xFFCC00, 0x34C759, 0x00C7BE, 0x30B0C7,
        0x007AFF, 0x5856D6, 0xAF52DE, 0xFF2D55, 0xA2845E, 0x8E8E93,
    )
}

/** 数値入力(slider / stepper / rating)の DSL 値の丸め(iOS の NumericInput)。AI の DSL は min > max・step ≤ 0 も来うる。 */
object NumericInput {
    /** min..max。省略・非有限は既定。max ≤ min なら min のすぐ上まで(幅 0 にしない)。 */
    fun range(min: Double?, max: Double?, defaultMin: Double, defaultMax: Double): ClosedFloatingPointRange<Double> {
        val lo = min?.takeIf { it.isFinite() } ?: defaultMin
        val rawHi = max?.takeIf { it.isFinite() } ?: defaultMax
        val hi = if (rawHi > lo) rawHi else maxOf(lo + 0.001, Math.nextUp(lo))
        return lo..hi
    }

    /** 刻み。0 以下・非有限は null。 */
    fun step(raw: Double?): Double? = raw?.takeIf { it.isFinite() && it > 0 }

    /** rating の星の数(1〜10・既定 5)。 */
    fun ratingCount(raw: Double?): Int {
        if (raw == null || !raw.isFinite()) return 5
        return raw.coerceIn(1.0, 10.0).toInt()
    }

    /** rating の保存値 → 塗る星の数(0〜count)。 */
    fun ratingValue(v: Double, count: Int): Int {
        if (!v.isFinite()) return 0
        return Math.round(v.coerceIn(0.0, count.toDouble())).toInt()
    }

    /** slider の値を刻みに合わせる(Slider の steps は「間の点の数」なので、刻みから数える)。 */
    fun sliderSteps(range: ClosedFloatingPointRange<Double>, step: Double?): Int {
        step ?: return 0
        val n = ((range.endInclusive - range.start) / step).toLong() - 1
        return n.coerceIn(0, 200).toInt()
    }

    fun snap(v: Double, range: ClosedFloatingPointRange<Double>, step: Double?): Double {
        val c = v.coerceIn(range.start, range.endInclusive)
        step ?: return c
        return (range.start + Math.rint((c - range.start) / step) * step).coerceIn(range.start, range.endInclusive)
    }
}

/** テキスト入力の規則(iOS の TextInputRules・#196)。 */
object TextInputRules {
    /** 複数行にするか。DSL の multiline が最優先。無ければ text 型か、本文・メモ系の名前(一覧の見出し field は除く)。 */
    fun isMultiline(c: Component, field: Field?, titleField: String?): Boolean {
        c.multiline?.let { return it }
        field ?: return false
        if (field.type == FieldType.TEXT) return true
        if (field.type != FieldType.STRING || field.id == titleField) return false
        val hay = (field.id + " " + field.name + " " + (c.label ?: "")).lowercase()
        return longTextHints.any { hay.contains(it) }
    }

    val longTextHints = listOf("本文", "日記", "メモ", "詳細", "説明", "感想", "body", "memo", "note", "description", "comment", "detail")

    /** textField の複数行の行数。 */
    val multilineLines = 3..8

    const val MAX_EDITOR_MIN_LINES = 30

    /** textEditor の行数。minLines(既定 4・最大 30)から、最低でも 10 行まで伸びる。 */
    fun editorLines(minLines: Int?): IntRange {
        val low = (minLines ?: 4).coerceIn(1, MAX_EDITOR_MIN_LINES)
        return low..maxOf(low + 6, 10)
    }
}

/** picker の選択肢と見た目(iOS の PickerLeaf)。 */
object PickerRules {
    /** 選択肢から消した値を持つ既存の記録は、その値も並べる(#194)。 */
    fun options(base: List<String>, current: String): List<String> =
        if (current.isEmpty() || current in base) base else base + current

    /** 省略時は 4 件以下なら segmented。 */
    fun style(c: Component, optionCount: Int): String =
        c.style?.takeIf { it != ComponentStyle.UNKNOWN }?.wire ?: if (optionCount <= 4) "segmented" else "menu"
}

/** datePicker の表示と、Material の DatePicker(UTC の 0 時のミリ秒)との受け渡し。 */
object DateInput {
    fun text(at: Instant, withTime: Boolean, locale: Locale): String = FieldCoding.displayDate(at, withTime, locale, DSLClock.now())

    /** 端末のタイムゾーンでのその日 → UTC のその日の 0 時。 */
    fun pickerMillis(at: Instant): Long =
        at.atZone(FieldCoding.zone).toLocalDate().atStartOfDay(ZoneOffset.UTC).toInstant().toEpochMilli()

    /** 選んだ日(UTC の 0 時)に、今の値の時刻(端末のタイムゾーン)を付ける。 */
    fun fromPicker(millis: Long, current: Instant): Instant {
        val day = Instant.ofEpochMilli(millis).atZone(ZoneOffset.UTC).toLocalDate()
        val time = ZonedDateTime.ofInstant(current, FieldCoding.zone).toLocalTime()
        return day.atTime(time).atZone(FieldCoding.zone).toInstant()
    }
}
