package jp.kz2601.metamon.services.live

import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.AppDocument
import jp.kz2601.metamon.dsl.DslJson
import jp.kz2601.metamon.dsl.ai.AiTextRequest
import jp.kz2601.metamon.dsl.ai.AiTextResult
import jp.kz2601.metamon.services.AIClient
import jp.kz2601.metamon.services.ChatCapability
import jp.kz2601.metamon.services.ChatMessage
import jp.kz2601.metamon.services.ChatMode
import jp.kz2601.metamon.services.ChatTurn
import jp.kz2601.metamon.services.ClientCapabilities
import jp.kz2601.metamon.services.GenerateMode
import jp.kz2601.metamon.services.GenerateProgress
import jp.kz2601.metamon.services.MorphResult
import jp.kz2601.metamon.services.http.ApiError
import jp.kz2601.metamon.services.http.ApiJson
import jp.kz2601.metamon.services.http.HttpRequest
import jp.kz2601.metamon.services.http.HttpResponse
import jp.kz2601.metamon.services.http.MetamonHttp
import jp.kz2601.metamon.services.http.SseParser
import jp.kz2601.metamon.services.local.DocumentCache
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.currentCoroutineContext
import kotlinx.coroutines.ensureActive
import kotlinx.coroutines.withContext
import kotlinx.serialization.Serializable
import kotlinx.serialization.SerializationException
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.contentOrNull
import kotlinx.serialization.json.intOrNull
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.put

/**
 * ai サービスの実装(iOS の `HTTPAIClient` + `GenerateStream`)。
 * - generate: body `{appId, baseVersion?, transcript, mode, locale?, allowedComponents?, disabledFeatures?, supportedFeatures}`
 * - `?stream=1` の SSE で途中経過を読む。使えない時は仮置きを捨てて(`restart` attempt 0)通常の呼び方に戻る
 * - 409(サーバー側の版とずれた)は現行基準で 1 回だけやり直す
 * - `locale` を渡さない時は選んだ UI の言語(`Accept-Language` と同じ値・#843)を送る。iOS は毎回 UI の言語を渡す
 * - 結果は [cache] にも入れる(appconfig から取り直さなくてもオフラインで起動できる・#1020)
 */
