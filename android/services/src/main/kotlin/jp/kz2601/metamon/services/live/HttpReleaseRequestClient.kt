package jp.kz2601.metamon.services.live

import jp.kz2601.metamon.services.http.ApiJson
import jp.kz2601.metamon.services.http.HttpRequest
import jp.kz2601.metamon.services.http.MetamonHttp
import jp.kz2601.metamon.services.release.ReleaseRequest
import jp.kz2601.metamon.services.release.ReleaseRequestClient
import kotlinx.serialization.Serializable

/** リリース申請の API(docs/backend/openapi-appconfig.yaml `/apps/{appId}/release-requests`・`/release-requests`)。iOS の `HTTPReleaseRequestClient`。 */
class HttpReleaseRequestClient(private val http: MetamonHttp) : ReleaseRequestClient {
    override suspend fun request(appId: String, purpose: String, contact: String?): ReleaseRequest {
        val body = ApiJson.encodeToString(Body.serializer(), Body(purpose, contact))
        val req = HttpRequest("POST", listOf("v1", "apps", appId, "release-requests"), body = body, timeoutMillis = TIMEOUT)
        return ApiJson.decodeFromString(ReleaseRequest.serializer(), http.send(req))
    }

    override suspend fun myRequests(): List<ReleaseRequest> {
        val body = http.send(HttpRequest("GET", listOf("v1", "release-requests"), timeoutMillis = TIMEOUT))
        return ApiJson.decodeFromString(ListResponse.serializer(), body).items
    }

    /** contact が無ければ送らない(backend は未知のキーを 400 にするので、増やさない) */
    @Serializable
    private data class Body(val purpose: String, val contact: String? = null)

    @Serializable
    private data class ListResponse(val items: List<ReleaseRequest> = emptyList())

    private companion object {
        const val TIMEOUT = 20_000L
    }
}
