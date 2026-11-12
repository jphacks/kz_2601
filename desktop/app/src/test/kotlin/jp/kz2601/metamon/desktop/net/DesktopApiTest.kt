package jp.kz2601.metamon.desktop.net

import jp.kz2601.metamon.services.live.HttpAppConfigClient
import kotlinx.coroutines.runBlocking
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import kotlinx.serialization.json.put
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test

/** API クライアントの共有(#1748): 申告のヘッダと、appconfig へのリクエストの形(通信は MockWebServer だけ)。 */
class DesktopApiTest {
    private lateinit var server: MockWebServer

    @Before
    fun setUp() {
        server = MockWebServer().apply { start() }
    }

    @After
    fun tearDown() {
        server.shutdown()
    }

    private fun client(platform: ClientPlatform, token: String? = "dev-token", userAgent: String = "ua"): HttpAppConfigClient {
        val http = DesktopApi.http(
            baseUrl = server.url("/").toString(),
            tokens = DesktopApi.devTokens(token),
            platform = platform,
            client = DesktopApi.okHttp(platform, userAgent),
        )
        return HttpAppConfigClient(http)
    }

    private fun json(body: String) = MockResponse().setHeader("Content-Type", "application/json").setBody(body)

    @Test
    fun platformFromOsName() {
        assertEquals(ClientPlatform.WINDOWS, ClientPlatform.from("Windows 11"))
        assertEquals(ClientPlatform.WINDOWS, ClientPlatform.from("Windows Server 2022"))
        assertEquals(ClientPlatform.LINUX, ClientPlatform.from("Linux"))
        assertEquals(ClientPlatform.MACOS, ClientPlatform.from("Mac OS X"))
        assertEquals(ClientPlatform.LINUX, ClientPlatform.from("FreeBSD"))
        assertEquals(listOf("windows", "linux", "macos"), ClientPlatform.entries.map { it.header })
    }

    @Test
    fun userAgentCarriesOsNameAndVersion() {
        assertEquals("TailorIt-Desktop/0.1.0 (Windows 11 10.0; amd64)", desktopUserAgent("0.1.0", "Windows 11", "10.0", "amd64"))
    }

    @Test
    fun sendsPlatformHeaderPerOs() = runBlocking {
        for (platform in ClientPlatform.entries) {
            server.enqueue(json("""{"items":[{"id":"a1","name":"家計簿","currentVersion":3}]}"""))
            val apps = client(platform, userAgent = desktopUserAgent("0.1.0", "OS-${platform.header}", "1")).listApps()
            assertEquals("家計簿", apps.single().name)

            val req = server.takeRequest()
            assertEquals(platform.header, req.getHeader("X-Client-Platform"))
            assertTrue(req.getHeader("User-Agent")!!.contains("OS-${platform.header} 1"))
            assertEquals("0.1.0", req.getHeader("X-Client-Version"))
            assertEquals("Bearer dev-token", req.getHeader("Authorization"))
            assertEquals("GET", req.method)
            assertEquals("/v1/apps?pageSize=200", req.path)
        }
    }

    @Test
    fun noTokenMeansNoAuthorization() = runBlocking {
        server.enqueue(json("""{"items":[]}"""))
        client(ClientPlatform.LINUX, token = null).listApps()
        assertNull(server.takeRequest().getHeader("Authorization"))
    }

    @Test
    fun appConfigRequestShapes() = runBlocking {
        val c = client(ClientPlatform.WINDOWS)
        val dsl = """{"schemaVersion":1,"app":{"name":"家計簿"},"initialScreen":"home","screens":[{"id":"home","title":"ホーム","components":[]}]}"""
        server.enqueue(json("""{"appId":"a1","version":4,"dsl":$dsl}"""))
        server.enqueue(json("""{"id":"r1","entity":"items","data":{"name":"牛乳"}}"""))
        server.enqueue(json("""{"id":"r1","entity":"items","data":{"name":"豆乳"}}"""))
        server.enqueue(MockResponse().setResponseCode(204))

        assertEquals(4, c.fetchDocument("a1").version)
        val created = c.createRecord("a1", "items", buildJsonObject { put("name", "牛乳") }, clientId = "c-1", authorName = "こた")
        assertEquals("r1", created.id)
        c.updateRecord("a1", "items", "r1", buildJsonObject { put("name", "豆乳") })
        c.deleteRecord("a1", "items", "r1")

        val get = server.takeRequest()
        assertEquals("GET /v1/apps/a1/dsl", "${get.method} ${get.path}")

        val post = server.takeRequest()
        assertEquals("POST /v1/apps/a1/entities/items/records", "${post.method} ${post.path}")
        val body = Json.parseToJsonElement(post.body.readUtf8()).jsonObject
        assertEquals("c-1", body["clientId"]!!.jsonPrimitive.content)
        assertEquals("こた", body["authorName"]!!.jsonPrimitive.content)
        assertEquals(JsonPrimitive("牛乳"), body["data"]!!.jsonObject["name"])
        assertEquals("windows", post.getHeader("X-Client-Platform"))

        val patch = server.takeRequest()
        assertEquals("PATCH /v1/apps/a1/entities/items/records/r1", "${patch.method} ${patch.path}")
        assertEquals(JsonPrimitive("豆乳"), Json.parseToJsonElement(patch.body.readUtf8()).jsonObject["data"]!!.jsonObject["name"])

        val delete = server.takeRequest()
        assertEquals("DELETE /v1/apps/a1/entities/items/records/r1", "${delete.method} ${delete.path}")
    }

    @Test
    fun configDefaultsToMockAndHidesToken() {
        val mock = DesktopApiConfig.fromEnvironment(emptyMap())
        assertFalse(mock.isLive)
        assertNull(DesktopApi.appConfig(mock))

        val stg = DesktopApiConfig.fromEnvironment(mapOf("TAILORIT_API" to "stg", "TAILORIT_DEV_TOKEN" to "secret-token"))
        assertEquals(DesktopApiConfig.Mode.STG, stg.mode)
        assertEquals(DesktopApiConfig.STG_APPCONFIG_URL, stg.appConfigUrl)
        assertEquals("secret-token", stg.devToken)
        assertFalse(stg.toString().contains("secret-token"))
        assertTrue(DesktopApi.appConfig(stg) != null)

        // prod はまだ選べない(知らない値はモック)
        assertFalse(DesktopApiConfig.fromEnvironment(mapOf("TAILORIT_API" to "prod")).isLive)
        val local = DesktopApiConfig.fromEnvironment(mapOf("TAILORIT_API" to "stg", "TAILORIT_APPCONFIG_URL" to "http://localhost:8081"))
        assertEquals("http://localhost:8081", local.appConfigUrl)
    }
}