class HttpAIClient(
    private val http: MetamonHttp,
    private val capabilities: ClientCapabilities = ClientCapabilities(),
    private val timeoutMillis: Long = 45_000,
    private val log: (String) -> Unit = {},
    private val cache: DocumentCache? = null,
) : AIClient {

    @Serializable
    internal data class GenerateBody(
        val appId: String,
        val baseVersion: Int? = null,
        val transcript: String,
        val mode: String,
        val locale: String? = null,
        val allowedComponents: List<String>? = null,
        val disabledFeatures: List<String>? = null,
        val supportedFeatures: List<String> = emptyList(),
    )

    @Serializable
    internal data class ChatBody(
        val appId: String,
        val messages: List<ChatMessage>,
        val locale: String? = null,
        val mode: ChatMode? = null,
        val capabilities: List<String> = ChatCapability.all,
        val allowedComponents: List<String>? = null,
        val disabledFeatures: List<String>? = null,
        val supportedFeatures: List<String> = emptyList(),
    )

    /** stream を諦めて通常の呼び方に戻す理由(外には出さない)。 */
    internal class StreamUnavailable(reason: String) : Exception(reason)

    override suspend fun morph(
        transcript: String,
        appId: String,
        currentVersion: Int,
        locale: String?,
        mode: GenerateMode,
    ): MorphResult {
        val body = body(appId, base(currentVersion), transcript, locale, mode)
        return try {
            generate(body)
        } catch (e: ApiError) {
            if (e.status != 409) throw e
            generate(body.copy(baseVersion = null))
        }.also { remember(it) }
    }

    override suspend fun morphStreaming(
        transcript: String,
        appId: String,
        currentVersion: Int,
        locale: String?,
        mode: GenerateMode,
        progress: (GenerateProgress) -> Unit,
    ): MorphResult {
        val body = body(appId, base(currentVersion), transcript, locale, mode)
        return try {
            generateStreaming(body, progress)
        } catch (e: ApiError) {
            if (e.status != 409) throw e
            progress(GenerateProgress.Restart(0))
            generateStreaming(body.copy(baseVersion = null), progress)
        }.also { remember(it) }
    }

    /** iOS の `GenerateStream.finish`。保存に失敗しても生成は成功のまま(DocumentCache が飲み込む)。 */
    private suspend fun remember(result: MorphResult) {
        cache?.store(result.document.appId, result.envelopeJson)
    }

    override suspend fun chat(appId: String, messages: List<ChatMessage>, locale: String?, mode: ChatMode?): ChatTurn {
        val body = ChatBody(
            appId = appId,
            messages = messages,
            locale = locale ?: http.language(),
            mode = mode,
            allowedComponents = capabilities.allowedComponents,
            disabledFeatures = capabilities.disabledFeatures,
            supportedFeatures = capabilities.supportedFeatures,
        )
        val json = http.send(
            HttpRequest("POST", listOf("v1", "chat"), body = ApiJson.encodeToString(ChatBody.serializer(), body), timeoutMillis = timeoutMillis),
        )
        return ApiJson.decodeFromString(ChatTurn.serializer(), json)
    }

    override suspend fun resetChat(appId: String) {
        try {
            http.send(HttpRequest("DELETE", listOf("v1", "chat", appId), timeoutMillis = 10_000))
        } catch (e: CancellationException) {
            throw e
        } catch (e: Exception) {
            log("resetChat failed: $e") // サーバー側は 30 分で消える
        }
    }

    override suspend fun text(request: AiTextRequest): AiTextResult = AiTextCall.send(http, request)

    // MARK: generate

    private fun base(currentVersion: Int): Int? = currentVersion.takeIf { it > 0 }

    private fun body(appId: String, baseVersion: Int?, transcript: String, locale: String?, mode: GenerateMode) = GenerateBody(
        appId = appId,
        baseVersion = baseVersion,
        transcript = transcript,
        mode = mode.wire,
        locale = locale ?: http.language(),
        allowedComponents = capabilities.allowedComponents,
        disabledFeatures = capabilities.disabledFeatures,
        supportedFeatures = capabilities.supportedFeatures,
    )

    private fun request(body: GenerateBody, stream: Boolean) = HttpRequest(
        method = "POST",
        path = listOf("v1", "generate"),
        query = if (stream) listOf("stream" to "1") else emptyList(),
        body = ApiJson.encodeToString(GenerateBody.serializer(), body),
        headers = if (stream) mapOf("Accept" to "text/event-stream") else emptyMap(),
        timeoutMillis = timeoutMillis,
    )

    internal suspend fun generate(body: GenerateBody): MorphResult =
        parse(http.send(request(body, stream = false)), body.appId)

    /**
     * 途中経過付きの生成。サーバーが返したエラー(`event: error`・401 / 429 以外の 4xx/5xx)と読めない結果はそのまま投げる。
     * [progress] は読み込みのスレッド(IO)から呼ぶ。
     */
    internal suspend fun generateStreaming(body: GenerateBody, progress: (GenerateProgress) -> Unit): MorphResult {
        try {
            return streamOnce(body, progress)
        } catch (e: ApiError) {
            throw e
        } catch (e: SerializationException) {
            throw e // 結果は届いたが読めない: 通常の呼び方と同じく失敗(生成をやり直さない)
        } catch (e: CancellationException) {
            throw e
        } catch (e: Exception) {
            currentCoroutineContext().ensureActive()
            log("generate stream unavailable ($e) -> plain request")
        }
        progress(GenerateProgress.Restart(0))
        return generate(body)
    }

    private suspend fun streamOnce(body: GenerateBody, progress: (GenerateProgress) -> Unit): MorphResult =
        http.openStream(request(body, stream = true)).use { response ->
            val status = response.code
            // 401 / 429 は通常の呼び方の再送(トークンの取り直し・Retry-After)に任せる
            if (status == 401 || status == 429) throw StreamUnavailable("status $status")
            val contentType = response.header("Content-Type").orEmpty()
            withContext(Dispatchers.IO) {
                if (status !in 200..299 || !contentType.contains("text/event-stream")) {
                    // stream を知らないサーバー(JSON で返った)・エラーの応答: 本文を全部読んで通常どおりに扱う
                    val r = HttpResponse(status, response.body.string(), response.headers)
                    return@withContext parse(http.check(r), body.appId)
                }
                val source = response.body.source()
                val parser = SseParser()
                while (true) {
                    val line = source.readUtf8Line() ?: break
                    val frame = parser.feedLine(line) ?: continue
                    when (val event = GenerateStreamEvent.decode(frame)) {
                        is GenerateStreamEvent.Progress -> progress(event.progress)
                        is GenerateStreamEvent.Result -> return@withContext parse(event.json, body.appId)
                        is GenerateStreamEvent.Failure -> throw event.error
                        null -> Unit
                    }
                }
                // 区切りの空行が来ないまま終わった最後の 1 件は捨てる(SSE の決まり)
                throw StreamUnavailable("truncated")
            }
        }

    companion object {
        /** 応答本文 → 結果。`envelopeJson` は appconfig の `DSLEnvelope` 形(キャッシュ用)。 */
        fun parse(json: String, appId: String): MorphResult {
            val obj = ApiJson.parseToJsonElement(json).jsonObject
            val dsl = obj["dsl"] as? JsonObject ?: throw SerializationException("generate response has no dsl")
            val version = (obj["version"] as? JsonPrimitive)?.intOrNull
                ?: throw SerializationException("generate response has no version")
            val resolvedAppId = (obj["appId"] as? JsonPrimitive)?.contentOrNull ?: appId
            val document = AppDocument(resolvedAppId, version, DslJson.decodeFromJsonElement(AppDSL.serializer(), dsl))
            val meta = obj["meta"] as? JsonObject
            return MorphResult(
                document = document,
                summary = (obj["summary"] as? JsonPrimitive)?.contentOrNull,
                mode = (obj["mode"] as? JsonPrimitive)?.contentOrNull,
                unchanged = (obj["unchanged"] as? JsonPrimitive)?.contentOrNull == "true",
                publicPersonalData = (meta?.get("publicPersonalData") as? JsonArray).orEmpty().filterIsInstance<JsonObject>(),
                envelopeJson = buildJsonObject {
                    put("appId", resolvedAppId)
                    put("version", version)
                    put("dsl", dsl)
                }.toString(),
            )
        }
    }
}
