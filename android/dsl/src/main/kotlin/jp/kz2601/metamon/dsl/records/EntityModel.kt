package jp.kz2601.metamon.dsl.records

import jp.kz2601.metamon.dsl.expr.Expr
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonNull
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.booleanOrNull
import kotlinx.serialization.json.doubleOrNull

// 集計・式が読む entity / field / filter / sort(iOS の Model/AppDocument.swift・Component.swift の該当部分)。
// :dsl の AppDSL は entities を JsonObject のまま持つので、ここで読む(Entity.from)。
// 生成器(#351)の型ができたら、その型からこの型へ写すだけにする。

/** Field.type。知らない値は STRING(iOS の Lenient fallback)。 */
enum class FieldType(val wire: String) {
    STRING("string"), TEXT("text"), INTEGER("integer"), NUMBER("number"), BOOLEAN("boolean"),
    DATE("date"), DATETIME("datetime"), ENUM("enum"), CURRENCY("currency"), PHOTO("photo"), LOCATION("location"),
    VIDEO("video"), AUDIO("audio"), REF("ref"), REFS("refs"), MEMBER("member"), MEMBERS("members");

    val isNumeric: Boolean get() = this == INTEGER || this == NUMBER || this == CURRENCY
    val isDate: Boolean get() = this == DATE || this == DATETIME

    companion object {
        fun of(wire: String?): FieldType = entries.firstOrNull { it.wire == wire } ?: STRING
    }
}

data class Field(
    val id: String,
    val name: String = id,
    val type: FieldType = FieldType.STRING,
    val required: Boolean? = null,
    val options: List<String>? = null,
    /** リテラル既定値。date / datetime は "now" 可。 */
    val defaultValue: JSONValue? = null,
    val unit: String? = null,
    /** type=ref / refs の参照先 entity id。 */
    val refEntity: String? = null,
    /** 読み取り専用の算出フィールド(#101)。Record には保存しない。 */
    val computed: Expr? = null,
    val health: String? = null,
    /** 回答者に見せない項目(#262)。 */
    val hidden: Boolean = false,
    /** "admins" = 管理者だけが変えられる項目(#272)。 */
    val editableBy: String? = null,
) {
    val isComputed: Boolean get() = computed != null
    val isAdminOnly: Boolean get() = editableBy == "admins"

    companion object {
        /** iOS の `Field.init(from:)`。id が無ければ例外、他のキーは型が合わなければ無視。 */
        fun from(e: JsonElement): Field {
            val o = e as? JsonObject ?: throw DslDecodeException("Field: object でない")
            val id = o.str("id") ?: throw DslDecodeException("Field: id が無い")
            return Field(
                id = id,
                name = o.strStrict("name") ?: id,
                type = FieldType.of(o.str("type")),
                required = o.bool("required"),
                options = (o["options"] as? JsonArray)?.let { a ->
                    if (a.all { it is JsonPrimitive && it.isString }) a.map { (it as JsonPrimitive).content } else null
                },
                defaultValue = o["defaultValue"]?.takeIf { it != JsonNull }?.let { JSONValue.from(it) },
                unit = o.str("unit"),
                refEntity = o.str("refEntity"),
                computed = o["computed"]?.takeIf { it != JsonNull }?.let { runCatching { Expr.from(it) }.getOrNull() },
                health = o.str("health"),
                hidden = o.bool("hidden") ?: false,
                editableBy = o.str("editableBy"),
            )
        }
    }
}

/** 共有 entity の「自分だけの印」1 つ(#255)。 */
data class Mark(val id: String, val icon: String, val label: String) {
    companion object {
        fun from(e: JsonElement): Mark {
            val o = e as? JsonObject ?: throw DslDecodeException("Mark: object でない")
            return Mark(
                o.str("id") ?: throw DslDecodeException("Mark: id"),
                o.str("icon") ?: throw DslDecodeException("Mark: icon"),
                o.str("label") ?: throw DslDecodeException("Mark: label"),
            )
        }
    }
}

