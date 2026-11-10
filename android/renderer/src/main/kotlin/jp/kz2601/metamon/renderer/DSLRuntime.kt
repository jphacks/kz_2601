package jp.kz2601.metamon.renderer

import jp.kz2601.metamon.dsl.Action
import jp.kz2601.metamon.dsl.records.Entity
import jp.kz2601.metamon.dsl.records.JSONValue
import jp.kz2601.metamon.dsl.records.Record
import jp.kz2601.metamon.renderer.components.ai.AiHost
import jp.kz2601.metamon.renderer.form.FormDraft
import jp.kz2601.metamon.renderer.navigation.NavigationState
import jp.kz2601.metamon.renderer.navigation.Route
import kotlinx.coroutines.flow.StateFlow

/** アクションを押した場所(iOS の `ActionContext`)。 */
data class ActionContext(
    val route: Route,
    /** その画面のフォームの下書き(entity を持つ画面だけ) */
    val form: FormDraft? = null,
    /** 一覧の行タップで渡す recordId */
    val recordId: String? = null,
) {
    /** 今の記録: 行タップならその行、フォーム上ならいま編集中の記録(新規・未保存なら null)。 */
    val currentRecordId: String? get() = recordId ?: form?.recordId
}

/** 画面遷移の状態を持つ側(:app の AppRuntime)。描画器はこれを読み、タブ・戻る・シートを閉じるで書き戻す。 */
interface NavigationHost {
    val navigation: StateFlow<NavigationState>

    fun setNavigation(next: NavigationState)
}

/** 描画器が使う能力。:app の AppRuntime が実装する(iOS の `DSLRuntime`)。:renderer は :services を知らない。 */
interface DSLRuntime {
    /** voiceInput が押された。音声認識は #359 で入れる。 */
    fun onVoiceInput(componentId: String?)

    /** アクション。[navigationHost] があれば navigate / back もここへ来る。既定は何もしない。 */
    fun onAction(action: Action, context: ActionContext) = Unit

    /** 遷移をランタイムが持つならその口。null なら描画器が自前で持つ(プレビュー・テスト)。 */
    val navigationHost: NavigationHost? get() = null

    // フォーム(画面ごとの下書きと入力部品が読む)

    /** DSL の entity を記録の型で読んだもの(式の評価・参照先の見出し)。 */
    val entityModels: List<Entity> get() = emptyList()

    /** null の entity にはフォームを作らない。 */
    fun entityModel(id: String): Entity? = entityModels.firstOrNull { it.id == id }

    /** 表示中の記録(entity id → 記録)。変わると描き直す。null なら記録なし(新規のフォームだけ)。 */
    val recordsFlow: StateFlow<Map<String, List<Record>>>? get() = null

    fun canEdit(entity: String, record: Record): Boolean = true

    fun canDelete(entity: String, record: Record): Boolean = true

    /** 一覧のワンタップ更新(list.item.toggle)。保存できたら true。既定は保存しない。 */
    suspend fun updateRecord(entity: String, recordId: String, data: Map<String, JSONValue>): Boolean = false

    /** サインイン中の uid(hidden の項目・members から自分を外す)。 */
    fun currentUid(): String? = null

    /** 共有データの参加者(member / members の選択肢・iOS の `membersHost.appMembers`)。読めなければ空。 */
    val formMembers: List<FormMember> get() = emptyList()

    // AI ボタン(#873)

    /** aiButton のオンライン版・端末内 AI・集計。null なら端末内 AI も無い(プレビュー・テスト)。 */
    val ai: AiHost? get() = null

    /** entity の記録を読んでおく([recordsFlow] に入る)。既定は何もしない。 */
    suspend fun loadRecords(entities: Set<String>) = Unit

    /** フォームを通さず 1 件足す(aiButton saveTo)。足せたら true。 */
    suspend fun addRecord(entity: String, data: Map<String, JSONValue>): Boolean = false
}

/** member / members の選択肢 1 人。保存値は [uid]。 */
data class FormMember(val uid: String, val displayName: String)

/** 何もしない実装(プレビュー・テスト用)。 */
object NoopDSLRuntime : DSLRuntime {
    override fun onVoiceInput(componentId: String?) = Unit
}
