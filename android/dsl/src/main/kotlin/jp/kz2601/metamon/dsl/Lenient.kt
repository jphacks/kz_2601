package jp.kz2601.metamon.dsl

import kotlinx.serialization.KSerializer
import kotlinx.serialization.builtins.ListSerializer
import kotlinx.serialization.descriptors.PrimitiveKind
import kotlinx.serialization.descriptors.PrimitiveSerialDescriptor
import kotlinx.serialization.descriptors.SerialDescriptor
import kotlinx.serialization.encoding.Decoder
import kotlinx.serialization.encoding.Encoder
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonDecoder
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.contentOrNull

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
