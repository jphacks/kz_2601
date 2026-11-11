package jp.kz2601.metamon.renderer

import jp.kz2601.metamon.dsl.Action
import jp.kz2601.metamon.renderer.navigation.Route

/** 描画器が使う能力。:app の AppRuntime が実装する(iOS の `DSLRuntime`)。:renderer は :services を知らない。 */
interface DSLRuntime {
    /** voiceInput が押された。音声認識は #359 で入れる。 */
    fun onVoiceInput(componentId: String?)

    /** 遷移(navigate / back)以外のアクション。保存・削除・URL などは子 Issue で入れる。既定は何もしない。 */
    fun onAction(action: Action, route: Route) = Unit
}

/** 何もしない実装(プレビュー・テスト用)。 */
object NoopDSLRuntime : DSLRuntime {
    override fun onVoiceInput(componentId: String?) = Unit
}
