package jp.kz2601.metamon.renderer.form

import jp.kz2601.metamon.dsl.expr.DSLClock
import jp.kz2601.metamon.dsl.expr.ExprEvaluator
import jp.kz2601.metamon.dsl.records.Field
import jp.kz2601.metamon.dsl.records.FieldCoding
import jp.kz2601.metamon.dsl.records.JSONValue
import jp.kz2601.metamon.dsl.records.Record
import jp.kz2601.metamon.renderer.DSLRuntime
import jp.kz2601.metamon.renderer.FormMember

/** 参照先の記録 1 件(保存値は [id])。 */
data class RefOption(val id: String, val title: String)

/** 型で描き替える入力が読む選択肢と表示(iOS の FormFieldOverrides.swift・`refTitle` / `memberName`)。 */
object FormChoices {
    fun records(runtime: DSLRuntime, entity: String): List<Record> = runtime.recordsFlow?.value?.get(entity).orEmpty()

    /** ref / refs の選択肢: 参照先の記録を、その見出し(titleField / 最初の見える文字の field)で。 */
    fun refOptions(field: Field, runtime: DSLRuntime): List<RefOption> {
        val ref = field.refEntity ?: return emptyList()
        return records(runtime, ref).map { RefOption(it.id, refTitle(it, ref, runtime)) }
    }

    fun refTitle(record: Record, refEntity: String, runtime: DSLRuntime): String {
        val target = runtime.entityModel(refEntity) ?: return record.id
        val tf = target.displayTitleField ?: return record.id
        return FieldCoding.display(record.data[tf] ?: JSONValue.Null, target.field(tf), now = DSLClock.now())
    }

    /** 参照先の entity の名前。見つからなければ [fallback](「参照先」)。 */
    fun refEntityName(field: Field, runtime: DSLRuntime, fallback: String): String =
        field.refEntity?.let { runtime.entityModel(it)?.name } ?: fallback

    /** member の選択肢(保存値 uid, 表示名)。先頭は未選択([placeholder])。今の値が一覧に無ければ(抜けた人)後ろに残す。 */
    fun memberOptions(current: String, members: List<FormMember>, placeholder: String): List<Pair<String, String>> {
        val listed = members.map { it.uid to it.displayName.ifEmpty { it.uid } }
        val kept = if (current.isEmpty() || members.any { it.uid == current }) emptyList() else listOf(current to current)
        return listOf("" to placeholder) + listed + kept
    }

    /** computed の値: 行 = 入力中の下書き(金額を打つと「1人あたり」が追従)。計算できなければ null。 */
    fun computedValue(field: Field, form: FormDraft, runtime: DSLRuntime): JSONValue? {
        val expr = field.computed ?: return null
        val row = ExprEvaluator.Row(form.entity, Record(form.recordId ?: "draft", form.payload), memoizable = false)
        val evaluator = ExprEvaluator(runtime.entityModels, { records(runtime, it) })
        return evaluator.evaluate(expr, row)?.json(field)
    }

    fun computedText(field: Field, form: FormDraft, runtime: DSLRuntime): String =
        FieldCoding.display(computedValue(field, form, runtime) ?: JSONValue.Null, field, now = DSLClock.now())
}
