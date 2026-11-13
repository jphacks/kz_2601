package jp.kz2601.metamon.desktop.net

import jp.kz2601.metamon.services.http.ClientInfo
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import okhttp3.mockwebserver.RecordedRequest
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Before
import org.junit.Test

/** DesktopClientInterceptor: 実際に送られた要求に OS の申告と User-Agent が付き、元の見出しは残る。 */
class DesktopClientInterceptorTest {
    private lateinit var server: MockWebServer

    @Before
    fun setUp() {
        server = MockWebServer().apply { start() }
    }

    @After
    fun tearDown() {
        server.shutdown()
    }

    private fun send(platform: ClientPlatform, userAgent: String, build: Request.Builder.() -> Unit = {}): RecordedRequest {
        server.enqueue(MockResponse().setBody("{}"))
        val http = OkHttpClient.Builder().addInterceptor(DesktopClientInterceptor(platform, userAgent)).build()
        val request = Request.Builder().url(server.url("/v1/apps")).apply(build).build()
        http.newCall(request).execute().use { assertEquals(200, it.code) }
        return server.takeRequest()
    }

    @Test
    fun windowsRequestCarriesPlatformAndUserAgent() {
        val ua = desktopUserAgent("0.1.0", "Windows 11", "10.0", "amd64")
        val req = send(ClientPlatform.WINDOWS, ua)
        assertEquals("windows", req.getHeader(ClientInfo.PLATFORM_HEADER))
        assertEquals("TailorIt-Desktop/0.1.0 (Windows 11 10.0; amd64)", req.getHeader("User-Agent"))
    }

    @Test
    fun linuxOverridesTheSharedAndroidPlatformAndKeepsOtherHeaders() {
        val req = send(ClientPlatform.LINUX, "ua-linux") {
            // 共有の MetamonHttp が先に付ける見出し(android の申告・版)を真似る
            header(ClientInfo.PLATFORM_HEADER, "android")
            header("X-Client-Version", "0.1.0")
            header("Authorization", "Bearer t")
            header("User-Agent", "okhttp/old")
        }
        assertEquals("linux", req.getHeader(ClientInfo.PLATFORM_HEADER))
        assertEquals("ua-linux", req.getHeader("User-Agent"))
        assertEquals("0.1.0", req.getHeader("X-Client-Version"))
        assertEquals("Bearer t", req.getHeader("Authorization"))
        assertEquals("上書きで二重にならない", 1, req.headers.values(ClientInfo.PLATFORM_HEADER).size)
        assertEquals(1, req.headers.values("User-Agent").size)
    }
}