data class Entity(
    val id: String,
    val name: String = id,
    val titleField: String? = null,
    /** "private"(省略時)/ "shared" / "public"。未知の値は private 扱い。 */
    val scope: String? = null,
    /** 共有レコードを誰が更新できるか(#252): "author"(省略時)/ "members"。 */
    val edit: String? = null,
    val marks: List<Mark> = emptyList(),
    /** 記録の保存期間(日数・#482)。正の整数だけ。 */
    val retentionDays: Int? = null,
    val match: String? = null,
    /** 設定値の置き場(#267)。レコードは常に 1 件。 */
    val single: Boolean = false,
    val unique: List<String>? = null,
    /** 共有レコードを誰が消せるか(#272): "author" / "admins" / "authorOrAdmins"。 */
    val deleteBy: String? = null,
    val addressedTo: String? = null,
    val fields: List<Field> = emptyList(),
) {
    /** field id、または予約値(`$author` / `$createdAt` / `$marks.<id>`)の仮 Field。 */
    fun field(id: String): Field? = fields.firstOrNull { it.id == id } ?: RecordValue.field(id, this)

    fun mark(id: String): Mark? = marks.firstOrNull { it.id == id }

    val isShared: Boolean get() = scope == "shared" || scope == "public"
    val membersCanEdit: Boolean get() = isShared && edit == "members"
    val isMatchProfile: Boolean get() = isShared && match == "profile"
    val isMatchMatched: Boolean get() = isShared && match == "matched"

    val storedFields: List<Field> get() = fields.filter { !it.isComputed }
    val computedFields: List<Field> get() = fields.filter { it.isComputed }

    /** 一覧の見出しに使う field(titleField → 最初の string field → 最初の field)。hidden は除く。 */
    val displayTitleField: String?
        get() {
            val visible = storedFields.filter { !it.hidden }
            if (titleField != null && field(titleField)?.hidden != true) return titleField
            return visible.firstOrNull { it.type == FieldType.STRING }?.id ?: visible.firstOrNull()?.id
        }

    val hiddenFieldIds: Set<String> get() = fields.filter { it.hidden }.map { it.id }.toSet()

    companion object {
        /** iOS の Entity デコード(合成 Decodable と同じ)。id・name・fields(配列)が必須、文字列の型違いは例外、fields の壊れた要素は捨てる。 */
        fun from(e: JsonElement): Entity {
            val o = e as? JsonObject ?: throw DslDecodeException("Entity: object でない")
            val id = o.str("id") ?: throw DslDecodeException("Entity: id が無い")
            val fieldsJson = o["fields"] as? JsonArray ?: throw DslDecodeException("Entity: fields が無い")
            return Entity(
                id = id,
                name = o.strStrict("name") ?: throw DslDecodeException("Entity: name が無い"),
                titleField = o.strStrict("titleField"),
                scope = o.strStrict("scope"),
                edit = o.strStrict("edit"),
                marks = (o["marks"] as? JsonArray)?.mapNotNull { runCatching { Mark.from(it) }.getOrNull() } ?: emptyList(),
                retentionDays = positiveInt(o["retentionDays"]),
                match = o.strStrict("match"),
                single = o.bool("single") ?: false,
                unique = o["unique"]?.takeIf { it != JsonNull }?.let { u ->
                    val a = u as? JsonArray ?: throw DslDecodeException("Entity: unique")
                    a.map { (it as? JsonPrimitive)?.takeIf { p -> p.isString }?.content ?: throw DslDecodeException("Entity: unique") }
                },
                deleteBy = o.strStrict("deleteBy"),
                addressedTo = o.strStrict("addressedTo"),
                fields = fieldsJson.mapNotNull { runCatching { Field.from(it) }.getOrNull() },
            )
        }

        fun parse(json: String): Entity = from(Json.parseToJsonElement(json))

        /** AppDSL.entities(JsonObject のまま)から読む。壊れた entity は捨てる。 */
        fun listFrom(entities: List<JsonObject>): List<Entity> = entities.mapNotNull { runCatching { from(it) }.getOrNull() }

        /** 正の整数だけ(#482 LossyPositiveInt)。30.0 は 30、1.5・0・負・文字列は null。 */
        private fun positiveInt(e: JsonElement?): Int? {
            val p = e as? JsonPrimitive ?: return null
            if (p.isString) return null
            val d = p.doubleOrNull ?: return null
            if (!d.isFinite() || d != Math.rint(d) || d > Int.MAX_VALUE) return null
            return d.toInt().takeIf { it > 0 }
        }
    }
}

/** list の並べ替え。 */
data class Sort(val field: String, val direction: String) {
    companion object {
        fun from(e: JsonElement): Sort? {
            val o = e as? JsonObject ?: return null
            return Sort(o.str("field") ?: return null, o.str("direction") ?: return null)
        }
    }
}

/**
 * list / stat / chart / agg の絞り込み。`value` はリテラル(後方互換)か Expr(#101: `{"ref":"id"}`)。
 * Expr の時は value が null で [valueExpr] に入り、評価器が外側の行で評価してから照合する。
 */
