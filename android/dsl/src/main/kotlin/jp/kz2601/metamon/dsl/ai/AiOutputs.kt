package jp.kz2601.metamon.dsl.ai

import jp.kz2601.metamon.dsl.expr.DSLClock
import jp.kz2601.metamon.dsl.expr.SwiftNumbers
import jp.kz2601.metamon.dsl.logic.RecordImport
import jp.kz2601.metamon.dsl.records.Entity
import jp.kz2601.metamon.dsl.records.Field
import jp.kz2601.metamon.dsl.records.FieldCoding
import jp.kz2601.metamon.dsl.records.FieldType
import jp.kz2601.metamon.dsl.records.JSONValue
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import java.time.Instant
import java.time.LocalDateTime

/**
 * `aiButton.outputs`(iOS の `AIOutputs`): generate の fields の項目名 → フォームの field id。検証は backend `dsl/ai_outputs.go`。
 * 結果を field の型に直してフォームのドラフトに入れる(保存は利用者)。読めない値は入れない(空のまま・入力済みは消さない)。
 */
object AiOutputs {
    /** 1 項目の入れ先と、入れる値(読めなければ null)。 */
    data class Fill(val item: String, val field: String, val fieldName: String, val value: JSONValue?)

    /** 入れた結果。[applied] は入れた(同じ値だった)項目、[kept] は入力済みで値が違うため入れなかった項目。 */
    data class Applied(val applied: List<Fill> = emptyList(), val kept: List<Fill> = emptyList())

    private val types = setOf(FieldType.STRING, FieldType.TEXT, FieldType.INTEGER, FieldType.NUMBER, FieldType.DATE, FieldType.DATETIME, FieldType.ENUM)

    /** 「分からない」の答え。どの型でも空のままにする。 */
    private val blanks = setOf("-", "－", "ー", "—", "―", "?", "？", "不明", "なし", "無し", "未定", "不詳", "n/a", "na", "none", "unknown", "tbd")

    /** DSL の `outputs`(項目名 → field id)。オブジェクトでない・値が文字でないものは無視(表示だけ)。 */
    fun from(e: JsonElement?): Map<String, String>? {
        val o = e as? JsonObject ?: return null
        val out = linkedMapOf<String, String>()
        for ((k, v) in o) {
            val s = (v as? JsonPrimitive)?.takeIf { it.isString }?.content ?: return null
            out[k] = s
        }
        return out
    }

    /** 結果の fields を結果の順で入れ先に対応させる。outputs に無い項目・入れられない field は飛ばす。 */
    fun fills(fields: List<AiTextField>, outputs: Map<String, String>, entity: Entity, now: Instant = DSLClock.now()): List<Fill> {
        val map = outputs.mapKeys { it.key.trim() }
        return fields.mapNotNull { f ->
            val name = f.name.trim()
            val id = map[name] ?: return@mapNotNull null
            val field = entity.storedFields.firstOrNull { it.id == id }?.takeIf { it.type in types } ?: return@mapNotNull null
            Fill(name, id, field.name, value(f.value, field, now))
        }
    }

    /** 読めた値を [values] に入れる。上書きしない: 空の field と、[replaceable](新規で開いたままの初期値・前回 AI が入れた値のまま)だけ。 */
    fun apply(fills: List<Fill>, values: MutableMap<String, JSONValue>, replaceable: (field: String, current: JSONValue) -> Boolean): Applied {
        val applied = mutableListOf<Fill>()
        val kept = mutableListOf<Fill>()
        for (f in fills) {
            val v = f.value ?: continue
            val cur = values[f.field] ?: JSONValue.Null
            if (cur == v || isBlank(cur) || replaceable(f.field, cur)) {
                values[f.field] = v
                applied += f
            } else {
                kept += f
            }
        }
        return Applied(applied, kept)
    }

    /** 入力が無い値(null・空白だけの文字)。 */
    internal fun isBlank(v: JSONValue): Boolean = v.isNull || (v is JSONValue.Str && v.value.isBlank())

