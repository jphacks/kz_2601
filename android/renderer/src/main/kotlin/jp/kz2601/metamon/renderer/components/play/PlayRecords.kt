package jp.kz2601.metamon.renderer.components.play

import androidx.compose.runtime.Composable
import androidx.compose.runtime.State
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.expr.DSLClock
import jp.kz2601.metamon.dsl.expr.ExprEvaluator
import jp.kz2601.metamon.dsl.records.Aggregator
import jp.kz2601.metamon.dsl.records.Field
import jp.kz2601.metamon.dsl.records.FieldCoding
import jp.kz2601.metamon.dsl.records.FieldType
import jp.kz2601.metamon.dsl.records.Filter
import jp.kz2601.metamon.dsl.records.HiddenFields
import jp.kz2601.metamon.dsl.records.JSONValue
import jp.kz2601.metamon.dsl.records.Record
import jp.kz2601.metamon.renderer.DSLRuntime
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive

/**
 * 計算・遊び系の部品が読む記録(iOS の `ComputedValues.swift` の viewRecords / filteredRecords / display)。
 * Compose に依らない関数にして JVM テストから呼ぶ。[all] は `runtime.recordsFlow` の今の値。
 */
object PlayRecords {
    /** 保存した記録そのもの(書き込み・トーナメントの固定の判定に使う)。 */
    fun records(all: Map<String, List<Record>>, entity: String): List<Record> = all[entity].orEmpty()

    /** computed を合成し hidden を落とした表示用の記録。 */
    fun viewRecords(runtime: DSLRuntime, all: Map<String, List<Record>>, entity: String): List<Record> =
        HiddenFields.redact(unredacted(runtime, all, entity), runtime.entityModel(entity))

    /** 部品の entity + filter を当てた表示用の記録(filter は hidden を落とす前に当てる)。 */
    fun filtered(runtime: DSLRuntime, all: Map<String, List<Record>>, c: Component, filters: List<Filter> = filters(c)): List<Record> {
        val entityId = c.entity.orEmpty()
        val entity = runtime.entityModel(entityId)
        val rows = unredacted(runtime, all, entityId)
        val out = if (filters.isEmpty()) {
            rows
        } else {
            val resolved = filters.map { it.resolvingMe(runtime.currentUid()) }
            if (resolved.all { it.valueExpr == null }) {
                rows.filter { r -> resolved.all { Aggregator.matches(it, r, entity?.field(it.field)) } }
            } else {
                evaluator(runtime, all).apply(resolved, entity, rows)
            }
        }
        return HiddenFields.redact(out, entity)
    }

    /** 部品の filter(:dsl の Component は 1 件だけ持つ)を記録の型で。読めなければ空(= 絞り込み無し)。 */
    fun filters(c: Component): List<Filter> {
        val f = c.filter ?: return emptyList()
        val json = JsonObject(
            buildMap {
                put("field", JsonPrimitive(f.field))
                put("op", JsonPrimitive(f.op.wire))
                f.value?.let { put("value", it) }
            },
        )
        return runCatching { listOf(Filter.from(json)) }.getOrDefault(emptyList())
    }

    /** 表示用の文字列。ref は参照先の見出し、member は表示名。 */
    fun display(runtime: DSLRuntime, all: Map<String, List<Record>>, value: JSONValue, field: Field?): String {
        when (field?.type) {
            FieldType.REF -> {
                val id = value.stringValue?.takeIf { it.isNotEmpty() } ?: return ""
                return refTitle(runtime, all, id, field.refEntity)
                    ?: if (field.options?.contains(id) == true) id else FieldCoding.UNAVAILABLE
            }
            FieldType.REFS -> return Aggregator.refIds(value).mapNotNull { refTitle(runtime, all, it, field.refEntity) }.joinToString("、")
            FieldType.MEMBER -> return memberName(runtime, value.stringValue)
            FieldType.MEMBERS -> return Aggregator.refIds(value).map { memberName(runtime, it) }.joinToString("、")
            else -> return FieldCoding.display(value, field, now = DSLClock.now())
        }
    }

    private fun memberName(runtime: DSLRuntime, uid: String?): String {
        if (uid.isNullOrEmpty()) return ""
        return runtime.formMembers.firstOrNull { it.uid == uid }?.displayName?.ifEmpty { null } ?: uid
    }

    private fun refTitle(runtime: DSLRuntime, all: Map<String, List<Record>>, id: String, refEntity: String?): String? {
        val target = refEntity?.let { runtime.entityModel(it) } ?: return null
        val record = all[target.id].orEmpty().firstOrNull { it.id == id } ?: return null
        val tf = target.displayTitleField ?: return record.id
        return FieldCoding.display(record[tf], target.field(tf), now = DSLClock.now())
    }

    private fun unredacted(runtime: DSLRuntime, all: Map<String, List<Record>>, entity: String): List<Record> {
        if (runtime.entityModel(entity)?.computedFields.isNullOrEmpty()) return records(all, entity)
        return evaluator(runtime, all).viewRecords(entity)
    }

    private fun evaluator(runtime: DSLRuntime, all: Map<String, List<Record>>) =
        ExprEvaluator(runtime.entityModels, { all[it].orEmpty() })
}

/** 表示中の記録(変わると描き直す)。runtime が記録を持たなければ空。 */
@Composable
internal fun rememberRecords(runtime: DSLRuntime): State<Map<String, List<Record>>> =
    runtime.recordsFlow?.collectAsState() ?: remember { mutableStateOf(emptyMap()) }
