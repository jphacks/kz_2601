package jp.kz2601.metamon.renderer

import jp.kz2601.metamon.dsl.Action
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
}

/** 何もしない実装(プレビュー・テスト用)。 */
object NoopDSLRuntime : DSLRuntime {
    override fun onVoiceInput(componentId: String?) = Unit
}
