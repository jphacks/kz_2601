package jp.kz2601.metamon.renderer.form

import androidx.compose.runtime.Composable
import androidx.compose.runtime.State
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import jp.kz2601.metamon.dsl.Screen
import jp.kz2601.metamon.dsl.records.Entity
import jp.kz2601.metamon.dsl.records.Record
import jp.kz2601.metamon.dsl.records.SingleRecord
import jp.kz2601.metamon.renderer.DSLRuntime
import jp.kz2601.metamon.renderer.navigation.Route

/** 画面ごとのフォームの下書きの作り方(iOS の `ScreenView.setUpForm` / `formRecord` / `makeDraft`)。 */
object ScreenForm {
    /** フォームが編集する記録。single の entity(#267)は recordId によらずその 1 件(無ければ新規)。 */
    fun formRecord(entity: Entity, recordId: String?, records: List<Record>): Record? {
        if (entity.single) return SingleRecord.pick(records)
        return recordId?.let { id -> records.firstOrNull { it.id == id } }
    }

    /** 下書きを作る。既存の記録なら編集・権限、新規なら直前に積まれた初期値(FormPrefill)を 1 回だけ受け取る。 */
    fun makeDraft(entity: Entity, record: Record?, runtime: DSLRuntime): FormDraft {
        val draft = FormDraft(entity, record)
        if (record != null) {
            draft.isReadOnly = !runtime.canEdit(entity.id, record)
            draft.canDelete = !entity.single && runtime.canDelete(entity.id, record)
        } else {
            // prefill は開いた時の値として扱う(閉じても破棄の確認を出さない・M10)
            draft.prefill(FormPrefill.consume(entity))
        }
        return draft
    }

    /** 今の下書きを使い続けるか(entity と編集する記録が同じなら入力を残す)。 */
    fun keeps(current: FormDraft?, entity: Entity, record: Record?): Boolean =
        current != null && current.entity.id == entity.id && current.recordId == record?.id
}

/**
 * `screen.entity` を持つ画面の下書き。entity が無い画面は null。
 * 記録が読み込まれて編集する記録が変わった時(recordId 付き・single)だけ作り直し、それ以外は入力を残す。
 */
@Composable
fun rememberScreenForm(screen: Screen, route: Route, runtime: DSLRuntime): FormDraft? {
    val entityId = screen.entity ?: return null
    val entity = remember(entityId, runtime) { runtime.entityModel(entityId) } ?: return null
    val records: State<Map<String, List<Record>>> =
        runtime.recordsFlow?.collectAsState() ?: remember { mutableStateOf(emptyMap()) }
    val record = ScreenForm.formRecord(entity, route.recordId, records.value[entityId].orEmpty())
    val holder = remember(route.id, entityId) { arrayOfNulls<FormDraft>(1) }
    val current = holder[0]
    if (current != null && ScreenForm.keeps(current, entity, record)) return current
    // 保存した後(新規 → 記録あり)は作り直さない。保存で recordId が付くのは AppRuntime の書き込みの後なので、
    // 開いた時に recordId が無かった新規フォームは、記録の一覧が変わっても下書きを残す
    if (current != null && route.recordId == null && !entity.single) return current
    return ScreenForm.makeDraft(entity, record, runtime).also {
        runtime.formDrafts?.restore(route.id, it) // プロセスが落とされる前の書きかけ(#1185)
        holder[0] = it
    }
}
