package jp.kz2601.metamon.dsl.ai

import jp.kz2601.metamon.dsl.AIChatSaveTo
import jp.kz2601.metamon.dsl.expr.DSLClock
import jp.kz2601.metamon.dsl.logic.RecordImport
import jp.kz2601.metamon.dsl.records.Entity
import jp.kz2601.metamon.dsl.records.Field
import jp.kz2601.metamon.dsl.records.FieldCoding
import jp.kz2601.metamon.dsl.records.FieldType
import jp.kz2601.metamon.dsl.records.JSONValue
import java.time.Instant

/**
 * `aiChat.saveTo`: 話しかけて記録する(「今日カレー食べた」→ 食事の記録の案)。iOS の `AIChatSaveTo`。検証は backend `dsl/ai_chat_save_to.go`。
 * 依頼に field の形を付けると、サーバーが返事と一緒に `suggest`(field id → 値)を返す。保存は利用者が「追加」を押した時だけ。
 */
object AiChatSaveToRules {
    /** backend `dsl.MaxAIChatSaveFields` / `AIChatSaveFieldTypes`。 */
    const val MAX_FIELDS = 8
    val fieldTypes: Set<FieldType> = setOf(FieldType.STRING, FieldType.TEXT, FieldType.NUMBER, FieldType.DATE, FieldType.ENUM)
    private const val MAX_NUMBER = 1e9

    /** 案に使う field(DSL の順)。entity が違う・使えない field が 1 つでもあれば null。 */
    fun saveFields(saveTo: AIChatSaveTo, entity: Entity): List<Field>? {
        if (entity.id != saveTo.entity || saveTo.fields.isEmpty() || saveTo.fields.size > MAX_FIELDS) return null
        return saveTo.fields.map { id ->
            val f = entity.storedFields.firstOrNull { it.id == id } ?: return null
            if (f.type !in fieldTypes) return null
            f
        }
    }

    /** 依頼に付ける saveTo(backend `chatSaveToOf` と同じ値)。 */
    fun request(saveTo: AIChatSaveTo, entity: Entity, today: Instant = DSLClock.now()): AiTextSaveTo? {
        val fs = saveFields(saveTo, entity) ?: return null
        return AiTextSaveTo(
            entity = entity.id,
            name = entity.name.trim().ifEmpty { null },
            today = FieldCoding.formatDay(today),
            fields = fs.map { f ->
                AiTextSaveField(
                    id = f.id,
                    name = f.name.trim(),
                    type = f.type.wire,
                    options = if (f.type == FieldType.ENUM) f.options else null,
                    unit = f.unit?.trim()?.ifEmpty { null },
                    required = if (f.required == true && f.defaultValue == null) true else null,
                )
            },
        )
    }

    /** 案の値のうち field に合うもの(サーバーと同じ規則で確かめ直す)。field の順。 */
    fun values(saveTo: AIChatSaveTo, suggest: Map<String, JSONValue>, entity: Entity): List<Pair<Field, JSONValue>> {
        val fs = saveFields(saveTo, entity) ?: return emptyList()
        return fs.mapNotNull { f -> suggest[f.id]?.let { check(it, f) }?.let { f to it } }
    }

    /** 「追加」で保存する data: 案の値 + ほかの field は新しいフォームと同じ既定値。何も合わない・required が空なら null。 */
    fun record(saveTo: AIChatSaveTo, suggest: Map<String, JSONValue>, entity: Entity, now: Instant = DSLClock.now()): Map<String, JSONValue>? {
        val got = values(saveTo, suggest, entity)
        if (got.isEmpty()) return null
        val data = got.associate { it.first.id to it.second }.toMutableMap()
        for (x in entity.storedFields) {
            if (data.containsKey(x.id)) continue
            val d = FieldCoding.defaultValue(x, now)
            if (!d.isNull) data[x.id] = d
        }
        for (x in entity.storedFields) {
            if (x.required != true || x.isComputed) continue
            val v = data[x.id]
            if (v == null || v.isNull || v.stringValue?.isEmpty() == true) return null
        }
        return data
    }

    fun check(v: JSONValue, f: Field): JSONValue? = when (f.type) {
        FieldType.NUMBER -> v.doubleValue?.takeIf { it.isFinite() && kotlin.math.abs(it) <= MAX_NUMBER }?.let { JSONValue.Num(it) }
        FieldType.ENUM -> v.stringValue?.takeIf { it in f.options.orEmpty() }?.let { JSONValue.Str(it) }
        FieldType.DATE -> v.stringValue?.takeIf { FieldCoding.strictDay(it) != null }?.let { JSONValue.Str(it) }
        FieldType.STRING, FieldType.TEXT -> {
            val s = (v.stringValue ?: "").trim()
            if (s.isEmpty()) null else JSONValue.Str(s.takeCodePoints(RecordImport.characterLimit(f)))
        }
        else -> null
    }

    private fun String.takeCodePoints(n: Int): String =
        if (codePointCount(0, length) <= n) this else substring(0, offsetByCodePoints(0, n))
}
