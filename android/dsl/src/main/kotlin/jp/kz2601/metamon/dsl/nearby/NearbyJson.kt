package jp.kz2601.metamon.dsl.nearby

import jp.kz2601.metamon.dsl.records.DslDecodeException
import jp.kz2601.metamon.dsl.records.JSONValue
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonNull
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.booleanOrNull
import kotlinx.serialization.json.doubleOrNull
import java.time.Instant
import java.time.OffsetDateTime
import java.time.format.DateTimeFormatter
import java.time.temporal.ChronoUnit

/**
 * 近距離共有の JSON。iOS の `JSONEncoder(.sortedKeys)` と同じ形・同じバイト数で書く(OS をまたいで読めるように・16 KB 判定がずれないように)。
 * - キーは名前順、`/` は `\/`、制御文字はエスケープ、整数の Double は小数点なし
 * - 読む側は iOS の合成 Decodable と同じ: 必須キーが無い・型が違うと例外、未知のキーは無視
 */
internal object NearbyJson {
    fun write(e: JsonElement): String = StringBuilder().also { write(e, it) }.toString()

    fun bytes(e: JsonElement): ByteArray = write(e).toByteArray(Charsets.UTF_8)

    fun parse(data: ByteArray): JsonElement =
        runCatching { Json.parseToJsonElement(data.toString(Charsets.UTF_8)) }.getOrElse { throw DslDecodeException("JSON が読めない") }

    private fun write(e: JsonElement, out: StringBuilder) {
        when (e) {
            JsonNull -> out.append("null")
            is JsonPrimitive -> when {
                e.isString -> string(e.content, out)
                e.booleanOrNull != null -> out.append(e.content)
                else -> out.append(JSONValue.numberText(e.doubleOrNull ?: throw DslDecodeException("数値でない: ${e.content}")))
            }
            is JsonArray -> {
                out.append('[')
                e.forEachIndexed { i, v -> if (i > 0) out.append(','); write(v, out) }
                out.append(']')
            }
            is JsonObject -> {
                out.append('{')
                e.keys.sortedWith(CodePointOrder).forEachIndexed { i, k ->
                    if (i > 0) out.append(',')
                    string(k, out)
                    out.append(':')
                    write(e.getValue(k), out)
                }
                out.append('}')
            }
        }
    }

    private fun string(s: String, out: StringBuilder) {
        out.append('"')
        for (c in s) {
            when (c) {
                '"' -> out.append("\\\"")
                '\\' -> out.append("\\\\")
                '/' -> out.append("\\/")
                '\n' -> out.append("\\n")
                '\r' -> out.append("\\r")
                '\t' -> out.append("\\t")
                '\b' -> out.append("\\b")
                '\u000C' -> out.append("\\f")
                else -> if (c < ' ') out.append("\\u%04x".format(c.code)) else out.append(c)
            }
        }
        out.append('"')
    }

    // MARK: - 読む(iOS の合成 Decodable と同じ厳しさ)

    fun obj(e: JsonElement?, what: String): JsonObject = e as? JsonObject ?: throw DslDecodeException("$what: object でない")

    fun str(o: JsonObject, key: String): String = optStr(o, key) ?: throw DslDecodeException("$key が無い")

    fun optStr(o: JsonObject, key: String): String? = when (val v = o[key]) {
        null, JsonNull -> null
        is JsonPrimitive -> if (v.isString) v.content else throw DslDecodeException("$key: 文字列でない")
        else -> throw DslDecodeException("$key: 文字列でない")
    }

    fun int(o: JsonObject, key: String): Int {
        val v = o[key] as? JsonPrimitive ?: throw DslDecodeException("$key が無い")
        val d = v.takeIf { !it.isString }?.doubleOrNull ?: throw DslDecodeException("$key: 数値でない")
        if (d != Math.rint(d) || d < Int.MIN_VALUE || d > Int.MAX_VALUE) throw DslDecodeException("$key: 整数でない")
        return d.toInt()
    }

    fun bool(o: JsonObject, key: String): Boolean {
        val v = o[key] as? JsonPrimitive ?: throw DslDecodeException("$key が無い")
        return v.takeIf { !it.isString }?.booleanOrNull ?: throw DslDecodeException("$key: 真偽値でない")
    }

    fun array(o: JsonObject, key: String): JsonArray = o[key] as? JsonArray ?: throw DslDecodeException("$key: 配列でない")

    fun record(e: JsonElement?, what: String): Map<String, JSONValue> = obj(e, what).mapValues { JSONValue.from(it.value) }

    fun record(data: Map<String, JSONValue>): JsonObject = JsonObject(data.mapValues { it.value.toJsonElement() })

    // MARK: - 日時(iOS の `.iso8601`: 秒まで・UTC)

    fun date(t: Instant): JsonPrimitive = JsonPrimitive(DateTimeFormatter.ISO_INSTANT.format(t.truncatedTo(ChronoUnit.SECONDS)))

    fun date(o: JsonObject, key: String): Instant = runCatching { OffsetDateTime.parse(str(o, key)).toInstant() }
        .getOrElse { throw DslDecodeException("$key: 日時でない") }

    /** Swift の String の大小(Unicode スカラー順)。UTF-16 の順とはサロゲートで違う。 */
    val CodePointOrder: Comparator<String> = Comparator { a, b ->
        var i = 0
        var j = 0
        while (i < a.length && j < b.length) {
            val ca = a.codePointAt(i)
            val cb = b.codePointAt(j)
            if (ca != cb) return@Comparator ca.compareTo(cb)
            i += Character.charCount(ca)
            j += Character.charCount(cb)
        }
        (a.length - i).compareTo(b.length - j)
    }
}
