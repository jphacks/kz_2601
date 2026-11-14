package jp.kz2601.metamon.dsl.records

import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.ComponentType
import jp.kz2601.metamon.dsl.DslJson
import jp.kz2601.metamon.dsl.Resets

/**
 * `list.item.resets: "daily"`(#132)の toggle field(entity id → field id)。iOS の `AppDSL.dailyToggleFields`。
 */
val AppDSL.dailyToggleFields: Map<String, Set<String>>
    get() {
        val out = mutableMapOf<String, MutableSet<String>>()
        fun walk(c: Component) {
            if (c.type == ComponentType.LIST && c.item?.resets == Resets.DAILY) {
                val e = c.entity
                val t = c.item?.toggle
                if (e != null && t != null) out.getOrPut(e) { mutableSetOf() } += t
            }
            c.children.forEach(::walk)
        }
        screens.forEach { s -> s.components.forEach(::walk) }
        return out
    }

/**
 * DSL の entities を記録の型で読む。読めない entity は落とす。
 * 毎日リセットの toggle の date field は新規作成で今日を入れない(入れると追加した瞬間に「済み」になる・iOS の `clearDailyToggleDefaults`)。
 */
fun AppDSL.entityModels(): List<Entity> {
    val daily = dailyToggleFields
    return entities.mapNotNull { e ->
        val model = runCatching { Entity.from(DslJson.encodeToJsonElement(jp.kz2601.metamon.dsl.Entity.serializer(), e)) }.getOrNull()
            ?: return@mapNotNull null
        val ids = daily[model.id] ?: return@mapNotNull model
        model.copy(fields = model.fields.map { f -> if (f.id in ids && f.type.isDate) f.copy(defaultValue = JSONValue.Null) else f })
    }
}
