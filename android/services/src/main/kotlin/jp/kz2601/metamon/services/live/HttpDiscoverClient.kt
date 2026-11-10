package jp.kz2601.metamon.services.live

import jp.kz2601.metamon.services.InstallResult
import jp.kz2601.metamon.services.discover.DiscoverClient
import jp.kz2601.metamon.services.discover.DiscoverSort
import jp.kz2601.metamon.services.discover.PublicApp
import jp.kz2601.metamon.services.discover.PublicAppPage
import jp.kz2601.metamon.services.discover.PublishRequest
import jp.kz2601.metamon.services.http.ApiJson
import jp.kz2601.metamon.services.http.HttpRequest
import jp.kz2601.metamon.services.http.MetamonHttp
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.put

/** 「探す」の REST 実装(iOS の `HTTPSharingClient` の public-apps / publish)。 */
class HttpDiscoverClient(private val http: MetamonHttp) : DiscoverClient {
    override suspend fun search(query: String, sort: DiscoverSort, pageToken: String?): PublicAppPage {
        val body = buildJsonObject {
            query.trim().takeIf { it.isNotEmpty() }?.let { put("q", it) }
            put("sort", sort.wire)
            put("pageSize", PAGE_SIZE)
            pageToken?.let { put("pageToken", it) }
        }
        val text = http.send(HttpRequest("POST", listOf("v1", "public-apps", "search"), body = body.toString(), timeoutMillis = TIMEOUT))
        return ApiJson.decodeFromString(PublicAppPage.serializer(), text)
    }

    override suspend fun install(publicAppId: String): InstallResult {
        val text = http.send(HttpRequest("POST", listOf("v1", "public-apps", publicAppId, "install"), body = "{}", timeoutMillis = TIMEOUT))
        return ApiJson.decodeFromString(InstallResult.serializer(), text)
    }

    override suspend fun publish(appId: String, request: PublishRequest): PublicApp {
        val body = buildJsonObject {
            request.name?.let { put("name", it) }
            request.description?.let { put("description", it) }
            if (request.tags.isNotEmpty()) put("tags", JsonArray(request.tags.map(::JsonPrimitive)))
            request.authorName?.let { put("authorName", it) }
        }
        val text = http.send(HttpRequest("POST", listOf("v1", "apps", appId, "publish"), body = body.toString(), timeoutMillis = TIMEOUT))
        return ApiJson.decodeFromString(PublicApp.serializer(), text)
    }

    override suspend fun unpublish(appId: String) {
        http.send(HttpRequest("DELETE", listOf("v1", "apps", appId, "publish"), timeoutMillis = TIMEOUT))
    }

    private companion object {
        const val PAGE_SIZE = 50
        const val TIMEOUT = 20_000L
    }
}
