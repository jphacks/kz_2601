package jp.kz2601.metamon.dsl.records

import jp.kz2601.metamon.dsl.expr.SwiftNumbers
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonNull
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.booleanOrNull
import kotlinx.serialization.json.doubleOrNull

/** 形の壊れた DSL の値(iOS の DecodingError)。呼び出し側は runCatching で握る。 */
class DslDecodeException(message: String) : IllegalArgumentException(message)

/**
 * 任意の JSON(iOS の `JSONValue`)。レコードの値・filter の値・式のリテラルの共通表現。
 * 数値は Double。`0.0 == -0.0`、NaN は自分とも等しくない(Swift の Double と同じ)。
 */
sealed class JSONValue {
    data class Str(val value: String) : JSONValue()

    class Num(val value: Double) : JSONValue() {
        override fun equals(other: Any?): Boolean = other is Num && value == other.value
        override fun hashCode(): Int = if (value == 0.0) 0 else value.hashCode()
        override fun toString(): String = "Num($value)"
    }

    data class Bool(val value: Boolean) : JSONValue()
    data object Null : JSONValue()
    data class Arr(val items: List<JSONValue>) : JSONValue()
    data class Obj(val fields: Map<String, JSONValue>) : JSONValue()

    val stringValue: String? get() = (this as? Str)?.value
    val doubleValue: Double? get() = (this as? Num)?.value
    val boolValue: Boolean? get() = (this as? Bool)?.value
    val arrayValue: List<JSONValue>? get() = (this as? Arr)?.items
    val objectValue: Map<String, JSONValue>? get() = (this as? Obj)?.fields
    val isNull: Boolean get() = this === Null

    /** テンプレート埋め込み・表示用。整数(1e15 未満)は小数点なし、それ以外は Swift の Double の書き方。 */
    val displayString: String
        get() = when (this) {
            is Str -> value
            is Num -> numberText(value)
            is Bool -> if (value) "true" else "false"
            Null -> ""
            is Arr -> items.joinToString(", ") { it.displayString }
            is Obj -> Json.encodeToString(JsonElement.serializer(), toJsonElement())
        }

    /** `visible` / `disabled` 判定用の真偽値化。 */
    val truthy: Boolean
        get() = when (this) {
            Null -> false
            is Bool -> value
            is Num -> value != 0.0
            is Str -> value.isNotEmpty() && value != "false"
            is Arr -> items.isNotEmpty()
            is Obj -> true
        }

    fun toJsonElement(): JsonElement = when (this) {
        is Str -> JsonPrimitive(value)
        is Num -> JsonPrimitive(value)
        is Bool -> JsonPrimitive(value)
        Null -> JsonNull
        is Arr -> JsonArray(items.map { it.toJsonElement() })
        is Obj -> JsonObject(fields.mapValues { it.value.toJsonElement() })
    }

    companion object {
        fun string(s: String): JSONValue = Str(s)
        fun number(n: Double): JSONValue = Num(n)
        fun number(n: Int): JSONValue = Num(n.toDouble())
        fun bool(b: Boolean): JSONValue = Bool(b)

        /** kotlinx の JSON 木 → JSONValue。 */
        fun from(e: JsonElement): JSONValue = when (e) {
            JsonNull -> Null
            is JsonPrimitive -> when {
                e.isString -> Str(e.content)
                e.booleanOrNull != null -> Bool(e.booleanOrNull!!)
                else -> e.doubleOrNull?.let { Num(it) } ?: Str(e.content)
            }
            is JsonArray -> Arr(e.map { from(it) })
            is JsonObject -> Obj(e.mapValues { from(it.value) })
        }

        fun parse(json: String): JSONValue = from(Json.parseToJsonElement(json))

        /** テスト・モック用: Kotlin の値から作る(String / Number / Boolean / null / List / Map)。 */
        fun of(v: Any?): JSONValue = when (v) {
            null -> Null
            is JSONValue -> v
            is String -> Str(v)
            is Number -> Num(v.toDouble())
            is Boolean -> Bool(v)
            is List<*> -> Arr(v.map { of(it) })
            is Map<*, *> -> Obj(v.entries.associate { it.key.toString() to of(it.value) })
            else -> throw IllegalArgumentException("JSONValue.of: ${v::class}")
        }

        /** 数値の表示(iOS の `JSONValue.number(n).displayString`)。 */
        fun numberText(n: Double): String =
            if (n == SwiftNumbers.rounded(n) && kotlin.math.abs(n) < 1e15) n.toLong().toString() else SwiftNumbers.description(n)
    }
}