    /** AI の 1 つの答え → field の値。string / text はそのまま(上限で切る)、数・日付・enum は読めた時だけ。 */
    fun value(raw: String, field: Field, now: Instant = DSLClock.now()): JSONValue? {
        val s = raw.trim()
        if (s.isEmpty() || s.lowercase() in blanks) return null
        return when (field.type) {
            FieldType.STRING, FieldType.TEXT -> {
                val limit = RecordImport.characterLimit(field)
                val n = s.codePointCount(0, s.length)
                JSONValue.Str(if (n > limit) s.substring(0, s.offsetByCodePoints(0, limit)) else s)
            }
            FieldType.INTEGER, FieldType.NUMBER -> {
                val plain = RecordImport.number(s, field.unit)
                val n = when {
                    plain != null -> plain
                    else -> {
                        val k = kanjiNumber(s)
                        // 万・千・億がある: 読めなければ null(「3万円」を 3 にしない)
                        if (k != null) k.value else loneNumber(s)
                    }
                } ?: return null
                JSONValue.Num(if (field.type == FieldType.INTEGER) SwiftNumbers.rounded(n) else n)
            }
            FieldType.DATE, FieldType.DATETIME -> {
                val t = rangeStart(clock(s))
                // date は書かれたオフセットのままの日付(端末の TZ に直すと日がずれる)
                if (field.type == FieldType.DATE) isoDay(t)?.let { return JSONValue.Str(it) }
                var d = RecordImport.date(s, now) ?: RecordImport.date(t, now) ?: return null
                if (!hasYear(t)) nextYearIfPast(d, now)?.let { d = it }
                JSONValue.Str(FieldCoding.encode(d, field))
            }
            FieldType.ENUM -> {
                val options = field.options.orEmpty()
                (options.firstOrNull { it == s } ?: options.firstOrNull { it.lowercase() == s.lowercase() })?.let { JSONValue.Str(it) }
            }
            else -> null
        }
    }

    /** 書き方の注文(数・日付・enum の項目だけ)。`POST /v1/text` の formats(書き方の文はサーバーが足す)。無ければ null。 */
    fun formats(items: List<String>, outputs: Map<String, String>, entity: Entity): List<AiTextFormat>? {
        val out = mutableListOf<AiTextFormat>()
        for (item in items) {
            val id = outputs[item] ?: continue
            val f = entity.storedFields.firstOrNull { it.id == id } ?: continue
            when (f.type) {
                FieldType.INTEGER, FieldType.NUMBER -> out += AiTextFormat(item, "number")
                FieldType.DATE -> out += AiTextFormat(item, "date")
                FieldType.DATETIME -> out += AiTextFormat(item, "datetime")
                FieldType.ENUM -> {
                    // backend の上限(20 個・各 40 字)に収める
                    val opts = f.options.orEmpty().map { it.trim() }.filter { it.isNotEmpty() && it.length <= 40 }.take(20)
                    if (opts.isNotEmpty()) out += AiTextFormat(item, "enum", opts)
                }
                else -> Unit
            }
        }
        return out.ifEmpty { null }
    }

    /** formats の書き方の文(端末内 AI 用・backend `textFormatHint` と同じ文)。無ければ null。 */
    fun hint(formats: List<AiTextFormat>?, english: Boolean): String? {
        if (formats.isNullOrEmpty()) return null
        val parts = formats.map { f ->
            val s = when (f.kind) {
                "number" -> if (english) "number only" else "数字だけ"
                "date" -> if (english) "YYYY/M/D (M/D if no year)" else "YYYY/M/D(年が無ければ M/D)"
                "datetime" -> if (english) "YYYY/M/D H:mm (M/D H:mm if no year)" else "YYYY/M/D H:mm(年が無ければ M/D H:mm)"
                else -> f.options.orEmpty().joinToString("/") + if (english) " (one of these)" else " のどれか"
            }
            "${f.item}=$s"
        }
        return (if (english) "Format: " else "書き方: ") + parts.joinToString(" / ") + if (english) " (unknown: -)" else "(分からなければ -)"
    }

    private val numberPattern = Regex("""-?\d[\d,]*(?:\.\d+)?""")

