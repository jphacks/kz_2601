package jp.kz2601.metamon.desktop.net

import jp.kz2601.metamon.services.http.ClientInfo
import jp.kz2601.metamon.services.live.HttpAppConfigClient
import kotlinx.coroutines.runBlocking
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.File

/** サーバーに送る版の申告(DesktopApi.clientInfo): VERSION・BUILD が入り、User-Agent・配布物の版と揃う。 */
class DesktopApiClientInfoTest {
    @Test
    fun clientInfoMatchesUserAgent() {
        val info = DesktopApi.clientInfo()
        assertEquals(DesktopApi.VERSION, info.version)
        assertEquals(DesktopApi.BUILD, info.build)
        assertEquals(ClientInfo.SUPPORTED_DSL_VERSION, info.dslVersion)
        assertEquals(DesktopApi.VERSION, info.headers["X-Client-Version"])
        assertEquals(DesktopApi.BUILD, info.headers["X-Client-Build"])
        assertTrue(desktopUserAgent(DesktopApi.VERSION, "Windows 11", "10.0", "amd64").startsWith("TailorIt-Desktop/${info.version} "))
    }

    @Test
    fun sentHeadersCarryTheSameVersionAsUserAgent() = runBlocking {
        val server = MockWebServer().apply { start() }
        try {
            server.enqueue(MockResponse().setHeader("Content-Type", "application/json").setBody("""{"items":[]}"""))
            // 既定の User-Agent(desktopUserAgent(VERSION))のまま送る
            HttpAppConfigClient(DesktopApi.http(server.url("/").toString(), DesktopApi.devTokens(null), ClientPlatform.LINUX)).listApps()
            val req = server.takeRequest()
            val ua = req.getHeader("User-Agent")!!
            val uaVersion = Regex("^TailorIt-Desktop/([^ ]+) ").find(ua)?.groupValues?.get(1)
            assertEquals(ua, req.getHeader("X-Client-Version"), uaVersion)
            assertEquals(DesktopApi.BUILD, req.getHeader("X-Client-Build"))
            assertEquals(ClientInfo.SUPPORTED_DSL_VERSION.toString(), req.getHeader("X-Client-DSL-Version"))
        } finally {
            server.shutdown()
        }
    }

    @Test
    fun versionMatchesPackageVersion() {
        // jpackage の packageVersion(desktop/app/build.gradle.kts)と合わせる
        val gradle = File("build.gradle.kts").readText()
        val packageVersion = Regex("""packageVersion\s*=\s*"([^"]+)"""").find(gradle)?.groupValues?.get(1)
        assertEquals(packageVersion, DesktopApi.VERSION)
    }
}
