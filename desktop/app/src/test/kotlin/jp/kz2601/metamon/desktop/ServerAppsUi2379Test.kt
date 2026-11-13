package jp.kz2601.metamon.desktop

import jp.kz2601.metamon.desktop.i18n.Lang
import jp.kz2601.metamon.desktop.i18n.StringTable
import jp.kz2601.metamon.desktop.i18n.Strings
import jp.kz2601.metamon.desktop.net.ClientPlatform
import jp.kz2601.metamon.desktop.net.DesktopApi
import jp.kz2601.metamon.desktop.net.ServerApps
import jp.kz2601.metamon.services.AppSummary
import jp.kz2601.metamon.services.http.MetamonHttp
import jp.kz2601.metamon.services.http.RetryPolicy
import jp.kz2601.metamon.services.live.HttpAppConfigClient
import jp.kz2601.metamon.services.local.DocumentCache
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

/** サーバーのアプリをホームとパレットに出し、選んだら DSL を読んで開く(#2379)。通信は MockWebServer だけ。 */
class ServerAppsUi2379Test {
    private lateinit var server: MockWebServer
    private val cache = DocumentCache(null)

    @Before
    fun setUp() {
        server = MockWebServer().apply { start() }
    }

    @After
    fun tearDown() {
        runCatching { server.shutdown() }
    }

    private fun client() = HttpAppConfigClient(
        MetamonHttp(
            baseUrl = server.url("/").toString(),
            tokens = DesktopApi.devTokens("dev-token"),
            clientInfo = DesktopApi.clientInfo(),
            client = DesktopApi.okHttp(ClientPlatform.LINUX, "ua"),
            retry = RetryPolicy(idempotentAttempts = 1),
        ),
        cache,
    )

    private fun json(body: String, status: Int = 200) = MockResponse().setResponseCode(status).setHeader("Content-Type", "application/json").setBody(body)

    private val kakeibo = Examples.load("kakeibo").app.name
    private val dsl = """{"appId":"a1","version":3,"dsl":${ServerAppsUi2379Test::class.java.classLoader.getResource("examples/kakeibo.json")!!.readText()}}"""

    @Test
    fun rowsKeepTheOrderAndNameTheUntitled() = runBlocking {
        val apps = listOf(AppSummary("b", "買い物"), AppSummary("a", "  "), AppSummary("c", "日記"))
        val rows = Strings.withLang(Lang.JA) { runBlocking { ServerAppPalette.rows(apps) { if (it == "c") "book" else null } } }
        assertEquals(listOf("b", "a", "c"), rows.map { it.id })
        assertEquals(listOf("買い物", "(無題)", "日記"), rows.map { it.name })
        assertEquals(listOf(null, null, "book"), rows.map { it.icon })
    }

    @Test
    fun paletteCommandsPerApp() = Strings.withLang(Lang.JA) {
        val opened = mutableListOf<String>()
        val rows = listOf(ServerAppRow("b", "買い物", null), ServerAppRow("c", "日記", "book"))
        val commands = ServerAppPalette.commands(rows, fromCache = false) { opened += it.id }
        assertEquals(listOf("server:b", "server:c"), commands.map { it.id })
        assertEquals(listOf("買い物", "日記"), commands.map { it.title })
        assertTrue(commands.all { it.kind == PaletteCommand.Kind.APP && it.hint == null })
        commands[1].run()
        assertEquals(listOf("c"), opened)
        // 写しなら「前回の一覧」を添える
        assertEquals(listOf("前回の一覧", "前回の一覧"), ServerAppPalette.commands(rows, fromCache = true) {}.map { it.hint })
        // id でも探せる
        assertEquals("server:c", CommandRanking.rank(commands, "c").first().id)
    }

    @Test
    fun paletteListsServerAppsAfterBundled() {
        val s = DesktopSession(Examples.load("shopping"), MockStore())
        val k = DesktopKeyboard(s.dsl, s, HostOs.WINDOWS)
        k.openApp = {}
        k.serverApps = { ServerAppPalette.commands(listOf(ServerAppRow("x", "サーバーのアプリ", null)), false) {} }
        val ids = k.commands().map { it.id }
        val server = ids.indexOf("server:x")
        assertTrue(server > ids.indexOfLast { it.startsWith("app:") })
    }

    @Test
    fun modelLoadsRowsWithIconsAndMarksTheCopy() = runBlocking {
        server.enqueue(json("""{"items":[{"id":"a1","name":"家計簿","currentVersion":3}]}"""))
        val model = ServerAppsModel(ServerApps(client(), cache) { true }) { if (it == "a1") "wallet" else null }
        assertTrue(model.available)
        model.refresh()
        assertFalse(model.loading)
        assertEquals(listOf(ServerAppRow("a1", "家計簿", "wallet")), model.rows)
        assertFalse(model.fromCache)
        server.enqueue(json("", 500))
        model.refresh()
        assertTrue(model.fromCache)
        assertEquals(1, model.rows.size)
    }

    @Test
    fun unavailableWithoutSource() = runBlocking {
        val model = ServerAppsModel(null)
        assertFalse(model.available)
        model.refresh()
        assertNull(model.result)
        assertEquals(emptyList<ServerAppRow>(), model.rows)
    }

    @Test
    fun openerReadsTheDslAndFallsBackToTheCopy() = runBlocking {
        server.enqueue(json(dsl))
        val opener = ServerAppOpener(client(), cache)
        val first = opener.load("a1")
        assertEquals(kakeibo, first?.app?.name)
        assertEquals("/v1/apps/a1/dsl", server.takeRequest().path)
        server.enqueue(json("", 502))
        assertEquals("圏外・5xx は写し", kakeibo, opener.load("a1")?.app?.name)
        server.enqueue(json("""{"error":{"code":"not_found","message":"x"}}""", 404))
        assertNull(opener.load("a1"))
        server.enqueue(json("", 500))
        assertNull("写しも無い", opener.load("zz"))
    }

    @Test
    fun hubOpensTheServerDslInItsWindow() {
        val hub = AppHub(load = { error("同梱には無い: $it") }, clearTempFiles = {})
        val serverDsl = Examples.load("kakeibo")
        hub.put("a1", serverDsl)
        assertTrue(hub.dsl("a1") === serverDsl)
        val registry = WindowRegistry()
        val w = registry.open(WindowKey.app("a1"))
        assertTrue("開いていれば前に出す", registry.open(WindowKey.app("a1")) === w)
        assertNotNull(hub.acquire("a1"))
    }

    @Test
    fun homeWindowIsNotAnApp() {
        val registry = WindowRegistry()
        registry.open(WindowKey.home())
        assertTrue(registry.active!!.key.isHome)
        assertNull("ホームにすぐ記録は入れない", registry.quickRecordWindow())
        assertFalse(WindowKey.app("shopping").isHome)
    }

    @Test
    fun textsInBothLanguages() {
        val table = StringTable.fromClasspath()
        val keys = listOf("desktop_server_apps_cached", "desktop_server_app_open_failed", "settings_usage_apps", "switcher_empty", "desktop_untitled")
        for (lang in Lang.entries) for (key in keys) assertTrue("$lang $key", table.hasString(lang, key))
    }
}
