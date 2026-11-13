package jp.kz2601.metamon.desktop.share

import jp.kz2601.metamon.desktop.i18n.Lang
import jp.kz2601.metamon.desktop.i18n.StringTable
import jp.kz2601.metamon.desktop.net.ClientPlatform
import jp.kz2601.metamon.desktop.net.DesktopApi
import jp.kz2601.metamon.services.Share
import jp.kz2601.metamon.services.live.HttpAppConfigClient
import jp.kz2601.metamon.services.sharing.IssuedShares
import jp.kz2601.metamon.services.sharing.ShareFailure
import jp.kz2601.metamon.services.sharing.ShareNotice
import kotlinx.coroutines.runBlocking
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test
import java.awt.datatransfer.Clipboard
import java.awt.datatransfer.DataFlavor

/** 共有のリンク(#2503): 発行・コピー・QR・取り消し・失敗の文言(通信は MockWebServer だけ)。 */
class ShareLinkTest {
    private lateinit var server: MockWebServer
    private val target = ShareTarget("app1", "家計簿")

    @Before
    fun setUp() {
        server = MockWebServer().apply { start() }
    }

    @After
    fun tearDown() {
        server.shutdown()
    }

    private fun client(): HttpAppConfigClient {
        val http = DesktopApi.http(
            baseUrl = server.url("/").toString(),
            tokens = DesktopApi.devTokens("dev-token"),
            platform = ClientPlatform.WINDOWS,
            client = DesktopApi.okHttp(ClientPlatform.WINDOWS, "ua"),
        )
        return HttpAppConfigClient(http)
    }

    private fun json(code: Int, body: String) = MockResponse().setResponseCode(code).setHeader("Content-Type", "application/json").setBody(body)

    private fun shareBody(token: String) =
        """{"token":"$token","appId":"app1","name":"家計簿","dslVersion":3,"url":"tailorit://import/$token","webUrl":"https://tailor-it.app/s/$token"}"""

    private fun error(code: String, reason: String? = null): String {
        val details = reason?.let { ""","details":{"reason":"$it"}""" }.orEmpty()
        return """{"error":{"code":"$code","message":"x"$details}}"""
    }

    @Test
    fun createsTheLinkAndCopiesTheAppUrl() = runBlocking {
        server.enqueue(json(201, shareBody("tok1")))
        val entry = ShareLinkEntry(target, client(), IssuedShares())
        entry.model.load()

        val req = server.takeRequest()
        assertEquals("POST", req.method)
        assertEquals("/v1/apps/app1/share", req.path)
        assertEquals("Bearer dev-token", req.getHeader("Authorization"))

        val share = requireNotNull(entry.model.state.value.share)
        assertEquals("tailorit://import/tok1", ShareLink.url(share))
        assertNotNull("QR が描ける", ShareLink.qr(share))

        val clipboard = Clipboard("test")
        assertTrue(ShareLink.copy(share, clipboard))
        assertEquals("tailorit://import/tok1", clipboard.getData(DataFlavor.stringFlavor))
        assertFalse("クリップボードが無い(headless)", ShareLink.copy(share, null))
    }

    @Test
    fun reopeningUsesTheIssuedLinkWithoutANewToken() = runBlocking {
        server.enqueue(json(201, shareBody("tok1")))
        val issued = IssuedShares()
        ShareLinkEntry(target, client(), issued).model.load()
        val again = ShareLinkEntry(target, client(), issued)
        again.model.load()
        assertEquals("tok1", again.model.state.value.share?.token)
        assertEquals(1, server.requestCount)
    }

    @Test
    fun urlFallsBackToTheTokenWhenTheServerUrlIsOffShape() {
        val share = Share(token = "tok9", appId = "app1", url = "https://example.com/x")
        assertEquals("tailorit://import/tok9", ShareLink.url(share))
    }

    @Test
    fun revokeDeletesThenIssuesANewLink() = runBlocking {
        server.enqueue(json(201, shareBody("tok1")))
        server.enqueue(MockResponse().setResponseCode(204))
        server.enqueue(json(201, shareBody("tok2")))
        val entry = ShareLinkEntry(target, client(), IssuedShares())
        entry.model.load()
        entry.model.revoke()

        server.takeRequest()
        val delete = server.takeRequest()
        assertEquals("DELETE", delete.method)
        assertEquals("/v1/shares/tok1", delete.path)
        assertEquals("POST", server.takeRequest().method)
        assertEquals("tok2", entry.model.state.value.share?.token)
        assertEquals(ShareNotice.REVOKED, entry.model.state.value.notice)
    }

    @Test
    fun revokeFailureKeepsTheLink() = runBlocking {
        server.enqueue(json(201, shareBody("tok1")))
        server.enqueue(json(403, error("forbidden")))
        val entry = ShareLinkEntry(target, client(), IssuedShares())
        entry.model.load()
        entry.model.revoke()
        assertEquals("tok1", entry.model.state.value.share?.token)
        assertEquals(ShareNotice.REVOKE_FAILED, entry.model.state.value.notice)
    }

    @Test
    fun failuresFromTheServer() = runBlocking {
        server.enqueue(json(400, error("invalid_argument")))
        server.enqueue(json(403, error("forbidden", reason = "not_member")))
        val entry = ShareLinkEntry(target, client(), IssuedShares())
        entry.model.load()
        assertEquals(ShareFailure.BLANK_APP, entry.model.state.value.failure)
        assertNull(entry.model.state.value.share)
        entry.model.issue()
        assertEquals(ShareFailure.NOT_MEMBER, entry.model.state.value.failure)
    }

    @Test
    fun withoutAServerItIsUnavailable() = runBlocking {
        val entry = ShareLinkEntry(target, null, IssuedShares())
        entry.model.load()
        assertEquals(ShareFailure.UNAVAILABLE, entry.model.state.value.failure)
        assertEquals(0, server.requestCount)
    }

    @Test
    fun paletteCommandOpensTheDialog() {
        val entry = ShareLinkEntry(target, null, IssuedShares())
        val command = entry.command()
        assertEquals("このアプリをシェア", command.title)
        assertFalse(entry.open)
        command.run()
        assertTrue(entry.open)
    }

    @Test
    fun everyTextExistsInJapaneseAndEnglish() {
        val table = StringTable.fromClasspath()
        val keys = ShareFailure.entries.map(ShareLink::failureKey) + ShareNotice.entries.map(ShareLink::noticeKey) + listOf(
            "share_title", "share_untitled", "share_settings_row", "share_action", "share_copy", "share_close", "share_retry",
            "share_creating", "share_qr_label", "share_qr_caption", "share_revoke", "share_revoke_title", "share_revoke_message",
            "share_revoke_confirm", "share_revoke_cancel", "desktop_qr_copy_image",
        )
        for (lang in Lang.entries) for (key in keys) assertTrue("$lang $key", table.hasString(lang, key))
        assertEquals("「家計簿」をシェア", ShareLink.title("家計簿"))
        assertEquals("「このアプリ」をシェア", ShareLink.title(" "))
    }
}
