package jp.kz2601.metamon.services.live

import jp.kz2601.metamon.services.follows.FollowsClient
import jp.kz2601.metamon.services.http.ApiJson
import jp.kz2601.metamon.services.http.HttpRequest
import jp.kz2601.metamon.services.http.MetamonHttp
import kotlinx.serialization.Serializable

/** フォローの API(docs/backend/openapi-appconfig.yaml `/apps/{appId}/follows`・#270)。iOS の `HTTPFollowsClient`。 */
class HttpFollowsClient(private val http: MetamonHttp) : FollowsClient {
    override suspend fun list(appId: String): List<String> {
        val body = http.send(HttpRequest("GET", listOf("v1", "apps", appId, "follows"), timeoutMillis = TIMEOUT))
        return ApiJson.decodeFromString(ListResponse.serializer(), body).uids
    }

    override suspend fun setFollow(appId: String, uid: String, on: Boolean): Boolean {
        val req = HttpRequest(if (on) "PUT" else "DELETE", listOf("v1", "apps", appId, "follows", uid), timeoutMillis = TIMEOUT)
        return ApiJson.decodeFromString(SetResponse.serializer(), http.send(req)).following
    }

    @Serializable
    private data class ListResponse(val uids: List<String> = emptyList())

    @Serializable
    private data class SetResponse(val following: Boolean)

    private companion object {
        const val TIMEOUT = 20_000L
    }
}
