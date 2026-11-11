package jp.kz2601.metamon.renderer.conformance

import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.ComponentType
import jp.kz2601.metamon.dsl.ai.chatSaveTo
import jp.kz2601.metamon.renderer.DSLRuntime
import jp.kz2601.metamon.renderer.components.ai.AiChatSend
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive

/**
 * 描画の木のうち aiChat(#874・#1170)の部分。DSL の値(textField・roleField・saveTo の entity・inputEntity)と
 * 会話の並び `turns: [{recordId, role, text}]`(Leaf と同じ `AiChatSend.turns`: 自分の記録だけ・古い順・roleField が無ければ交互)。
 * 返事は AI 次第なので木に入れない。entity は declared が入れる。
 */
object AiChatNodes {
    fun props(c: Component, runtime: DSLRuntime): Map<String, JsonElement> {
        if (c.type != ComponentType.AI_CHAT) return emptyMap()
        return buildMap {
            c.textField?.let { put("textField", JsonPrimitive(it)) }
            c.roleField?.let { put("roleField", JsonPrimitive(it)) }
            c.chatSaveTo?.let { put("saveTo", JsonObject(mapOf("entity" to JsonPrimitive(it.entity)))) }
            c.inputEntity?.let { put("inputEntity", JsonPrimitive(it)) }
            val spec = AiChatSend.Spec.of(c) ?: return@buildMap
            put(
                "turns",
                JsonArray(
                    AiChatSend.turns(runtime, spec).map { t ->
                        JsonObject(
                            mapOf(
                                "recordId" to JsonPrimitive(t.id),
                                "role" to JsonPrimitive(t.role.wire),
                                "text" to JsonPrimitive(t.text),
                            ),
                        )
                    },
                ),
            )
        }
    }
}
