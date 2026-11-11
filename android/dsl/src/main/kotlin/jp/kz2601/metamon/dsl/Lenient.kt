@file:OptIn(ExperimentalSerializationApi::class)

package jp.kz2601.metamon.dsl

import kotlinx.serialization.ExperimentalSerializationApi
import kotlinx.serialization.KSerializer
import kotlinx.serialization.SerializationException
import kotlinx.serialization.builtins.ListSerializer
import kotlinx.serialization.descriptors.PrimitiveKind
import kotlinx.serialization.descriptors.PrimitiveSerialDescriptor
import kotlinx.serialization.descriptors.SerialDescriptor
import kotlinx.serialization.descriptors.nullable
import kotlinx.serialization.encoding.Decoder
import kotlinx.serialization.encoding.Encoder
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonDecoder
import kotlinx.serialization.json.JsonNull
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.booleanOrNull
import kotlinx.serialization.json.contentOrNull
import kotlinx.serialization.json.doubleOrNull
import kotlinx.serialization.json.longOrNull

// 生成した型(generated/)が使う寛容な読み方。iOS の Model/Decoding.swift(Lossy・Lenient)に当たる。

/**
 * DSL を読む Json。iOS と同じく「知らないキー・型の合わない値・null」で落とさない。
 * - 新しいキーは無視する
 * - 型の合わない値・null は既定値へ
 */
val DslJson: Json = Json {
    ignoreUnknownKeys = true
    coerceInputValues = true
    explicitNulls = false
}

/** 文字列の enum。知らない値は [fallback] に丸める(iOS の `Lenient.decode(..., fallback:)`)。 */
abstract class LenientEnumSerializer<T : Enum<T>>(
    serialName: String,
    private val entries: List<T>,
    private val wire: (T) -> String,
    private val fallback: T,
) : KSerializer<T> {
    override val descriptor: SerialDescriptor = PrimitiveSerialDescriptor(serialName, PrimitiveKind.STRING)

    override fun deserialize(decoder: Decoder): T {
        val raw = if (decoder is JsonDecoder) {
            (decoder.decodeJsonElement() as? JsonPrimitive)?.contentOrNull
        } else {
            decoder.decodeString()
        }
        return entries.firstOrNull { wire(it) == raw } ?: fallback
    }

    override fun serialize(encoder: Encoder, value: T) = encoder.encodeString(wire(value))
}

/** 配列の壊れた要素だけを捨てる(iOS の `@Lossy`)。配列でなければ空。 */
class LossyListSerializer<T>(private val element: KSerializer<T>) : KSerializer<List<T>> {
    private val delegate = ListSerializer(element)
    override val descriptor: SerialDescriptor = delegate.descriptor

    override fun deserialize(decoder: Decoder): List<T> {
        val json = decoder as? JsonDecoder ?: return delegate.deserialize(decoder)
        val array = json.decodeJsonElement() as? JsonArray ?: return emptyList()
        return array.mapNotNull { runCatching { json.json.decodeFromJsonElement(element, it) }.getOrNull() }
    }

    override fun serialize(encoder: Encoder, value: List<T>) = delegate.serialize(encoder, value)
}

/**
 * 省略できるキーの値 1 つ。型の合わない値は null にする(親の部品・entity ごと落とさない)。
 * iOS の `try? c.decodeIfPresent(...)` と同じ。
 */
abstract class LenientValueSerializer<T : Any>(serialName: String, kind: PrimitiveKind) : KSerializer<T?> {
    override val descriptor: SerialDescriptor = PrimitiveSerialDescriptor(serialName, kind).nullable

    protected abstract fun read(value: JsonPrimitive): T?

    protected abstract fun write(encoder: Encoder, value: T)

    override fun deserialize(decoder: Decoder): T? {
        val json = decoder as? JsonDecoder ?: throw SerializationException("DSL は JSON からだけ読める")
        val element = json.decodeJsonElement() as? JsonPrimitive ?: return null
        return if (element is JsonNull) null else read(element)
    }

    override fun serialize(encoder: Encoder, value: T?) {
        if (value == null) encoder.encodeNull() else write(encoder, value)
    }
}

object LenientStringSerializer : LenientValueSerializer<String>("jp.kz2601.metamon.dsl.LenientString", PrimitiveKind.STRING) {
    override fun read(value: JsonPrimitive): String? = if (value.isString) value.content else null
    override fun write(encoder: Encoder, value: String) = encoder.encodeString(value)
}

/** 整数。`3.0` のような整数の小数は受ける(iOS の `LossyPositiveInt` と同じ)。 */
object LenientIntSerializer : LenientValueSerializer<Int>("jp.kz2601.metamon.dsl.LenientInt", PrimitiveKind.INT) {
    override fun read(value: JsonPrimitive): Int? {
        if (value.isString) return null
        value.longOrNull?.let { return if (it in Int.MIN_VALUE..Int.MAX_VALUE) it.toInt() else null }
        val d = value.doubleOrNull ?: return null
        return if (d.isFinite() && d == Math.rint(d) && d >= Int.MIN_VALUE && d <= Int.MAX_VALUE) d.toInt() else null
    }

    override fun write(encoder: Encoder, value: Int) = encoder.encodeInt(value)
}

object LenientDoubleSerializer : LenientValueSerializer<Double>("jp.kz2601.metamon.dsl.LenientDouble", PrimitiveKind.DOUBLE) {
    override fun read(value: JsonPrimitive): Double? = if (value.isString) null else value.doubleOrNull
    override fun write(encoder: Encoder, value: Double) = encoder.encodeDouble(value)
}

object LenientBooleanSerializer : LenientValueSerializer<Boolean>("jp.kz2601.metamon.dsl.LenientBoolean", PrimitiveKind.BOOLEAN) {
    override fun read(value: JsonPrimitive): Boolean? = if (value.isString) null else value.booleanOrNull
    override fun write(encoder: Encoder, value: Boolean) = encoder.encodeBoolean(value)
}

/** 省略できるオブジェクト。形の合わない値は null にする(iOS の `try? c.decodeIfPresent(Theme.self, ...)`)。 */
open class LenientObjectSerializer<T : Any>(private val inner: KSerializer<T>) : KSerializer<T?> {
    override val descriptor: SerialDescriptor = inner.descriptor.nullable

    override fun deserialize(decoder: Decoder): T? {
        val json = decoder as? JsonDecoder ?: throw SerializationException("DSL は JSON からだけ読める")
        val element = json.decodeJsonElement() as? JsonObject ?: return null
        return runCatching { json.json.decodeFromJsonElement(inner, element) }.getOrNull()
    }

    override fun serialize(encoder: Encoder, value: T?) {
        if (value == null) encoder.encodeNull() else encoder.encodeSerializableValue(inner, value)
    }
}

/** 中身を型にしないオブジェクト(`additionalProperties` だけのもの)。オブジェクトでなければ null。 */
object LenientJsonObjectSerializer : KSerializer<JsonObject?> {
    override val descriptor: SerialDescriptor = JsonObject.serializer().descriptor.nullable

    override fun deserialize(decoder: Decoder): JsonObject? {
        val json = decoder as? JsonDecoder ?: throw SerializationException("DSL は JSON からだけ読める")
        return json.decodeJsonElement() as? JsonObject
    }

    override fun serialize(encoder: Encoder, value: JsonObject?) {
        if (value == null) encoder.encodeNull() else encoder.encodeSerializableValue(JsonObject.serializer(), value)
    }
}
