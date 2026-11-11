package jp.kz2601.metamon.services.live

import jp.kz2601.metamon.services.GenerateProgress
import jp.kz2601.metamon.services.StreamedComponent
import jp.kz2601.metamon.services.http.ApiError
import jp.kz2601.metamon.services.http.ApiJson
import jp.kz2601.metamon.services.http.MetamonHttp
import jp.kz2601.metamon.services.http.SseFrame
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.contentOrNull
import kotlinx.serialization.json.intOrNull
import kotlinx.serialization.json.jsonObject

/**
 * 生成の SSE の 1 件を型にしたもの(iOS の `GenerateStreamEvent`)。
 * 契約: backend/internal/ai/handler.go(`serveSSE`)・generate.go(`Event`)・component_stream.go(`ComponentEvent`)。
 * `app` / `summary` / `component` / `restart` は仮置き、`result` が通常の応答と同じ本文、`error` が通常のエラー封筒。
 */
sealed interface GenerateStreamEvent {
    data class Progress(val progress: GenerateProgress) : GenerateStreamEvent

    /** 通常の応答と同じ本文(`GenerateResponse`) */
    data class Result(val json: String) : GenerateStreamEvent

    data class Failure(val error: ApiError) : GenerateStreamEvent

    companion object {
        /** 知らない event・壊れた本文は null(読み飛ばす)。 */
        fun decode(frame: SseFrame): GenerateStreamEvent? {
            val obj = { runCatching { ApiJson.parseToJsonElement(frame.data).jsonObject }.getOrNull() }
            return when (frame.event) {
                "app" -> {
                    val o = obj() ?: return null
                    val name = o.str("name")?.trim().orEmpty()
                    if (name.isEmpty()) return null
                    val tabs = (o["tabs"] as? JsonArray).orEmpty().mapNotNull { (it as? JsonPrimitive)?.contentOrNull }
                    Progress(GenerateProgress.App(name, tabs, o.int("attempt") ?: 0))
                }
                "summary" -> {
                    val o = obj() ?: return null
                    val text = o.str("summary")?.trim().orEmpty()
                    if (text.isEmpty()) return null
                    Progress(GenerateProgress.Summary(text, o.int("attempt") ?: 0))
                }
                "component" -> {
                    val o = obj() ?: return null
                    val c = o["component"] as? JsonObject ?: return null
                    val type = c.str("type")?.trim().orEmpty()
                    val index = c.int("index")
                    if (type.isEmpty() || index == null) return null
                    Progress(
                        GenerateProgress.Component(
                            StreamedComponent(
                                attempt = o.int("attempt") ?: 0,
                                screen = o.str("screen") ?: "",
                                index = index,
                                id = c.str("id"),
                                type = type,
                                area = c.str("area"),
                                label = c.str("label"),
                            ),
                        ),
                    )
                }
                "restart" -> Progress(GenerateProgress.Restart(obj()?.int("attempt") ?: 0))
                "result" -> Result(frame.data)
                "error" -> {
                    val inner = MetamonHttp.envelope(frame.data)
                        ?: return Failure(ApiError.Http(500, frame.data))
                    val status = MetamonHttp.statusOf(inner) ?: 500
                    Failure(MetamonHttp.errorFrom(status, frame.data, null))
                }
                else -> null
            }
        }

        private fun JsonObject.str(key: String): String? = (this[key] as? JsonPrimitive)?.takeIf { it.isString }?.content
        private fun JsonObject.int(key: String): Int? = (this[key] as? JsonPrimitive)?.intOrNull
    }
}
