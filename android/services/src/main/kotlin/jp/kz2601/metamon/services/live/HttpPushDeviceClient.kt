package jp.kz2601.metamon.services.live

import jp.kz2601.metamon.services.http.ApiJson
import jp.kz2601.metamon.services.http.HttpRequest
import jp.kz2601.metamon.services.http.MetamonHttp
import jp.kz2601.metamon.services.push.PushDeviceClient
import jp.kz2601.metamon.services.push.PushDeviceRegistration
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.put

/** appconfig の端末登録と同意(iOS の `HTTPPushDeviceClient`)。トークンはログに出さない。 */
class HttpPushDeviceClient(private val http: MetamonHttp) : PushDeviceClient {
    override suspend fun putDevice(installationId: String, registration: PushDeviceRegistration) {
        http.send(HttpRequest("PUT", listOf("v1", "me", "devices", installationId), body = ApiJson.encodeToString(PushDeviceRegistration.serializer(), registration)))
    }

    override suspend fun deleteDevice(installationId: String) {
        http.send(HttpRequest("DELETE", listOf("v1", "me", "devices", installationId)))
    }

    override suspend fun setNotifyConsent(appId: String, allowed: Boolean) {
        http.send(HttpRequest("PUT", listOf("v1", "me", "notify-consent", appId), body = buildJsonObject { put("allowed", allowed) }.toString()))
    }
}
