package jp.kz2601.metamon.dsl

import kotlinx.serialization.Serializable

// 型そのもの(AppDSL・Screen・Component など)は generated/ にある(backend/cmd/dslgen がスキーマから作る)。
// ここは手で書く入口だけ: 封筒・読み込み・最後の手段。

/** appconfig の `DSLEnvelope`(`GET /apps/{appId}/dsl`)。 */
@Serializable
data class AppDocument(
    val appId: String = "local",
    /** 単調増加。進んだ時だけ再描画する。 */
    val version: Int = 0,
    val dsl: AppDSL,
) {
    companion object {
        fun decode(json: String): AppDocument = DslJson.decodeFromString(serializer(), json)
    }
}

/** DSL 本文を読む。知らない type・キー・値では落とさない(壊れた部品だけ捨てる)。 */
fun AppDSL.Companion.decode(json: String): AppDSL = DslJson.decodeFromString(AppDSL.serializer(), json)

/** 読み込みに失敗した時の最後の手段。 */
val AppDSL.Companion.fallback: AppDSL get() = FallbackDSL

private val FallbackDSL: AppDSL by lazy {
    AppDSL.decode(
        """
        {"schemaVersion":1,"app":{"name":"Tailor It","voiceEntry":"none"},"initialScreen":"home",
         "screens":[{"id":"home","title":"Tailor It","components":[{"type":"text","text":"DSL を読み込めませんでした"}]}]}
        """.trimIndent(),
    )
}

fun AppDSL.screen(id: String): Screen? = screens.firstOrNull { it.id == id }

/** 初期状態(音声画面だけ)か。 */
val AppDSL.isVoiceOnly: Boolean get() = app.voiceEntry == VoiceEntry.FULLSCREEN
