package jp.kz2601.metamon.desktop.apps

import jp.kz2601.metamon.desktop.i18n.Lang
import jp.kz2601.metamon.desktop.i18n.StringTable
import jp.kz2601.metamon.desktop.i18n.Strings
import jp.kz2601.metamon.desktop.net.ClientPlatform
import jp.kz2601.metamon.desktop.net.DesktopApi
import kotlinx.coroutines.runBlocking
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test

/** アプリの一覧の印(#2504): 応答 → 印・読み上げ・文言(通信は MockWebServer だけ)。 */
class AppBadgesTest {
    private lateinit var server: MockWebServer

    @Before
    fun setUp() {
        server = MockWebServer().apply { start() }
    }

    @After
    fun tearDown() {
        server.shutdown()
    }

    private fun http() = DesktopApi.http(
        baseUrl = server.url("/").toString(),
        tokens = DesktopApi.devTokens("dev-token"),
        platform = ClientPlatform.LINUX,
        client = DesktopApi.okHttp(ClientPlatform.LINUX, "ua"),
    )

    private val requests = """
        {"items":[
          {"id":"rr_1","appId":"a1","name":"家計簿","dslVersion":3,"purpose":"x","status":"reviewing"},
          {"id":"rr_2","appId":"a2","name":"日記","dslVersion":1,"purpose":"x","status":"declined"},
          {"id":"rr_3","appId":"a3","name":"体重","dslVersion":1,"purpose":"x","status":"declined"},
          {"id":"rr_4","appId":"a3","name":"体重","dslVersion":2,"purpose":"x","status":"received"},
          {"id":"rr_5","appId":"a4","name":"本","dslVersion":2,"purpose":"x","status":"typo"}
        ]}
    """.trimIndent()

    @Test
    fun requestedAppsSkipDeclinedOnly() {
        assertEquals(setOf("a1", "a3", "a4"), AppBadges.requestedApps(requests))
    }

    @Test
    fun requestedAppsFromOddResponses() {
        assertEquals(emptySet<String>(), AppBadges.requestedApps("""{"items":[]}"""))
        assertEquals("items が無い = 申請が無い", emptySet<String>(), AppBadges.requestedApps("{}"))
        assertEquals(emptySet<String>(), AppBadges.requestedApps("""{"items":[{"status":"received"},{"appId":"","status":"received"},1]}"""))
        assertNull(AppBadges.requestedApps("""{"items":"x"}"""))
        assertNull(AppBadges.requestedApps("not json"))
        assertNull(AppBadges.requestedApps("[]"))
    }

    @Test
    fun badgesInFixedOrderAndUnknownIsNotShown() {
        val source = AppBadgeSource(published = setOf("a1"), shared = setOf("a1", "a2"), requested = setOf("a1"))
        assertEquals(listOf(AppBadge.PUBLISHED, AppBadge.SHARED, AppBadge.REQUESTED), AppBadges.of("a1", source))
        assertEquals(listOf(AppBadge.SHARED), AppBadges.of("a2", source))
        assertEquals(emptyList<AppBadge>(), AppBadges.of("a9", source))
        // 応答に無い(null)物は出さない
        assertEquals(emptyList<AppBadge>(), AppBadges.of("a1", AppBadgeSource()))
        assertEquals(listOf(AppBadge.REQUESTED), AppBadges.of("a1", AppBadgeSource(requested = AppBadges.requestedApps(requests))))
    }

    @Test
    fun loadsRequestsFromTheServer() = runBlocking {
        server.enqueue(MockResponse().setHeader("Content-Type", "application/json").setBody(requests))
        assertEquals(setOf("a1", "a3", "a4"), AppBadges.loadRequested(http()))
        val req = server.takeRequest()
        assertEquals("GET", req.method)
        assertEquals("/v1/release-requests", req.path)
        assertEquals("Bearer dev-token", req.getHeader("Authorization"))
    }

    @Test
    fun serverFailureShowsNoBadge() = runBlocking {
        server.enqueue(MockResponse().setResponseCode(404).setHeader("Content-Type", "application/json").setBody("""{"error":{"code":"not_found","message":"x"}}"""))
        assertNull(AppBadges.loadRequested(http()))
    }

    @Test
    fun a11yReadsTheNameThenTheBadges() {
        val both = listOf(AppBadge.PUBLISHED, AppBadge.REQUESTED)
        assertEquals("家計簿", AppBadges.a11y("家計簿", emptyList()))
        assertEquals("家計簿(公開中・申請中)", Strings.withLang(Lang.JA) { AppBadges.a11y("家計簿", both) })
        assertEquals("Budget (Published, Release requested)", Strings.withLang(Lang.EN) { AppBadges.a11y("Budget", both) })
    }

    @Test
    fun everyTextExistsInJapaneseAndEnglish() {
        val table = StringTable.fromClasspath()
        val keys = AppBadge.entries.map { it.key } + listOf("desktop_app_badges_a11y", "desktop_app_badges_separator")
        for (lang in Lang.entries) for (key in keys) assertTrue("$lang $key", table.hasString(lang, key))
    }
}
