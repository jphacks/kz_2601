package jp.kz2601.metamon.services.sharing

import jp.kz2601.metamon.services.consent.InMemoryKeyValueStore
import jp.kz2601.metamon.services.http.errorBody
import jp.kz2601.metamon.services.http.json
import jp.kz2601.metamon.services.http.metamonHttp
import jp.kz2601.metamon.services.http.take
import jp.kz2601.metamon.services.live.HttpAppConfigClient
import jp.kz2601.metamon.services.social.AuthorProfile
import kotlinx.coroutines.runBlocking
import okhttp3.mockwebserver.MockWebServer
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test

class ShareLinkOpenerTest {
    private lateinit var server: MockWebServer
    private lateinit var client: HttpAppConfigClient
    private val profile = AuthorProfile(InMemoryKeyValueStore())

    @Before
    fun setUp() {
        server = MockWebServer().apply { start() }
        client = HttpAppConfigClient(server.metamonHttp())
    }

    @After
    fun tearDown() {
        server.shutdown()
    }

    private fun opener() = ShareLinkOpener(client, profile)

    /** 型が違ったら中身ごと出す(ClassCastException だと何が返ったか分からない)。 */
    private inline fun <reified T : ShareOpenResult> ShareOpenResult.expect(): T =
        this as? T ?: throw AssertionError("expected ${T::class.simpleName} but was $this")

    private fun previewJson(sharedData: Boolean, mine: Boolean = false) =
        """{"token":"tok","name":"家族の買い物","description":"","components":["list"],"sharedData":$sharedData,"mine":$mine}"""

    private fun installJson(member: Boolean = false, joined: Boolean = false) =
        """{"id":"app9","name":"家族の買い物","currentVersion":3,"status":"active","dsl":{},"created":true,"joined":$joined,"member":$member}"""

    // MARK: URL の読み方

    @Test
    fun readsTokenOnlyFromImportLinks() {
        assertEquals("abc_1-2.x:y", ShareLinks.token("tailorit://import/abc_1-2.x:y"))
        assertEquals("abc", ShareLinks.token("TailorIt://IMPORT/abc"))
        assertEquals("abc", ShareLinks.token("tailorit://import/abc?utm=1"))
        assertNull(ShareLinks.token("tailorit://import/"))
        assertNull(ShareLinks.token("tailorit://other/abc"))
        assertNull(ShareLinks.token("https://example.com/s/abc"))
        assertNull(ShareLinks.token("tailorit://import/a%20b"))
        assertNull(ShareLinks.token("tailorit://import/" + "a".repeat(129)))
    }

    // MARK: サインイン前の保留

    @Test
    fun inboxKeepsLinkUntilReadyAndHandsItOverOnce() {
        val inbox = ShareLinkInbox()
        assertTrue(inbox.receive("tailorit://import/tok"))
        assertNull(inbox.take(ready = false))
        assertEquals("tok", inbox.pending.value)
        assertEquals("tok", inbox.take(ready = true))
        assertNull(inbox.take(ready = true))
    }

    @Test
    fun clearDropsTheHeldLink() {
        val inbox = ShareLinkInbox()
        inbox.receive("tailorit://import/of-a")
        inbox.clear()
        assertNull(inbox.pending.value)
        assertNull(inbox.take(ready = true))
    }

    @Test
    fun inboxIgnoresOtherUrlsAndKeepsTheLatestLink() {
        val inbox = ShareLinkInbox()
        assertFalse(inbox.receive(null))
        assertFalse(inbox.receive("https://example.com/s/tok"))
        assertNull(inbox.pending.value)
        inbox.receive("tailorit://import/first")
        assertFalse(inbox.receive("mailto:a@example.com"))
        inbox.receive("tailorit://import/second")
        assertEquals("second", inbox.take(ready = true))
    }

    @Test
    fun readyOnlyAfterSignInBootstrapAndConsent() {
        assertFalse(ShareLinkInbox.ready(signedIn = false, bootstrapped = true, consentShowing = false, consentPending = false))
        assertFalse(ShareLinkInbox.ready(signedIn = true, bootstrapped = false, consentShowing = false, consentPending = false))
        assertFalse(ShareLinkInbox.ready(signedIn = true, bootstrapped = true, consentShowing = true, consentPending = false))
        assertFalse(ShareLinkInbox.ready(signedIn = true, bootstrapped = true, consentShowing = false, consentPending = true))
        assertTrue(ShareLinkInbox.ready(signedIn = true, bootstrapped = true, consentShowing = false, consentPending = false))
    }

    // MARK: 参加と取り込みの分け方

    @Test
    fun sharedDataAppAsksToJoinWithoutImporting() = runBlocking<Unit> {
        server.enqueue(json(200, previewJson(sharedData = true)))
        val result = opener().open("tok")
        val join = result.expect<ShareOpenResult.AskToJoin>().join
        assertEquals("tok", join.token)
        assertEquals("家族の買い物", join.preview.name)
        val req = server.take()
        assertEquals("GET", req.method)
        assertEquals("/v1/shares/tok", req.path)
        assertEquals(1, server.requestCount)
    }

