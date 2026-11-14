package jp.kz2601.metamon.dsl

import jp.kz2601.metamon.dsl.ai.AiSaveTo
import jp.kz2601.metamon.dsl.records.JSONValue
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonNull
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.doubleOrNull
import jp.kz2601.metamon.dsl.expr.Expr as EvalExpr
import jp.kz2601.metamon.dsl.records.Filter as RecordFilter

// 生成した型で JsonElement のまま残るキー(形が 2 通りある)の読み口(#1007)。
// 読み方はほかの寛容な読み方と同じ: 形が違えば null / 空にして、部品ごと落とさない。

/** `target` が数のリテラルならその数(`{"setting"}` などの式・文字列なら null)。 */
val Component.targetNumber: Double?
    get() = (target as? JsonPrimitive)?.takeIf { !it.isString }?.doubleOrNull

/** `target` が式({"setting"} など)ならその式。数のリテラル・null・読めない式は null。 */
val Component.targetExpr: EvalExpr?
    get() {
        val t = target ?: return null
        if (t == JsonNull || targetNumber != null) return null
        return runCatching { EvalExpr.from(t) }.getOrNull()
    }

/** aiButton の `saveTo`(`{entity, field}`)。形が違えば null。 */
val Component.aiSaveTo: AiSaveTo?
    get() = AiSaveTo.from(saveTo)

/** aiChat の `saveTo`(`{entity, fields, …}`。aiButton と同じキー)。形が違えば null。 */
val Component.chatSaveTo: AIChatSaveTo?
    get() {
        val o = saveTo as? JsonObject ?: return null
        if (o["fields"] == null) return null
        return runCatching { DslJson.decodeFromJsonElement(AIChatSaveTo.serializer(), o) }.getOrNull()
    }

/** field の `defaultValue`(どの型の値でもよい)。無い・null なら null。 */
val Field.defaultJSONValue: JSONValue?
    get() = DslReaders.defaultValue(defaultValue)

/** 集計の `filter`。単体でも配列(AND)でも読む。壊れていれば空(= 絞り込み無し)。 */
val ExprAgg.filters: List<RecordFilter>
    get() = RecordFilter.oneOrMany(filter)

object DslReaders {
    /** `defaultValue` の JSON → 値。無い・null なら null。 */
    fun defaultValue(e: JsonElement?): JSONValue? = e?.takeIf { it != JsonNull }?.let { JSONValue.from(it) }
}
