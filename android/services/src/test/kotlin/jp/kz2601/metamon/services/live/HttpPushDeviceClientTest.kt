package jp.kz2601.metamon.services.live

import jp.kz2601.metamon.services.http.ApiJson
import jp.kz2601.metamon.services.http.json
import jp.kz2601.metamon.services.http.metamonHttp
import jp.kz2601.metamon.services.http.take
import jp.kz2601.metamon.services.push.PushDeviceRegistration
import kotlinx.coroutines.runBlocking
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.jsonObject
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Before
import org.junit.Test

class HttpPushDeviceClientTest {
    private lateinit var server: MockWebServer
    private lateinit var client: HttpPushDeviceClient

    @Before
    fun setUp() {
        server = MockWebServer().apply { start() }
        client = HttpPushDeviceClient(server.metamonHttp())
    }

    @After
    fun tearDown() {
        server.shutdown()
    }

    @Test
    fun putDeviceSendsTheTokenWithThePlatform() = runBlocking {
        server.enqueue(json(200, """{"installationId":"install-1"}"""))
        client.putDevice("install-1", PushDeviceRegistration(token = "fcm-a", appVersion = "0.1.0", os = "Android 15"))
        val req = server.take()
        assertEquals("PUT", req.method)
        assertEquals("/v1/me/devices/install-1", req.path)
        val body = ApiJson.parseToJsonElement(req.body.readUtf8()).jsonObject
        assertEquals(
            JsonObject(
                mapOf(
                    "token" to JsonPrimitive("fcm-a"),
                    "environment" to JsonPrimitive("production"),
                    "platform" to JsonPrimitive("android"),
                    "appVersion" to JsonPrimitive("0.1.0"),
                    "os" to JsonPrimitive("Android 15"),
                ),
            ),
            body,
        )
    }

    @Test
    fun deleteDeviceAndConsent() = runBlocking {
        server.enqueue(MockResponse().setResponseCode(204))
        server.enqueue(json(200, """{"appId":"kakeibo-1","allowed":true,"updatedAt":"2026-11-11T00:00:00Z"}"""))
        client.deleteDevice("install-1")
        client.setNotifyConsent("kakeibo-1", true)
        val del = server.take()
        assertEquals("DELETE", del.method)
        assertEquals("/v1/me/devices/install-1", del.path)
        val consent = server.take()
        assertEquals("PUT", consent.method)
        assertEquals("/v1/me/notify-consent/kakeibo-1", consent.path)
        assertEquals("""{"allowed":true}""", consent.body.readUtf8())
    }
}
