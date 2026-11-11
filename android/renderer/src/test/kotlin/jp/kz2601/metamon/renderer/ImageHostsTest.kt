package jp.kz2601.metamon.renderer

import jp.kz2601.metamon.renderer.components.ImageHosts
import jp.kz2601.metamon.renderer.components.ImageRedirects
import jp.kz2601.metamon.renderer.components.PhotoValue
import jp.kz2601.metamon.renderer.components.PublicOnlyDns
import jp.kz2601.metamon.renderer.components.RemoteImages
import okhttp3.Dns
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Assert.fail
import org.junit.Test
import java.io.IOException
import java.net.InetAddress
import java.net.UnknownHostException

/** 画像を読む先は公開のホストだけ。 */
class ImageHostsTest {
    @Test
    fun loadableUrlRejectsLocalAndLiteralHosts() {
        assertEquals("https://images.example.com/a.jpg", RemoteImages.loadableUrl("https://images.example.com/a.jpg"))
        assertEquals("https://example.com./a.jpg", RemoteImages.loadableUrl("https://example.com./a.jpg"))
        listOf(
            "https://localhost/a.jpg",
            "https://LOCALHOST:8443/a.jpg",
            "https://foo.localhost/a.jpg",
            "https://printer.local/a.jpg",
            "https://svc.internal/a.jpg",
            "https://nas.home.arpa/a.jpg",
            "https://router/a.jpg",
            "https://127.0.0.1/a.jpg",
            "https://10.0.0.5/a.jpg",
            "https://192.168.1.1/a.jpg",
            "https://8.8.8.8/a.jpg",
            "https://2130706433/a.jpg",
            "https://0x7f000001/a.jpg",
            "https://0x7f.0.0.1/a.jpg",
            "https://127.1/a.jpg",
            "https://[::1]/a.jpg",
            "https://[2606:4700::1111]/a.jpg",
        ).forEach { assertNull(it, RemoteImages.loadableUrl(it)) }
        // 読まない画像は写真なしの枠
        assertFalse(PhotoValue.hasPhoto("https://localhost/a.jpg"))
    }

    @Test
    fun publicNames() {
        assertTrue(ImageHosts.isPublicName("cdn.example.co.jp"))
        assertTrue(ImageHosts.isPublicName("localhost.example.com"))
        assertFalse(ImageHosts.isPublicName(null))
        assertFalse(ImageHosts.isPublicName(""))
        assertFalse(ImageHosts.isPublicName("a..b"))
        assertFalse(ImageHosts.isPublicName("::1"))
    }

    @Test
    fun publicAddresses() {
        fun ip(s: String) = InetAddress.getByName(s)
        listOf("8.8.8.8", "1.1.1.1", "203.0.114.1", "100.128.0.1", "172.32.0.1", "2606:4700::1111", "64:ff9b::808:808")
            .forEach { assertTrue(it, ImageHosts.isPublicAddress(ip(it))) }
        listOf(
            "0.0.0.0", "10.1.2.3", "100.64.0.1", "100.127.255.255", "127.0.0.1", "169.254.169.254",
            "172.16.0.1", "172.31.255.255", "192.168.0.1", "192.0.0.1", "198.18.0.1", "224.0.0.1", "255.255.255.255",
            "::", "::1", "fc00::1", "fd12:3456::1", "fe80::1", "fec0::1", "ff02::1", "::ffff:127.0.0.1",
            "::ffff:10.0.0.1", "64:ff9b::a00:1", "2002:c0a8:101::1", "2001:db8::1",
        ).forEach { assertFalse(it, ImageHosts.isPublicAddress(ip(it))) }
    }

    @Test
    fun dnsDropsPrivateAnswers() {
        val answers = mapOf(
            "rebind.example.com" to listOf("127.0.0.1", "10.0.0.1"),
            "mixed.example.com" to listOf("192.168.0.10", "93.184.216.34"),
            "public.example.com" to listOf("93.184.216.34"),
        )
        val base = Dns { host -> answers.getValue(host).map { InetAddress.getByName(it) } }
        val dns = PublicOnlyDns(base)
        assertEquals(listOf(InetAddress.getByName("93.184.216.34")), dns.lookup("public.example.com"))
        assertEquals(listOf(InetAddress.getByName("93.184.216.34")), dns.lookup("mixed.example.com"))
        expect<UnknownHostException> { dns.lookup("rebind.example.com") }
        expect<UnknownHostException> { dns.lookup("localhost") }
    }

    @Test
    fun redirectsAreCheckedBeforeFollowing() {
        MockWebServer().use { server ->
            server.enqueue(MockResponse().setResponseCode(302).addHeader("Location", "/ok.jpg"))
            server.enqueue(MockResponse().setBody("ok"))
            server.enqueue(MockResponse().setResponseCode(302).addHeader("Location", "/blocked.jpg"))
            server.start()
            val client = OkHttpClient.Builder()
                .followRedirects(false)
                .addInterceptor(ImageRedirects({ !it.encodedPath.startsWith("/blocked") }))
                .build()
            fun get(path: String) = client.newCall(Request.Builder().url(server.url(path)).build()).execute().use { it.body.string() }
            assertEquals("ok", get("/a.jpg"))
            expect<IOException> { get("/b.jpg") }
            expect<IOException> { get("/blocked-first.jpg") }
            // 止めた先には接続しない
            assertEquals(3, server.requestCount)
        }
    }

    @Test
    fun imageClientRefusesLocalServer() {
        MockWebServer().use { server ->
            server.enqueue(MockResponse().setBody("x"))
            server.start()
            val images = RemoteImages.imageClient(OkHttpClient(), requireHttps = false, publicHostsOnly = true)
            expect<IOException> { images.newCall(Request.Builder().url(server.url("/a.jpg")).build()).execute().close() }
            assertEquals(0, server.requestCount)
        }
    }

    private inline fun <reified T : Throwable> expect(block: () -> Unit) {
        try {
            block()
            fail("${T::class.simpleName} が出るはず")
        } catch (e: Throwable) {
            if (e !is T) throw e
        }
    }
}
