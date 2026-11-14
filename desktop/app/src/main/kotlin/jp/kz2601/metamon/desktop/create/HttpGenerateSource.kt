package jp.kz2601.metamon.desktop.create

import jp.kz2601.metamon.services.consent.AiConsentRequired
import jp.kz2601.metamon.services.http.ApiJson
import jp.kz2601.metamon.services.http.HttpRequest
import jp.kz2601.metamon.services.http.HttpResponse
import jp.kz2601.metamon.services.http.MetamonHttp
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.currentCoroutineContext
import kotlinx.coroutines.ensureActive
import kotlinx.coroutines.withContext
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.put
import kotlinx.serialization.json.putJsonArray

/**
 * `POST /v1/generate?stream=1` の行をそのまま渡す(#2163)。HttpAIClient と同じ本文・ヘッダー。
 * HttpAIClient は途中で切れると通常の呼び方に戻るので、「途中で切れた」を見分けるために行を自分で読む(#2382 と同じ理由)。
 * - 同意([isGranted])が無ければ送らない
 * - 2xx 以外は共通のエラー封筒を読んで投げる。stream を知らないサーバー(JSON で返った)は result 1 件の行にする
 */
class HttpGenerateSource(
    private val http: MetamonHttp,
    private val isGranted: () -> Boolean = { true },
    private val timeoutMillis: Long = 45_000,
) : GenerateSource {
    override suspend fun stream(request: GenerateRequest, onLine: (String) -> Boolean) {
        if (!isGranted()) throw AiConsentRequired()
        val body = buildJsonObject {
            put("appId", request.appId)
            put("transcript", request.transcript)
            put("mode", "auto")
            put("locale", request.locale ?: http.language())
            putJsonArray("supportedFeatures") {}
        }
        val req = HttpRequest(
            method = "POST",
            path = listOf("v1", "generate"),
            query = listOf("stream" to "1"),
            body = body.toString(),
            headers = mapOf("Accept" to "text/event-stream"),
            timeoutMillis = timeoutMillis,
        )
        http.openStream(req).use { response ->
            withContext(Dispatchers.IO) {
                val status = response.code
                if (status !in 200..299) throw http.errorFrom(HttpResponse(status, response.body.string(), response.headers))
                if (!response.header("Content-Type").orEmpty().contains("text/event-stream")) {
                    // 1 行にそろえる(本文の改行で data が割れないように)
                    val text = response.body.string()
                    val oneLine = runCatching { ApiJson.parseToJsonElement(text).toString() }.getOrDefault(text.replace('\n', ' '))
                    for (line in listOf("event: result", "data: $oneLine", "")) if (onLine(line)) break
                    return@withContext
                }
                val source = response.body.source()
                while (true) {
                    currentCoroutineContext().ensureActive()
                    val line = source.readUtf8Line() ?: break
                    if (onLine(line)) break
                }
            }
        }
    }
}