    /** 「85 点」「約 3,000 円」: 数がちょうど 1 つの時だけその数。「85/100」のように 2 つあれば読まない。 */
    internal fun loneNumber(s: String): Double? {
        val hits = numberPattern.findAll(RecordImport.fullwidthToHalfwidth(s)).toList()
        if (hits.size != 1) return null
        return hits[0].value.replace(",", "").toDoubleOrNull()?.takeIf { it.isFinite() }
    }

    private val kanjiPattern = Regex(
        """^(?:約|およそ|だいたい)?(-)?(?:(\d+(?:\.\d+)?)億)?(?:(\d+(?:\.\d+)?)(千)?万)?(?:(\d+(?:\.\d+)?)千)?(\d+(?:\.\d+)?)?(?:円|人|点|件|個|回|本|枚|冊|歩|名|ドル|円くらい|円ほど|円程度|人程度|程度|くらい|ほど)?$""",
    )

    /** 万・千・億の読みの結果。[value] が null なら「あるが読めない」(空のまま)。 */
    internal class Kanji(val value: Double?)

    /**
     * 「3万円」「約1.5万人」「1億2000万」「3千万」「5千」: 万・千・億の数。
     * 万・千・億が無ければ null(ほかの読み方へ)。あって読めない(「3万〜5万」「3万円以上」)なら Kanji(null) = 空のまま。
     */
    internal fun kanjiNumber(s: String): Kanji? {
        var t = RecordImport.fullwidthToHalfwidth(s)
        if (t.none { it in "万千億" }) return null
        for (token in listOf(",", " ", "¥")) t = t.replace(token, "")
        val m = kanjiPattern.find(t) ?: return Kanji(null)
        fun num(i: Int): Double? = m.groups[i]?.value?.toDoubleOrNull()
        val oku = num(2)
        val man = num(3)
        val sen = num(5)
        val rest = num(6)
        if (oku == null && man == null && sen == null) return Kanji(null)
        val manScale = if (m.groups[4] == null) 1e4 else 1e7
        var n = (oku ?: 0.0) * 1e8 + (man ?: 0.0) * manScale + (sen ?: 0.0) * 1e3 + (rest ?: 0.0)
        if (m.groups[1] != null) n = -n
        return Kanji(n.takeIf { it.isFinite() })
    }

    private val isoDayPattern = Regex("""^([0-9]{4}-[0-9]{2}-[0-9]{2})T[0-9]{2}:[0-9]{2}(?::[0-9]{2}(?:\.[0-9]+)?)?(?:Z|[+-][0-9]{2}:?[0-9]{2})?$""")

    /** 「2026-11-20T23:30:00-05:00」の日付の部分(オフセットのまま)。正しい日でなければ null。 */
    internal fun isoDay(s: String): String? {
        val day = isoDayPattern.matchEntire(s)?.groupValues?.get(1) ?: return null
        return if (FieldCoding.strictDay(day) == null) null else day
    }

    /** 年が書いてあるか(4 桁の数)。 */
    internal fun hasYear(s: String): Boolean = Regex("""\d{4}""").containsMatchIn(s)

    /** 年なしの日付が今日より 30 日以上前なら来年(12 月に「1/10」→ 翌年 1 月)。そうでなければ null。 */
    internal fun nextYearIfPast(d: Instant, now: Instant): Instant? {
        if (!d.isBefore(now.minusSeconds(30L * 86_400))) return null
        val zone = FieldCoding.zone
        return LocalDateTime.ofInstant(d, zone).plusYears(1).atZone(zone).toInstant()
    }

    /** 「18時30分」「18時」→「18:30」「18:00」(全角数字も)。 */
    internal fun clock(s: String): String {
        var t = RecordImport.fullwidthToHalfwidth(s)
        t = Regex("""(\d{1,2})時(\d{1,2})分""").replace(t, "$1:$2")
        t = Regex("""(\d{1,2})時""").replace(t, "$1:00")
        return Regex("""日((?:\([^)]*\))?)(\d)""").replace(t, "日$1 $2")
    }

    /** 「18:00〜20:00」「11/20 - 11/22」は始まりだけ。 */
    internal fun rangeStart(s: String): String {
        for (sep in listOf("〜", "～", "~", " - ", "–")) {
            val i = s.indexOf(sep)
            if (i >= 0) return s.substring(0, i).trim()
        }
        return s
    }
}
