package jp.kz2601.metamon.services.automations

import jp.kz2601.metamon.services.http.ApiError
import jp.kz2601.metamon.services.http.errorBody
import jp.kz2601.metamon.services.http.json
import jp.kz2601.metamon.services.http.metamonHttp
import jp.kz2601.metamon.services.http.take
import jp.kz2601.metamon.services.live.HttpAutomationsClient
import kotlinx.coroutines.runBlocking
import okhttp3.mockwebserver.MockWebServer
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Assert.fail
import org.junit.Before
import org.junit.Test
import java.util.concurrent.TimeUnit

/** 自動の動きの API(openapi-appconfig.yaml `/apps/{appId}/automations`・`PATCH /dsl`・#905)。 */
class HttpAutomationsClientTest {
    private lateinit var server: MockWebServer
    private lateinit var client: HttpAutomationsClient

    @Before
    fun setUp() {
        server = MockWebServer().apply { start() }
        client = HttpAutomationsClient(server.metamonHttp())
    }

    @After
    fun tearDown() {
        server.shutdown()
    }

    @Test
    fun listParsesRulesQuotaAndStops() = runBlocking {
        server.enqueue(
            json(
                200,
                """{"appId":"a1","notifyPaused":{"until":"2026-11-12T00:00:00Z"},"quota":{"used":3,"limit":100,"resetAt":"2026-11-12T15:00:00Z"},
                "rules":[{"id":"r1","description":"記録が作られた時に通知","trigger":"record","result":"failed","failureReason":"timeout","enabled":true,"consecutiveFailures":4,"lastRunAt":"2026-11-11T01:00:00Z","extra":1},
                {"id":"r2","trigger":"schedule","result":"brand_new"},
                {"id":"r3","description":"x","enabled":false,"result":"disabled"}]}""",
            ),
        )
        val a = client.automations("app 1")
        assertEquals("/v1/apps/app%201/automations", server.take().path)
        assertEquals("a1", a.appId)
        assertNull(a.stopped)
        assertEquals("2026-11-12T00:00:00Z", a.notifyPaused?.until)
        assertEquals(AutomationQuota(3, 100, "2026-11-12T15:00:00Z"), a.quota)
        assertEquals(listOf("r1", "r2", "r3"), a.rules.map { it.id })
        val r1 = a.rules[0]
        assertEquals(AutomationResult.FAILED, r1.result)
        assertTrue(r1.suggestsFix)
        assertEquals(true, r1.enabled)
        // 説明が無ければ id・知らない結果は OTHER・enabled を返さない古いサーバーは null
        assertEquals("r2", a.rules[1].label)
        assertTrue(a.rules[1].isSchedule)
        assertEquals(AutomationResult.OTHER, a.rules[1].result)
        assertNull(a.rules[1].enabled)
        assertFalse(a.rules[2].result.isProblem)
    }

    @Test
    fun notCreatorIs403() = runBlocking {
        server.enqueue(json(403, errorBody("forbidden")))
        try {
            client.automations("a1")
            fail()
        } catch (e: ApiError) {
            assertEquals(403, e.status)
        }
    }

    @Test
    fun turningOffAddsEnabledFalseWithIfMatch() = runBlocking {
        server.enqueue(json(200, """{"version":7,"dsl":{"serverRules":[{"id":"r0"},{"id":"r1"}]}}"""))
        server.enqueue(json(200, """{"version":8}"""))
        client.setRule("r1", enabled = false, appId = "a1")
        assertEquals("GET", server.take().method)
        val patch = server.take()
        assertEquals("PATCH", patch.method)
        assertEquals("/v1/apps/a1/dsl", patch.path)
        assertEquals("\"7\"", patch.getHeader("If-Match"))
        assertEquals("""{"patch":[{"op":"add","path":"/serverRules/1/enabled","value":false}]}""", patch.body.readUtf8())
    }

    @Test
    fun turningOnRemovesEnabled() = runBlocking {
        server.enqueue(json(200, """{"version":2,"dsl":{"serverRules":[{"id":"r1","enabled":false}]}}"""))
        server.enqueue(json(200, """{"version":3}"""))
        client.setRule("r1", enabled = true, appId = "a1")
        server.take()
        assertEquals("""{"patch":[{"op":"remove","path":"/serverRules/0/enabled"}]}""", server.take().body.readUtf8())
    }

    @Test
    fun alreadyInStateSendsNoPatch() = runBlocking {
        server.enqueue(json(200, """{"version":2,"dsl":{"serverRules":[{"id":"r1"}]}}"""))
        client.setRule("r1", enabled = true, appId = "a1")
        server.take()
        assertNull(server.takeRequest(200, TimeUnit.MILLISECONDS))
    }

    @Test
    fun missingRuleIsConflict() = runBlocking {
        server.enqueue(json(200, """{"version":2,"dsl":{"serverRules":[]}}"""))
        try {
            client.setRule("gone", enabled = false, appId = "a1")
            fail()
        } catch (e: AutomationToggle.Failure.RuleNotFound) {
            assertTrue(AutomationToggle.isConflict(e))
        }
    }

    @Test
    fun preconditionFailedIsConflict() = runBlocking {
        server.enqueue(json(200, """{"version":2,"dsl":{"serverRules":[{"id":"r1"}]}}"""))
        server.enqueue(json(412, errorBody("precondition_failed")))
        try {
            client.setRule("r1", enabled = false, appId = "a1")
            fail()
        } catch (e: ApiError) {
            assertTrue(AutomationToggle.isConflict(e))
        }
        assertFalse(AutomationToggle.isConflict(ApiError.Http(500, "")))
    }

    @Test
    fun mockTogglesInMemory() = runBlocking {
        val mock = MockAutomationsClient()
        val id = MockAutomationsClient.DEMO[0].id
        mock.setRule(id, enabled = false, appId = "x")
        assertEquals(false, mock.automations("x").rules.first { it.id == id }.enabled)
        assertEquals(true, mock.automations("y").rules.first { it.id == id }.enabled)
    }
}
