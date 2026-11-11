package jp.kz2601.metamon.services.live

import jp.kz2601.metamon.services.automations.AppAutomations
import jp.kz2601.metamon.services.automations.AutomationToggle
import jp.kz2601.metamon.services.automations.AutomationsClient
import jp.kz2601.metamon.services.http.ApiJson
import jp.kz2601.metamon.services.http.HttpRequest
import jp.kz2601.metamon.services.http.MetamonHttp
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.buildJsonObject

/** appconfig の自動の動き(docs/backend/openapi-appconfig.yaml `/apps/{appId}/automations`・#905)。iOS の `HTTPAutomationsClient`。 */
class HttpAutomationsClient(private val http: MetamonHttp) : AutomationsClient {
    override suspend fun automations(appId: String): AppAutomations {
        val body = http.send(HttpRequest("GET", listOf("v1", "apps", appId, "automations"), timeoutMillis = TIMEOUT))
        return ApiJson.decodeFromString(AppAutomations.serializer(), body)
    }

    /** 今の DSL を読んで id から位置を引き、その version を If-Match にして PATCH する(間に変わっていれば 412)。 */
    override suspend fun setRule(ruleId: String, enabled: Boolean, appId: String) {
        val path = listOf("v1", "apps", appId, "dsl")
        val current = AutomationToggle.parse(http.send(HttpRequest("GET", path, timeoutMillis = TIMEOUT)))
        val ops = AutomationToggle.ops(ruleId, enabled, current.serverRules)
        if (ops.isEmpty()) return
        val body = buildJsonObject { put("patch", JsonArray(ops)) }.toString()
        http.send(
            HttpRequest(
                "PATCH", path, body = body, timeoutMillis = TIMEOUT,
                headers = mapOf("If-Match" to HttpAppConfigClient.etag(current.version)),
            ),
        )
    }

    private companion object {
        const val TIMEOUT = 20_000L
    }
}
