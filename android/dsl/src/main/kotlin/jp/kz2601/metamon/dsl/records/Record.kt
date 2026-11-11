package jp.kz2601.metamon.dsl.records

import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonNull
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.booleanOrNull
import kotlinx.serialization.json.doubleOrNull

/** 1 件のレコードの印の状態(appconfig `MarkState`)。 */
data class MarkState(val count: Int, val mine: Boolean) {
    /** 押した直後の楽観的な状態(件数は 0 未満にしない)。 */
    fun toggled(): MarkState = MarkState(maxOf(0, count + if (mine) -1 else 1), !mine)
}

/**
 * 生成アプリのユーザーデータ 1 件(appconfig `Record`: {id, entity, data, author?, authorName?, createdAt, updatedAt, marks?})。
 * `data` のキーは Field.id。iOS の Records/Record.swift。
 */
data class Record(
    val id: String,
    val data: Map<String, JSONValue> = emptyMap(),
    val createdAt: String? = null,
    val updatedAt: String? = null,
    /** 作った人の uid(shared / public の entity だけ)。未送信の自分のレコードは null。 */
    val author: String? = null,
    /** 投稿者の表示名(shared / public)。 */
    val authorName: String? = null,
    /** 印の状態(#255)。null = サーバーが返していない。 */
    val marks: Map<String, MarkState>? = null,
    /** 端末で作ってまだサーバーに届いていない。ローカル DB の行だけが立てる。 */
    val unsent: Boolean = false,
) {
    /** field の値。`$author` は表示名、`$createdAt` は作成日時、`$marks.<id>` は印の件数。無ければ Null。 */
    operator fun get(field: String): JSONValue {
        RecordValue.markId(field)?.let { return JSONValue.Num(markState(it).count.toDouble()) }
        return when (field) {
            RecordValue.AUTHOR -> authorName?.let { JSONValue.Str(it) } ?: JSONValue.Null
            RecordValue.CREATED_AT -> createdAt?.let { JSONValue.Str(it) } ?: JSONValue.Null
            else -> data[field] ?: JSONValue.Null
        }
    }

    /** 印の状態(サーバーが付けていなければ 0 件・未)。 */
    fun markState(id: String): MarkState = marks?.get(id) ?: MarkState(0, false)

    companion object {
        /** iOS の `Record.init(from:)`。id 必須、data は object(無ければ空)、他は型違いを無視。 */
        fun from(e: JsonElement): Record {
            val o = e as? JsonObject ?: throw DslDecodeException("Record: object でない")
            val id = o.strStrict("id") ?: throw DslDecodeException("Record: id が無い")
            val data = when (val d = o["data"]) {
                null, JsonNull -> emptyMap()
                is JsonObject -> d.mapValues { JSONValue.from(it.value) }
                else -> throw DslDecodeException("Record: data が object でない")
            }
            return Record(
                id = id,
                data = data,
                createdAt = o.str("createdAt"),
                updatedAt = o.str("updatedAt"),
                author = o.str("author"),
                authorName = o.str("authorName"),
                marks = marks(o["marks"]),
            )
        }

        fun parse(json: String): Record = from(Json.parseToJsonElement(json))

        private fun marks(e: JsonElement?): Map<String, MarkState>? {
            val o = e as? JsonObject ?: return null
            val out = mutableMapOf<String, MarkState>()
            for ((k, v) in o) {
                val m = v as? JsonObject ?: return null
                val c = (m["count"] as? JsonPrimitive)?.takeIf { !it.isString }?.doubleOrNull ?: return null
                val mine = (m["mine"] as? JsonPrimitive)?.takeIf { !it.isString }?.booleanOrNull ?: return null
                if (c != Math.rint(c)) return null
                out[k] = MarkState(c.toInt(), mine)
            }
            return out
        }
    }
}

/**
 * field id の代わりに書ける、どのレコードにもある値(#250 #251・schema の `x-recordValues`)。
 * - `$author`: 投稿者の表示名(shared / public)。filter では投稿者の uid と比べ、値は `{"me": true}`
 * - `$createdAt`: レコードを作った日時
 * - `$marks.<id>`: 印の件数(#255)
 */
object RecordValue {
    const val AUTHOR = "\$author"
    const val CREATED_AT = "\$createdAt"
    const val MARK_PREFIX = "\$marks."
    val all: Set<String> = setOf(AUTHOR, CREATED_AT, MARK_PREFIX + "<id>")

    /** `$marks.<id>` の id(それ以外は null)。 */
    fun markId(field: String): String? = if (field.startsWith(MARK_PREFIX)) field.substring(MARK_PREFIX.length) else null

    /** 表示・並べ替え用の仮の Field。entity で使えない値は null。名前は日本語(画面の文言は描画側で引き直す)。 */
    fun field(id: String, entity: Entity): Field? {
        markId(id)?.let { mid ->
            if (!entity.isShared) return null
            val mark = entity.mark(mid) ?: return null
            return Field(id = id, name = mark.label, type = FieldType.INTEGER)
        }
        return when {
            id == AUTHOR && entity.isShared -> Field(id = AUTHOR, name = "投稿者", type = FieldType.STRING)
            id == CREATED_AT -> Field(id = CREATED_AT, name = "投稿日時", type = FieldType.DATETIME)
            else -> null
        }
    }
}

/** `Field.hidden`(#262・クイズの正解)を誰に見せるか。 */
object HiddenFields {
    /** フォームで hidden の入力を出してよいか。 */
    fun canEdit(record: Record?, currentUid: String?): Boolean {
        val author = record?.author
        if (author.isNullOrEmpty()) return true
        if (currentUid.isNullOrEmpty()) return false
        return author == currentUid
    }

    /** 表示用レコードから hidden の値を落とす。 */
    fun redact(records: List<Record>, entity: Entity?): List<Record> {
        val hidden = entity?.hiddenFieldIds
        if (hidden.isNullOrEmpty()) return records
        return records.map { r -> r.copy(data = r.data.filterKeys { it !in hidden }) }
    }

    fun redact(data: Map<String, JSONValue>, entity: Entity?): Map<String, JSONValue> {
        val hidden = entity?.hiddenFieldIds
        if (hidden.isNullOrEmpty()) return data
        return data.filterKeys { it !in hidden }
    }
}
