package jp.kz2601.metamon.services.update

import jp.kz2601.metamon.services.consent.InMemoryKeyValueStore
import jp.kz2601.metamon.services.http.RetryPolicy
import jp.kz2601.metamon.services.http.TokenProvider
import jp.kz2601.metamon.services.http.json
import jp.kz2601.metamon.services.http.metamonHttp
import jp.kz2601.metamon.services.http.take
import kotlinx.coroutines.runBlocking
import okhttp3.mockwebserver.MockWebServer
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test

class UpdateCheckerTest {
    private lateinit var server: MockWebServer

    @Before
    fun setUp() {
        server = MockWebServer().apply { start() }
    }

    @After
    fun tearDown() {
        server.shutdown()
    }

    private fun checker(build: String = "7"): UpdateChecker {
        val http = server.metamonHttp(tokens = TokenProvider { null }, retry = RetryPolicy(idempotentAttempts = 1))
        return UpdateChecker(HttpClientConfigClient(http), build)
    }

    // --- 版の比べ方 ---

    @Test
    fun buildNumbersCompareNumericallyPartByPart() {
        fun b(raw: String) = BuildNumber.parse(raw)!!
        assertTrue(b("9") < b("10"))
        assertTrue(b("1.2") < b("1.10"))
        assertEquals(b("1.2"), b("1.2.0"))
        assertTrue(b("12") > b("11.99"))
        assertNull(BuildNumber.parse(""))
        assertNull(BuildNumber.parse("1..2"))
        assertNull(BuildNumber.parse("1.2-beta"))
        assertNull(BuildNumber.parse("-1"))
    }

    @Test
    fun belowMinimumIsRequiredAndBelowLatestIsRecommended() {
        val config = ClientConfig(minAndroidBuild = 10, latestAndroidBuild = 15)
        assertEquals(UpdateRequirement.Required(10), UpdateRequirement.evaluate(config, "9"))
        assertEquals(UpdateRequirement.Recommended(15), UpdateRequirement.evaluate(config, "10"))
        assertEquals(UpdateRequirement.None, UpdateRequirement.evaluate(config, "15"))
        // 自分の版が読めない時は止めない
        assertEquals(UpdateRequirement.None, UpdateRequirement.evaluate(config, "dev"))
        assertEquals(UpdateRequirement.None, UpdateRequirement.evaluate(null, "1"))
    }

    // --- Android の値が無い時 ---

    @Test
    fun iosOnlyValuesDoNotStopAndroid() = runBlocking {
        server.enqueue(json(200, """{"minIOSBuild":9999,"latestIOSBuild":9999,"iosAppStoreUrl":"https://apps.apple.com/app/id1"}"""))
        val result = checker(build = "1").check()
        assertEquals(UpdateRequirement.None, result.requirement)
        assertNull(result.storeUrl)
        val req = server.take()
        assertEquals("GET", req.method)
        assertEquals("/v1/client-config", req.path)
        assertNull(req.getHeader("Authorization"))
    }

    @Test
    fun androidValuesAreUsedWhenPresent() = runBlocking {
        server.enqueue(json(200, """{"minAndroidBuild":8,"androidStoreUrl":"https://play.google.com/store/apps/details?id=x"}"""))
        val result = checker(build = "7").check()
        assertEquals(UpdateRequirement.Required(8), result.requirement)
        assertEquals("https://play.google.com/store/apps/details?id=x", result.storeUrl)
    }

    @Test
    fun failuresAndBrokenBodiesFailOpen() = runBlocking {
        server.enqueue(json(503, """{"error":{"code":"unavailable","message":"x"}}"""))
        assertEquals(UpdateChecker.Result(UpdateRequirement.None), checker().check())
        server.enqueue(json(200, "not json"))
        assertEquals(UpdateChecker.Result(UpdateRequirement.None), checker().check())
        assertEquals(UpdateChecker.Result(UpdateRequirement.None), UpdateChecker(client = null, build = "1").check())
    }

    @Test
    fun storeUrlMustBeHttpsOrMarket() = runBlocking {
        server.enqueue(json(200, """{"androidStoreUrl":"javascript:alert(1)"}"""))
        assertNull(checker().check().storeUrl)
        assertTrue(UpdateChecker.isStoreUrl("market://details?id=jp.kz2601.metamon"))
        assertFalse(UpdateChecker.isStoreUrl("http://play.google.com"))
    }

    // --- お知らせ ---

    @Test
    fun noticeIsDecodedAndBrokenNoticeKeepsTheRest() = runBlocking {
        server.enqueue(
            json(
                200,
                """{"minAndroidBuild":8,"notice":{"id":"2026-11-02-ai","severity":"warning","message":{"ja":"止めています","en":"Paused"},"linkUrl":"https://example.com/status"}}""",
            ),
        )
        val notice = checker(build = "9").check().notice!!
        assertEquals("2026-11-02-ai", notice.id)
        assertEquals(ServiceNotice.Severity.WARNING, notice.severity)
        assertEquals("Paused", notice.text("en"))
        assertEquals("止めています", notice.text("ja"))
        assertEquals("https://example.com/status", notice.link)

        server.enqueue(json(200, """{"minAndroidBuild":8,"notice":{"id":"x"}}"""))
        val broken = checker(build = "7").check()
        assertNull(broken.notice)
        assertEquals(UpdateRequirement.Required(8), broken.requirement)
    }

    @Test
    fun noticeFallsBackAndFiltersLinksAndIosOnlyNotices() = runBlocking {
        val notice = ServiceNotice(id = "a", severity = ServiceNotice.Severity.INFO, ja = " 日本語 ", en = "  ", linkUrl = "http://example.com")
        assertEquals("日本語", notice.text("en"))
        assertNull(notice.link)
        // 知らない severity は info
        server.enqueue(json(200, """{"notice":{"id":"b","severity":"critical","message":{"ja":"x"}}}"""))
        assertEquals(ServiceNotice.Severity.INFO, checker().check().notice!!.severity)
        // iOS の版に絞ったお知らせは Android に出さない
        server.enqueue(json(200, """{"notice":{"id":"c","message":{"ja":"x"},"maxIOSBuild":12}}"""))
        assertNull(checker().check().notice)
    }

    // --- 閉じた印 ---

    @Test
    fun dismissalsAreRememberedAndCapped() {
        val store = InMemoryKeyValueStore()
        val dismissals = NoticeDismissals(store)
        assertFalse(dismissals.isDismissed("a"))
        dismissals.dismiss("a")
        assertTrue(NoticeDismissals(store).isDismissed("a"))
        repeat(NoticeDismissals.LIMIT) { dismissals.dismiss("n$it") }
        assertFalse(dismissals.isDismissed("a"))
        assertEquals(NoticeDismissals.LIMIT, dismissals.ids.size)
        // もう一度閉じたものは新しい側に移る
        dismissals.dismiss("n0")
        assertEquals("n0", dismissals.ids.last())
        assertEquals(NoticeDismissals.LIMIT, dismissals.ids.size)
    }
}
