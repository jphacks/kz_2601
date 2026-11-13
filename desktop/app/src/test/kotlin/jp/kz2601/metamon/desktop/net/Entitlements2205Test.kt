package jp.kz2601.metamon.desktop.net

import jp.kz2601.metamon.desktop.auth.AccountPlanText
import jp.kz2601.metamon.desktop.i18n.Lang
import jp.kz2601.metamon.desktop.i18n.Strings
import jp.kz2601.metamon.services.MyUsage
import jp.kz2601.metamon.services.UsageMeter
import jp.kz2601.metamon.services.consent.InMemoryKeyValueStore
import jp.kz2601.metamon.services.http.MetamonHttp
import jp.kz2601.metamon.services.http.RetryPolicy
import kotlinx.coroutines.runBlocking
import okhttp3.mockwebserver.Dispatcher
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import okhttp3.mockwebserver.RecordedRequest
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test
import java.time.Instant
import java.time.ZoneId

/** #2205: `GET /v1/me/entitlements` を読んで設定に出す(MockWebServer だけ・本物の API は呼ばない)。 */
class Entitlements2205Test {
    private lateinit var server: MockWebServer
    private val now = Instant.parse("2026-11-13T00:00:00Z")

    /** パスごとの応答(null = 500)。 */
    private var entitlements: String? = null
    private var usage: String? = """{"plan":"free","apps":{"used":2,"limit":5}}"""

    @Before
    fun setUp() {
        server = MockWebServer().apply {
            dispatcher = object : Dispatcher() {
                override fun dispatch(request: RecordedRequest): MockResponse {
                    val body = when (request.requestUrl!!.encodedPath) {
                        "/v1/me/entitlements" -> entitlements
                        "/v1/me/usage" -> usage
                        else -> null
                    }
                    return body?.let { MockResponse().setHeader("Content-Type", "application/json").setBody(it) } ?: MockResponse().setResponseCode(500)
                }
            }
            start()
        }
    }

    @After
    fun tearDown() {
        server.shutdown()
    }

    private val store = InMemoryKeyValueStore()

    private fun loader(): AccountPlanLoader {
        val http = MetamonHttp(
            baseUrl = server.url("/").toString(),
            tokens = DesktopApi.devTokens("id-token"),
            clientInfo = DesktopApi.clientInfo(),
            client = DesktopApi.okHttp(),
            retry = RetryPolicy(idempotentAttempts = 1),
            sleep = {},
        )
        return AccountPlanLoader(HttpAccountPlanApi(http), store) { now }
    }

    private fun pro(expiresAt: String, platform: String? = null, isPro: Boolean = true, extra: String = "") =
        """{"isPro":$isPro,"pro":{"productId":"jp.kz2601.metamon.pro.monthly","originalTransactionId":"o","transactionId":"t","environment":"Production",""" +
            """"expiresAt":"$expiresAt","updatedAt":"2026-11-01T00:00:00Z"${platform?.let { ",\"platform\":\"$it\"" }.orEmpty()}$extra}}"""

    @Test
    fun pro() = runBlocking {
        entitlements = pro("2026-12-01T00:00:00.123Z", platform = "googlePlay")
        val plan = loader().load("uid-1")
        assertEquals(AccountPlan.Kind.PRO, plan.kind)
        assertEquals(Instant.parse("2026-12-01T00:00:00.123Z"), plan.until)
        assertEquals(AccountPlan.PurchaseStore.GOOGLE_PLAY, plan.store)
        assertFalse(plan.cached)
        assertEquals(UsageMeter(used = 2, limit = 5), plan.usage?.apps)
        val auth = server.takeRequest().getHeader("Authorization")
        assertEquals("Bearer id-token", auth)
        // 文: 期限と買った所(値段・上限はサーバーの物だけ)
        assertEquals("Pro", AccountPlanText.value(plan, Lang.JA))
        assertEquals(listOf("2026年12月1日 まで", "Google Play で購入"), AccountPlanText.details(plan, Lang.JA, ZoneId.of("Asia/Tokyo")))
        assertEquals(listOf("Until Dec 1, 2026", "Purchased on Google Play"), AccountPlanText.details(plan, Lang.EN, ZoneId.of("Asia/Tokyo")))
        assertEquals("2 / 5", AccountPlanText.apps(plan, Lang.JA))
    }

