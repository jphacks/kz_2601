package jp.kz2601.metamon.dsl.records

import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.DslJson
import jp.kz2601.metamon.dsl.expr.Expr
import jp.kz2601.metamon.dsl.expr.ExprEvaluator
import jp.kz2601.metamon.dsl.Filter as DslFilter
import jp.kz2601.metamon.dsl.Sort as DslSort

/**
 * list / table / stat / chart … が読む記録(iOS の `DSLRuntime.filteredRecords`)。
 * computed を合成 → filter(`{"me"}` は uid に・式は評価して)→ hidden を落とす。
 */
object RecordQuery {
    /** 部品の `filter`(生成器の型)→ 集計の型。読めなければ空(= 絞り込み無し)。 */
    fun filters(f: DslFilter?): List<Filter> {
        f ?: return emptyList()
        return Filter.oneOrMany(DslJson.encodeToJsonElement(DslFilter.serializer(), f))
    }

    /** 部品の式(生成器の型)→ 評価器の型。読めなければ null。 */
    fun expr(e: jp.kz2601.metamon.dsl.Expr?): Expr? {
        e ?: return null
        return runCatching { Expr.from(DslJson.encodeToJsonElement(jp.kz2601.metamon.dsl.ExprSerializer, e)) }.getOrNull()
    }

    /** 部品の `sort`(生成器の型)→ 集計の型。 */
    fun sort(s: DslSort?): Sort? = s?.let { Sort(it.field, it.direction.wire) }

    /** 部品が見る記録。`followed` は `{"myFollows": true}` を解く uid(読めなければ null = 一致しない)。 */
    fun filtered(
        c: Component,
        entity: Entity?,
        evaluator: ExprEvaluator,
        uid: String?,
        followed: Set<String>? = null,
    ): List<Record> {
        val rows = evaluator.viewRecords(c.entity ?: "")
        val fs = filters(c.filter)
        val out = if (fs.isEmpty()) rows else evaluator.apply(fs.map { it.resolvingFollows(followed).resolvingMe(uid) }, entity, rows)
        return HiddenFields.redact(out, entity)
    }
}
