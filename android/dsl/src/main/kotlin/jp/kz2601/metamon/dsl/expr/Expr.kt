package jp.kz2601.metamon.dsl.expr

import jp.kz2601.metamon.dsl.records.DslDecodeException
import jp.kz2601.metamon.dsl.records.Filter
import jp.kz2601.metamon.dsl.records.JSONValue
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonNull
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive

/**
 * 計算式(#101・backend の expreval と共通契約)。iOS の Logic/Expr.swift。JSON 表現:
 * ```
 * Expr := number | string | boolean
 *       | {"ref": "<fieldId>"}                       行のフィールド値("id" でレコード id)
 *       | {"ref": "<refField>.<fieldId>"}            ref field が指す先のレコードの値(1 段・#134)
 *       | {"agg": "count|sum|avg|min|max|latest|earliest", "entity": "...", "field"?, "by"?, "filter"?: Filter | [Filter]}
 *       | {"op": "<name>", "args": [Expr, ...]}
 *       | {"if": Expr, "then": Expr, "else": Expr}
 *       | {"setting": "<fieldId>"}                    single の entity の 1 件のフィールド値(#267)
 * ```
 * 正典は docs/dsl/expressions.md。リテラルの null は寛容に受ける。形の壊れた式は [from] が例外を投げる。
 */
sealed class Expr {
    data class Literal(val value: JSONValue) : Expr()
    data class Ref(val id: String) : Expr()
    data class Agg(val agg: AggExpr) : Expr()
    data class Op(val name: String, val args: List<Expr>) : Expr()
    data class Cond(val cond: Expr, val then: Expr, val otherwise: Expr) : Expr()
    data class Setting(val id: String) : Expr()

    /** リテラルならその値(Filter.value の後方互換)。 */
    val literalValue: JSONValue? get() = (this as? Literal)?.value

    /** 式が参照する entity id(agg の entity と filter.value の中)。 */
    val referencedEntities: Set<String>
        get() = when (this) {
            is Literal, is Ref, is Setting -> emptySet()
            is Agg -> agg.filters.fold(setOf(agg.entity)) { s, f -> s + (f.valueExpr?.referencedEntities ?: emptySet()) }
            is Op -> args.fold(emptySet()) { s, a -> s + a.referencedEntities }
            is Cond -> cond.referencedEntities + then.referencedEntities + otherwise.referencedEntities
        }

    fun toJsonElement(): JsonElement = when (this) {
        is Literal -> value.toJsonElement()
        is Ref -> JsonObject(mapOf("ref" to JsonPrimitive(id)))
        is Agg -> JsonObject(
            buildMap {
                put("agg", JsonPrimitive(agg.op))
                put("entity", JsonPrimitive(agg.entity))
                agg.field?.let { put("field", JsonPrimitive(it)) }
                agg.by?.let { put("by", JsonPrimitive(it)) }
                if (agg.filters.size == 1) put("filter", agg.filters[0].toJsonElement())
                else if (agg.filters.isNotEmpty()) put("filter", JsonArray(agg.filters.map { it.toJsonElement() }))
            },
        )
        is Op -> JsonObject(mapOf("op" to JsonPrimitive(name), "args" to JsonArray(args.map { it.toJsonElement() })))
        is Cond -> JsonObject(mapOf("if" to cond.toJsonElement(), "then" to then.toJsonElement(), "else" to otherwise.toJsonElement()))
        is Setting -> JsonObject(mapOf("setting" to JsonPrimitive(id)))
    }

    companion object {
        val NULL: Expr = Literal(JSONValue.Null)

        fun parse(json: String): Expr = from(Json.parseToJsonElement(json))

        /** iOS の `Expr.init(from:)` と同じ規則で読む。読めなければ [DslDecodeException]。 */
        fun from(e: JsonElement): Expr {
            when (e) {
                JsonNull -> return NULL
                is JsonPrimitive -> return Literal(JSONValue.from(e))
                is JsonArray -> throw DslDecodeException("Expr: 配列は式でない")
                is JsonObject -> {}
            }
            val o = e as JsonObject
            str(o, "ref")?.let { return Ref(it) }
            str(o, "agg")?.let { a ->
                return Agg(
                    AggExpr(
                        op = a,
                        entity = str(o, "entity") ?: throw DslDecodeException("Expr: agg に entity が無い"),
                        field = str(o, "field"),
                        by = str(o, "by"),
                        filters = Filter.oneOrMany(o["filter"]),
                    ),
                )
            }
            str(o, "op")?.let { name ->
                val args = when (val a = o["args"]) {
                    null, JsonNull -> emptyList()
                    is JsonArray -> a.map { from(it) }
                    else -> throw DslDecodeException("Expr: args が配列でない")
                }
                return Op(name, args)
            }
            str(o, "setting")?.let { return Setting(it) }
            if (o.containsKey("if")) {
                return Cond(
                    from(o.getValue("if")),
                    o["then"]?.let { from(it) } ?: NULL,
                    o["else"]?.let { from(it) } ?: NULL,
                )
            }
            throw DslDecodeException("Expr: ref/agg/op/if/setting のいずれも無い")
        }

        /** decodeIfPresent(String): 無い・null は null、文字列でなければ例外。 */
        private fun str(o: JsonObject, key: String): String? {
            val v = o[key] ?: return null
            if (v == JsonNull) return null
            val p = v as? JsonPrimitive
            if (p == null || !p.isString) throw DslDecodeException("Expr: $key が文字列でない")
            return p.content
        }
    }
}

data class AggExpr(
    /** count|sum|avg|min|max|latest|earliest */
    val op: String,
    val entity: String,
    val field: String? = null,
    /** latest / earliest の並べ替えに使う日付 field(#133)。 */
    val by: String? = null,
    /** AND で全部満たすレコードだけ集計する。 */
    val filters: List<Filter> = emptyList(),
)