    @Test
    fun copyAppAsksToAddBeforeImporting() = runBlocking<Unit> {
        server.enqueue(json(200, previewJson(sharedData = false)))
        val add = opener().open("tok").expect<ShareOpenResult.AskToAdd>().add
        assertEquals("tok", add.token)
        assertEquals("家族の買い物", add.preview?.name)
        assertEquals("/v1/shares/tok", server.take().path)
        assertEquals("確認の前に取り込まない", 1, server.requestCount)
    }

    @Test
    fun addImportsAfterConfirm() = runBlocking<Unit> {
        server.enqueue(json(200, previewJson(sharedData = false)))
        server.enqueue(json(201, installJson()))
        val o = opener()
        val add = o.open("tok").expect<ShareOpenResult.AskToAdd>().add
        val result = o.add(add).expect<ShareOpenResult.Added>()
        assertEquals("app9", result.app.id)
        assertFalse(result.viaJoin)
        server.take()
        val req = server.take()
        assertEquals("POST", req.method)
        assertEquals("/v1/shares/tok/import", req.path)
        assertEquals("{}", req.body.readUtf8())
    }

    @Test
    fun ownSharedAppAsksToAddNotToJoin() = runBlocking<Unit> {
        server.enqueue(json(200, previewJson(sharedData = true, mine = true)))
        server.enqueue(json(200, installJson(joined = true)))
        val o = opener()
        val add = o.open("tok").expect<ShareOpenResult.AskToAdd>().add
        assertEquals(1, server.requestCount)
        assertTrue(o.add(add) is ShareOpenResult.Added)
    }

    @Test
    fun previewFailureStillAsksToAddWithoutName() = runBlocking<Unit> {
        server.enqueue(json(500, errorBody("internal")))
        server.enqueue(json(201, installJson()))
        val o = opener()
        val add = o.open("tok").expect<ShareOpenResult.AskToAdd>().add
        assertEquals(PendingAdd("tok", null), add)
        assertEquals(1, server.requestCount)
        assertTrue(o.add(add) is ShareOpenResult.Added)
    }

    @Test
    fun addFailureReturnsReason() = runBlocking<Unit> {
        server.enqueue(json(404, errorBody("not_found")))
        assertEquals(ShareOpenResult.Failed(ShareOpenFailure.REVOKED), opener().add(PendingAdd("tok", null)))
        assertEquals(ShareOpenResult.Failed(ShareOpenFailure.UNAVAILABLE), ShareLinkOpener(null, profile).add(PendingAdd("tok", null)))
    }

    @Test
    fun joinSendsDisplayNameAndRemembersIt() = runBlocking<Unit> {
        server.enqueue(json(200, previewJson(sharedData = true)))
        server.enqueue(json(201, installJson(member = true)))
        val o = opener()
        val join = o.open("tok").expect<ShareOpenResult.AskToJoin>().join
        val result = o.join(join, "  たろう  ").expect<ShareOpenResult.Added>()
        assertTrue(result.viaJoin)
        assertTrue(result.app.member)
        assertEquals("/v1/shares/tok", server.take().path)
        val importReq = server.take()
        assertEquals("/v1/shares/tok/import", importReq.path)
        assertEquals("""{"displayName":"たろう"}""", importReq.body.readUtf8())
        assertEquals("たろう", profile.name)
        assertEquals("たろう", o.initialDisplayName())
    }

    @Test
    fun blankNameJoinsWithoutNameAndKeepsProfile() = runBlocking<Unit> {
        profile.name = "父"
        server.enqueue(json(201, installJson(member = true)))
        val join = PendingJoin("tok", jp.kz2601.metamon.services.SharePreview(token = "tok", sharedData = true))
        assertTrue(opener().join(join, "   ") is ShareOpenResult.Added)
        assertEquals("{}", server.take().body.readUtf8())
        assertEquals("父", profile.name)
    }

    // MARK: 失敗

    @Test
    fun failuresMapToTheirOwnReasons() = runBlocking<Unit> {
        val join = PendingJoin("tok", jp.kz2601.metamon.services.SharePreview(token = "tok", sharedData = true))
        val cases = listOf(
            json(404, errorBody("not_found")) to ShareOpenFailure.REVOKED,
            json(403, errorBody("forbidden", details = """{"reason":"inviter_left"}""")) to ShareOpenFailure.INVITER_LEFT,
            json(403, errorBody("forbidden", details = """{"reason":"members_full"}""")) to ShareOpenFailure.MEMBERS_FULL,
            json(422, errorBody("text_not_allowed")) to ShareOpenFailure.NAME_REJECTED,
            json(500, errorBody("internal")) to ShareOpenFailure.OTHER,
        )
        for ((response, expected) in cases) {
            server.enqueue(response)
            assertEquals(ShareOpenResult.Failed(expected), opener().join(join, "たろう"))
        }
    }

    @Test
    fun offlineAndUnavailable() = runBlocking<Unit> {
        assertEquals(ShareOpenResult.Failed(ShareOpenFailure.UNAVAILABLE), ShareLinkOpener(null, profile).open("tok"))
        server.shutdown()
        val add = opener().open("tok").expect<ShareOpenResult.AskToAdd>().add
        assertEquals(ShareOpenResult.Failed(ShareOpenFailure.OFFLINE), opener().add(add))
    }
}