data class Filter(
    val field: String,
    /** eq|neq|gt|gte|lt|lte|today|thisWeek|thisMonth|thisYear|contains */
    val op: String,
    val value: JSONValue? = null,
    val valueExpr: Expr? = null,
) {
    /** `{"me": true}` = サインイン中の人(field `$author` / `$marks.<id>` 専用・#251)。 */
    val isMe: Boolean get() = value == meValue

    /** `{"myFollows": true}` = フォローしている人(#270)。 */
    val isMyFollows: Boolean get() = value == myFollowsValue

    /** `{"me": true}` を uid に置き換える。uid が分からない時は空文字。 */
    fun resolvingMe(uid: String?): Filter = if (isMe) Filter(field, op, JSONValue.Str(uid ?: "")) else this

    /** `{"myFollows": true}` をフォローしている uid の配列に置き換える。uids が null ならそのまま(= どの行にも一致しない)。 */
    fun resolvingFollows(uids: Set<String>?): Filter {
        if (!isMyFollows || uids == null) return this
        return Filter(field, op, JSONValue.Arr(uids.sorted().map { JSONValue.Str(it) }))
    }

    /** `resolvingFollows` 後の uid の集合(それ以外の filter は null)。 */
    val followedAuthors: Set<String>?
        get() {
            if (this.field != RecordValue.AUTHOR) return null
            val items = (value as? JSONValue.Arr)?.items ?: return null
            return items.mapNotNull { it.stringValue }.toSet()
        }

    fun toJsonElement(): JsonElement = JsonObject(
        mapOf(
            "field" to JsonPrimitive(field),
            "op" to JsonPrimitive(op),
            "value" to (valueExpr?.toJsonElement() ?: (value ?: JSONValue.Null).toJsonElement()),
        ),
    )

    companion object {
        val meValue: JSONValue = JSONValue.Obj(mapOf("me" to JSONValue.Bool(true)))
        val myFollowsValue: JSONValue = JSONValue.Obj(mapOf("myFollows" to JSONValue.Bool(true)))

        /** Expr を値に持つ filter。リテラルの Expr は value に入れる。 */
        fun withExpr(field: String, op: String, valueExpr: Expr): Filter {
            val lit = valueExpr.literalValue
            return if (lit != null) Filter(field, op, lit) else Filter(field, op, null, valueExpr)
        }

        /** iOS の `Filter.init(from:)`。field / op が無い・value が式として読めない時は例外。 */
        fun from(e: JsonElement): Filter {
            val o = e as? JsonObject ?: throw DslDecodeException("Filter: object でない")
            val field = o.strStrict("field") ?: throw DslDecodeException("Filter: field が無い")
            val op = o.strStrict("op") ?: throw DslDecodeException("Filter: op が無い")
            val v = o["value"]
            if (v == null || v == JsonNull) return Filter(field, op)
            when (boolMap(v)) {
                mapOf("me" to true) -> return Filter(field, op, meValue)
                mapOf("myFollows" to true) -> return Filter(field, op, myFollowsValue)
            }
            return withExpr(field, op, Expr.from(v))
        }

        fun parse(json: String): Filter = from(Json.parseToJsonElement(json))

        /** `filter` キーが単体でも配列(AND)でも読む。壊れていれば空(= 絞り込み無し)。 */
        fun oneOrMany(e: JsonElement?): List<Filter> {
            if (e == null || e == JsonNull) return emptyList()
            runCatching { return listOf(from(e)) }
            val a = e as? JsonArray ?: return emptyList()
            return a.mapNotNull { runCatching { from(it) }.getOrNull() }
        }

        /** `[String: Bool]` として読めるか(Swift の JSONDecoder と同じく値は真偽値だけ)。 */
        private fun boolMap(e: JsonElement): Map<String, Boolean>? {
            val o = e as? JsonObject ?: return null
            val out = mutableMapOf<String, Boolean>()
            for ((k, v) in o) {
                val p = v as? JsonPrimitive ?: return null
                if (p.isString) return null
                out[k] = p.booleanOrNull ?: return null
            }
            return out
        }
    }
}

// MARK: - JsonObject の寛容な読み取り

/** 文字列なら値。無い・null・型違いは null。 */
internal fun JsonObject.str(key: String): String? = (this[key] as? JsonPrimitive)?.takeIf { it.isString }?.content

/** 文字列なら値。無い・null は null、型違いは例外(Swift の decodeIfPresent)。 */
internal fun JsonObject.strStrict(key: String): String? {
    val v = this[key] ?: return null
    if (v == JsonNull) return null
    val p = v as? JsonPrimitive
    if (p == null || !p.isString) throw DslDecodeException("$key: 文字列でない")
    return p.content
}

internal fun JsonObject.bool(key: String): Boolean? = (this[key] as? JsonPrimitive)?.takeIf { !it.isString }?.booleanOrNull
