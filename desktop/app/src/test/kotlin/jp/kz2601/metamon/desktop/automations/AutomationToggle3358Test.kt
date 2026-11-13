package jp.kz2601.metamon.desktop.automations

import jp.kz2601.metamon.desktop.i18n.Lang
import jp.kz2601.metamon.desktop.i18n.StringTable
import jp.kz2601.metamon.desktop.net.ClientPlatform
import jp.kz2601.metamon.desktop.net.DesktopApi
import jp.kz2601.metamon.services.automations.AppAutomations
import jp.kz2601.metamon.services.automations.AutomationRule
import jp.kz2601.metamon.services.automations.AutomationsClient
import jp.kz2601.metamon.services.live.HttpAutomationsClient
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.async
import kotlinx.coroutines.runBlocking
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test

/** 自動の動きのルールのオン / オフ(#3358): 先に見た目を変え、失敗したら戻す。通信は MockWebServer だけ。 */
class AutomationToggle3358Test {
    private lateinit var server: MockWebServer

    @Before
    fun setUp() {
        server = MockWebServer().apply { start() }
    }

    @After
    fun tearDown() {
        server.shutdown()
    }

    private fun model(): AutomationsModel {
        val http = DesktopApi.http(
            baseUrl = server.url("/").toString(),
            tokens = DesktopApi.devTokens("dev-token"),
            platform = ClientPlatform.LINUX,
            client = DesktopApi.okHttp(ClientPlatform.LINUX, "ua"),
        )
        return AutomationsModel("a1", HttpAutomationsClient(http))
    }

    private fun json(body: String, status: Int = 200) =
        MockResponse().setResponseCode(status).setHeader("Content-Type", "application/json").setBody(body)

    private val list = """{"appId":"a1","rules":[{"id":"r1","description":"毎朝まとめる","trigger":"schedule","enabled":true},{"id":"r2","description":"古い","trigger":"record"}]}"""
    private val dsl = """{"version":7,"dsl":{"serverRules":[{"id":"r1"},{"id":"r2"}]}}"""

    private fun rules(m: AutomationsModel) = (m.phase.value as AutomationsModel.Phase.Loaded).automations.rules

    @Test
    fun turningOffSendsAPatchWithIfMatchAndKeepsTheNewState() = runBlocking {
        server.enqueue(json(list))
        server.enqueue(json(dsl))
        server.enqueue(json("""{"version":8}"""))
        val m = model()
        m.load()
        assertEquals(true, rules(m).first { it.id == "r1" }.enabled)

        m.setEnabled(false, "r1")

        assertEquals(false, rules(m).first { it.id == "r1" }.enabled)
        assertNull(m.toggleError.value)
        assertTrue(m.pending.value.isEmpty())
        assertEquals("/v1/apps/a1/automations", server.takeRequest().path)
        assertEquals("GET", server.takeRequest().method)
        val patch = server.takeRequest()
        assertEquals("PATCH", patch.method)
        assertEquals("/v1/apps/a1/dsl", patch.path)
        assertEquals("\"7\"", patch.getHeader("If-Match"))
        assertEquals("""{"patch":[{"op":"add","path":"/serverRules/0/enabled","value":false}]}""", patch.body.readUtf8())
    }

    @Test
    fun failureRollsBackAndSaysSo() = runBlocking {
        server.enqueue(json(list))
        server.enqueue(json(dsl))
        server.enqueue(json("""{"error":{"code":"bad_request","message":"x"}}""", status = 400))
        val m = model()
        m.load()

        m.setEnabled(false, "r1")

        assertEquals("元に戻す", true, rules(m).first { it.id == "r1" }.enabled)
        assertEquals(AutomationsModel.ToggleError.FAILED, m.toggleError.value)
        assertEquals(3, server.requestCount)
    }

    @Test
    fun conflictRollsBackAndReloads() = runBlocking {
        server.enqueue(json(list))
        server.enqueue(json(dsl))
        server.enqueue(json("""{"error":{"code":"precondition_failed","message":"x"}}""", status = 412))
        server.enqueue(json(list.replace("\"enabled\":true", "\"enabled\":false")))
        val m = model()
        m.load()

        m.setEnabled(false, "r1")

        assertEquals(AutomationsModel.ToggleError.CONFLICT, m.toggleError.value)
        assertEquals("取り直した値", false, rules(m).first { it.id == "r1" }.enabled)
        assertEquals(4, server.requestCount)
    }

    @Test
    fun notCreatorShowsNoList() = runBlocking {
        server.enqueue(json("""{"error":{"code":"forbidden","message":"x"}}""", status = 403))
        val m = model()
        m.load()
        assertEquals(AutomationsModel.Phase.NotCreator, m.phase.value)
        m.setEnabled(false, "r1")
        assertEquals("一覧が無ければ送らない", 1, server.requestCount)
    }

    @Test
    fun optimisticBeforeTheServerAnswersAndNoDoubleSend() = runBlocking {
        val gate = CompletableDeferred<Unit>()
        var calls = 0
        val client = object : AutomationsClient {
            override suspend fun automations(appId: String) =
                AppAutomations(appId, rules = listOf(AutomationRule(id = "r1", enabled = true)))

            override suspend fun setRule(ruleId: String, enabled: Boolean, appId: String) {
                calls++
                gate.await()
            }
        }
        val m = AutomationsModel("a1", client)
        m.load()
        val job = async { m.setEnabled(false, "r1") }
        while (m.pending.value.isEmpty()) kotlinx.coroutines.yield()
        assertEquals("先に見た目を変える", false, rules(m).single().enabled)
        m.setEnabled(true, "r1")
        assertEquals("送っている間は押させない", 1, calls)
        gate.complete(Unit)
        job.await()
        assertEquals(false, rules(m).single().enabled)
    }

    @Test
    fun toggleIsHiddenWhenTheServerDoesNotSayEnabled() {
        assertTrue(AutomationsModel.canToggle(AutomationRule(id = "r1", enabled = false)))
        assertFalse(AutomationsModel.canToggle(AutomationRule(id = "r2")))
    }

    @Test
    fun stringsResolveInBothLanguages() {
        val t = StringTable.fromClasspath()
        val keys = AutomationsModel.ToggleError.entries.map { it.key } + listOf(
            "automations_toggle_label", "automations_on", "automations_off", "automations_not_creator", "automations_loading",
        )
        for (lang in Lang.entries) for (k in keys) assertTrue("$k ($lang)", t.hasString(lang, k))
    }
}
