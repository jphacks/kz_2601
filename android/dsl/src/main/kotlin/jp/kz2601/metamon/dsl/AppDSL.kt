package jp.kz2601.metamon.dsl

import kotlinx.serialization.Serializable
import kotlinx.serialization.json.JsonObject

// DSL v1(docs/dsl/dsl.schema.json)の Kotlin 表現。iOS の MetamonDSL/Model/AppDocument.swift に合わせる。
// 骨格(#356)の仮の型。#351 の生成器の出力に置き換える。

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

/** DSL 本文(schema の root)。 */
@Serializable
data class AppDSL(
    val schemaVersion: Int,
    val app: AppMeta,
    val initialScreen: String,
    /** 骨格では中身を解釈しない(#357 で型を付ける)。 */
    @Serializable(with = LossyListSerializer::class)
    val entities: List<JsonObject> = emptyList(),
    @Serializable(with = LossyListSerializer::class)
    val screens: List<Screen> = emptyList(),
) {
    fun screen(id: String): Screen? = screens.firstOrNull { it.id == id }

    /** 初期状態(音声画面だけ)か。 */
    val isVoiceOnly: Boolean get() = app.voiceEntry == VoiceEntry.FULLSCREEN

    companion object {
        fun decode(json: String): AppDSL = DslJson.decodeFromString(serializer(), json)

        /** 読み込みに失敗した時の最後の手段。 */
        val fallback: AppDSL = decode(
            """
            {"schemaVersion":1,"app":{"name":"Tailor It","voiceEntry":"none"},"initialScreen":"home",
             "screens":[{"id":"home","title":"Tailor It","components":[{"type":"text","text":"DSL を読み込めませんでした"}]}]}
            """.trimIndent(),
        )
    }
}

@Serializable(with = VoiceEntrySerializer::class)
enum class VoiceEntry(val wire: String) {
    FULLSCREEN("fullscreen"),
    FLOATING("floating"),
    NONE("none"),
}

internal object VoiceEntrySerializer : LenientEnumSerializer<VoiceEntry>(
    "jp.kz2601.metamon.dsl.VoiceEntry",
    VoiceEntry.entries,
    VoiceEntry::wire,
    VoiceEntry.FLOATING,
)

@Serializable
data class AppMeta(
    val name: String = "Tailor It",
    val description: String? = null,
    val icon: String? = null,
    val voiceEntry: VoiceEntry = VoiceEntry.FLOATING,
    /** 2〜5 個ならタブバー表示。 */
    @Serializable(with = LossyListSerializer::class)
    val tabs: List<Tab> = emptyList(),
)

@Serializable
data class Tab(
    val screen: String,
    val title: String,
    val icon: String? = null,
)

@Serializable
data class Screen(
    val id: String,
    val title: String = "",
    @Serializable(with = LossyListSerializer::class)
    val components: List<Component> = emptyList(),
)
