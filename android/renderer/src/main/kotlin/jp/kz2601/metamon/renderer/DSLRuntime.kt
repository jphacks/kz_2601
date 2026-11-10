package jp.kz2601.metamon.renderer

/** 描画器が使う能力。:app の AppRuntime が実装する(iOS の `DSLRuntime`)。:renderer は :services を知らない。 */
interface DSLRuntime {
    /** voiceInput が押された。音声認識は #359 で入れる。 */
    fun onVoiceInput(componentId: String?)
}

/** 何もしない実装(プレビュー・テスト用)。 */
object NoopDSLRuntime : DSLRuntime {
    override fun onVoiceInput(componentId: String?) = Unit
}
