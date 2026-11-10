package jp.kz2601.metamon.services.live

import jp.kz2601.metamon.dsl.ai.AiTextError
import jp.kz2601.metamon.dsl.ai.AiTextRequest
import jp.kz2601.metamon.dsl.ai.AiTextResult
import jp.kz2601.metamon.services.http.ApiError
import jp.kz2601.metamon.services.http.ApiJson
import jp.kz2601.metamon.services.http.ErrorCodes
import jp.kz2601.metamon.services.http.HttpRequest
import jp.kz2601.metamon.services.http.MetamonHttp
import kotlinx.serialization.SerializationException

/**
 * `POST /v1/text`(#417・契約: backend/internal/ai/text.go / openapi-ai.yaml `/text`。iOS の `HTTPAIClient+Text`)。
 * 答えはサーバーが整えた後でも端末で同じ規則([AiTextResult.normalize])で確かめる。
 */
internal object AiTextCall {
    const val CREATOR_NEEDS_PRO = "creator_needs_pro"

    suspend fun send(http: MetamonHttp, request: AiTextRequest): AiTextResult {
        val json = try {
            http.send(
                HttpRequest(
                    "POST",
                    listOf("v1", "text"),
                    body = ApiJson.encodeToString(AiTextRequest.serializer(), request),
                    // long は出力が長い(サーバーは 60 秒まで待つ)
                    timeoutMillis = if (request.isLong) 75_000 else 30_000,
                ),
            )
        } catch (e: ApiError) {
            throw error(e)
        }
        return parse(json, request) ?: throw AiTextError.BadOutput
    }

    fun parse(json: String, request: AiTextRequest): AiTextResult? {
        val raw = try {
            ApiJson.decodeFromString(AiTextResult.serializer(), json)
        } catch (_: SerializationException) {
            return null
        } catch (_: IllegalArgumentException) {
            return null
        }
        return AiTextResult.normalize(raw.text, raw.items, raw.fields, request)
    }

    /** サーバーの失敗を部品が出す理由に寄せる。利用上限(rate_limited)などはサーバーの文をそのまま出す。 */
    fun error(e: ApiError): Exception = when (e.code) {
        ErrorCodes.REFUSED -> AiTextError.Refused
        ErrorCodes.BAD_OUTPUT -> AiTextError.BadOutput
        // サーバーも Pro を確かめる。共有のアプリの参加者は作成者の Pro が要る(課金画面は出さない)
        ErrorCodes.PRO_REQUIRED -> if (e.reason == CREATOR_NEEDS_PRO) AiTextError.CreatorNeedsPro else AiTextError.NeedsPro
        ErrorCodes.AI_PAUSED -> AiTextError.Paused
        ErrorCodes.PAYLOAD_TOO_LARGE -> AiTextError.TooLarge
        // 1 日の利用上限など: サーバーの日本語をそのまま出す
        ErrorCodes.RATE_LIMITED -> AiTextError.Failed((e as? ApiError.Server)?.serverMessage?.ifEmpty { null } ?: e.message.orEmpty())
        else -> e
    }
}