    @Test
    fun appStoreWhenNoPlatformAndGraceExtendsUntil() = runBlocking {
        entitlements = pro("2026-11-10T00:00:00Z", extra = ""","graceExpiresAt":"2026-11-20T00:00:00Z"""")
        val plan = loader().load("uid-1")
        assertEquals(AccountPlan.PurchaseStore.APP_STORE, plan.store)
        assertEquals("猶予の終わりまで Pro", Instant.parse("2026-11-20T00:00:00Z"), plan.until)
    }

    @Test
    fun free() = runBlocking {
        entitlements = """{"isPro":false,"pro":null}"""
        usage = """{"plan":"free","apps":{"used":7}}"""
        val plan = loader().load("uid-1")
        assertEquals(AccountPlan(AccountPlan.Kind.FREE, usage = MyUsage(plan = "free", apps = UsageMeter(used = 7))), plan)
        assertEquals(Strings.table.get(Lang.JA, "settings_pro_free"), AccountPlanText.value(plan, Lang.JA))
        assertTrue(AccountPlanText.details(plan, Lang.JA).isEmpty())
        assertEquals("上限が無ければ上限なし", Strings.table.format(Lang.JA, "settings_usage_no_limit", 7), AccountPlanText.apps(plan, Lang.JA))
    }

    @Test
    fun expired() = runBlocking {
        entitlements = pro("2026-11-01T00:00:00Z", isPro = false)
        val plan = loader().load("uid-1")
        assertEquals(AccountPlan.Kind.FREE, plan.kind)
        assertTrue(plan.expired)
        assertEquals(listOf("Pro の期限が切れました(2026年11月1日)"), AccountPlanText.details(plan, Lang.JA, ZoneId.of("Asia/Tokyo")))
    }

    @Test
    fun unreadableUsesCachedProUntilItExpires() = runBlocking {
        entitlements = pro("2026-12-01T00:00:00Z")
        loader().load("uid-1")
        entitlements = null
        usage = null
        val plan = loader().load("uid-1")
        assertEquals(AccountPlan.Kind.PRO, plan.kind)
        assertTrue(plan.cached)
        assertNull("量も読めなければ出さない", AccountPlanText.apps(plan))
        assertEquals(Strings.table.get(Lang.JA, "desktop_plan_cached"), AccountPlanText.details(plan, Lang.JA).last())
        assertEquals("別の人の値は使わない", AccountPlan.Kind.UNKNOWN, loader().load("uid-2").kind)
    }

    @Test
    fun unreadableIgnoresExpiredCacheAndBrokenBodies() = runBlocking {
        entitlements = pro("2026-11-12T00:00:00Z")
        // サーバーはまだ Pro と言った(時計のずれ)が、残した値の期限は過ぎている
        loader().load("uid-1")
        entitlements = null
        val plan = loader().load("uid-1")
        assertEquals(AccountPlan.Kind.UNKNOWN, plan.kind)
        assertEquals(Strings.table.get(Lang.JA, "desktop_plan_unknown"), AccountPlanText.value(plan, Lang.JA))

        entitlements = """{"not":"json-we-know"}"""
        assertEquals("読めない本文は残さない", AccountPlan.Kind.UNKNOWN, loader().load("uid-1").kind)
        entitlements = """{"isPro":false,"pro":null}"""
        loader().load("uid-1")
        entitlements = null
        assertEquals("無料は前回の値で出す", AccountPlan(AccountPlan.Kind.FREE, cached = true, usage = plan.usage), loader().load("uid-1"))
        loader().forget()
        assertEquals(AccountPlan.Kind.UNKNOWN, loader().load("uid-1").kind)
    }

    @Test
    fun revokedShowsRevokedTime() {
        val e = Entitlements.decode(pro("2026-12-01T00:00:00Z", isPro = false, extra = ""","revokedAt":"2026-11-05T00:00:00Z""""))
        val plan = AccountPlan.of(e)
        assertTrue(plan.expired)
        assertEquals(Instant.parse("2026-11-05T00:00:00Z"), plan.until)
    }
}
