package jp.kz2601.metamon.services.sync

import jp.kz2601.metamon.services.http.ApiJson
import jp.kz2601.metamon.services.http.HttpRequest
import jp.kz2601.metamon.services.http.MetamonHttp
import jp.kz2601.metamon.services.http.SseFrame
import jp.kz2601.metamon.services.http.SseParser
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.awaitCancellation
import kotlinx.coroutines.coroutineScope
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.contentOrNull
import kotlinx.serialization.json.intOrNull
import okio.BufferedSource
import java.io.IOException

/**
 * `GET /v1/apps/{appId}/events` の 1 件(docs/backend/openapi-appconfig.yaml)。届くのは「何が変わったか」だけ。
 */
sealed interface ChangeEvent {
    /** つながった。今の版と、通知の対象の entity */
    data class Hello(val version: Int?, val entities: List<String>) : ChangeEvent

    /** DSL の版が進んだ(直後に bye が来て張り直す) */
    data class Dsl(val version: Int?) : ChangeEvent

    data class Records(val entity: String) : ChangeEvent

    /** サーバーの予定終了(既定 50 秒)。すぐ張り直してよい */
    data object Bye : ChangeEvent

    companion object {
        /** 知らない event・読めない data は null(読み飛ばす)。 */
        fun decode(frame: SseFrame): ChangeEvent? {
            val obj = runCatching { ApiJson.parseToJsonElement(frame.data) as? JsonObject }.getOrNull() ?: JsonObject(emptyMap())
            val version = (obj["version"] as? JsonPrimitive)?.intOrNull
            return when (frame.event) {
                "hello" -> Hello(
                    version,
                    (obj["entities"] as? JsonArray).orEmpty().mapNotNull { (it as? JsonPrimitive)?.contentOrNull },
                )
                "dsl" -> Dsl(version)
                "records" -> (obj["entity"] as? JsonPrimitive)?.contentOrNull?.let(::Records)
                "bye" -> Bye
                else -> null
            }
        }
    }
}

/** 変化通知の購読 1 本。テストで差し替えるために SSE の読み込みから切り出す。 */
fun interface ChangeEventSource {
    /**
     * 1 本の購読を読み切る。`bye` で終わったら true(予定終了)、それ以外で終わったら false。
     * 401 は [StreamUnauthorized]、それ以外の失敗は例外のまま投げる。
     * @param forceRefresh 401 の後の張り直し(トークンを取り直す)
     */
    suspend fun stream(appId: String, forceRefresh: Boolean, onEvent: suspend (ChangeEvent) -> Unit): Boolean
}

class StreamUnauthorized : IOException("events: 401")

class StreamStatus(val status: Int) : IOException("events: status $status")

/**
 * SSE の実装(#994 の [MetamonHttp.openStream] + [SseParser])。
 * 心拍は 15 秒ごとなので、[idleTimeoutMillis] より長く無音なら切れたとみなす(読み込みの時間切れ)。
 */
class SseChangeEventSource(
    private val http: MetamonHttp,
    private val idleTimeoutMillis: Long = 40_000,
) : ChangeEventSource {

    override suspend fun stream(appId: String, forceRefresh: Boolean, onEvent: suspend (ChangeEvent) -> Unit): Boolean {
        val req = HttpRequest(
            method = "GET",
            path = listOf("v1", "apps", appId, "events"),
            headers = mapOf("Accept" to "text/event-stream", "Cache-Control" to "no-cache"),
            timeoutMillis = idleTimeoutMillis,
        )
        return http.openStream(req, forceRefresh).use { response ->
            if (response.code == 401) throw StreamUnauthorized()
            if (response.code != 200) throw StreamStatus(response.code)
            coroutineScope {
                // 読み込みは cancel で止まらないので、cancel(背面へ・画面を閉じた)されたら応答を閉じて終わらせる
                val closer = launch {
                    try {
                        awaitCancellation()
                    } finally {
                        response.close()
                    }
                }
                try {
                    withContext(Dispatchers.IO) { read(response.body.source(), onEvent) }
                } finally {
                    closer.cancel()
                }
            }
        }
    }

    private suspend fun read(source: BufferedSource, onEvent: suspend (ChangeEvent) -> Unit): Boolean {
        val parser = SseParser()
        while (true) {
            val line = source.readUtf8Line() ?: return false
            val frame = parser.feedLine(line) ?: continue
            val event = ChangeEvent.decode(frame) ?: continue
            if (event == ChangeEvent.Bye) return true
            onEvent(event)
        }
    }
}
