package jp.kz2601.metamon.desktop.net

import jp.kz2601.metamon.desktop.DesktopKeyboard
import jp.kz2601.metamon.desktop.DesktopSession
import jp.kz2601.metamon.desktop.Examples
import jp.kz2601.metamon.desktop.HostOs
import jp.kz2601.metamon.desktop.MockStore
import jp.kz2601.metamon.desktop.chat.AiConsentGate
import jp.kz2601.metamon.desktop.chat.ChatEntry
import jp.kz2601.metamon.desktop.chat.ChatSession
import jp.kz2601.metamon.desktop.i18n.Lang
import jp.kz2601.metamon.desktop.i18n.Strings
import jp.kz2601.metamon.desktop.store.JsonKeyValueStore
import jp.kz2601.metamon.services.ChatMessage
import jp.kz2601.metamon.services.ChatTurn
import jp.kz2601.metamon.services.consent.AiConsentRequired
import jp.kz2601.metamon.services.consent.AiConsentStore
import kotlinx.coroutines.runBlocking
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Assert.fail
import org.junit.Before
import org.junit.Test
import java.net.InetAddress
import java.util.concurrent.TimeUnit

/** AI の接続先と同意(#2160): `TAILORIT_AI_URL`・モックは通信しない・同意が無ければ送らない・`/v1/chat` を 1 往復(MockWebServer だけ)。 */
class DesktopAi2160Test {
    private lateinit var server: MockWebServer

    @Before
    fun setUp() {
        server = MockWebServer().apply { start() }
    }

    @After
    fun tearDown() {
        server.shutdown()
    }

    private fun baseUrl() = server.url("/").newBuilder().host(InetAddress.getByName(server.hostName).hostAddress).build().toString()

    private fun consent(granted: Boolean) = AiConsentStore(JsonKeyValueStore(null)).also { if (granted) it.grant() }

    @Test
    fun aiUrlFromEnvironment() {
        // 既定(モック): AI の接続先は無い
        assertNull(DesktopApiConfig.fromEnvironment(emptyMap()).aiUrl)
        assertNull(DesktopApiConfig.fromEnvironment(mapOf("TAILORIT_AI_URL" to "  ")).aiUrl)
        // stg: 既定は stg の ai。TAILORIT_AI_URL で差し替え
        assertEquals(DesktopApiConfig.STG_AI_URL, DesktopApiConfig.fromEnvironment(mapOf("TAILORIT_API" to "stg")).aiUrl)
        assertEquals(
            "http://localhost:8082",
            DesktopApiConfig.fromEnvironment(mapOf("TAILORIT_API" to "stg", "TAILORIT_AI_URL" to " http://localhost:8082 ")).aiUrl,
        )
        // モックのままでも AI の URL だけ渡せる(手元の backend)
        val local = DesktopApiConfig.fromEnvironment(mapOf("TAILORIT_AI_URL" to "http://localhost:8082"))
        assertFalse(local.isLive)
        assertEquals("http://localhost:8082", local.aiUrl)
    }

    @Test
    fun toStringHidesToken() {
        val text = DesktopApiConfig(aiUrl = "http://ai", devToken = "secret-token").toString()
        assertFalse(text, text.contains("secret-token"))
        assertTrue(text, text.contains("aiUrl=http://ai"))
    }

    @Test
    fun mockHasNoClientAndNoChatEntry() {
        assertNull(DesktopApi.ai(DesktopApiConfig(), isGranted = { true }))
        assertNull(ChatEntry.factory(null))
    }

    @Test
    fun chatRoundTripWithConsent() = runBlocking {
        server.enqueue(
            MockResponse().setHeader("Content-Type", "application/json")
                .setBody("""{"kind":"question","text":"What do you track?","turn":1,"suggestions":["Spending"]}"""),
        )
        val ai = DesktopApi.ai(DesktopApiConfig(aiUrl = baseUrl(), devToken = "dev-token"), isGranted = { true })!!

        val turn = ai.chat("app-1", listOf(ChatMessage(ChatMessage.Role.USER, "budget")), "en", null)

        assertEquals(ChatTurn.Kind.QUESTION, turn.kind)
        assertEquals("What do you track?", turn.text)
        val req = server.takeRequest(5, TimeUnit.SECONDS)!!
        assertEquals("POST", req.method)
        assertEquals("/v1/chat", req.path)
        assertEquals("Bearer dev-token", req.getHeader("Authorization"))
        assertTrue(req.body.readUtf8().contains("\"appId\":\"app-1\""))
    }

    @Test
    fun noConsentSendsNothing() = runBlocking {
        val ai = DesktopApi.ai(DesktopApiConfig(aiUrl = baseUrl()), isGranted = { false })!!
        try {
            ai.chat("app-1", listOf(ChatMessage(ChatMessage.Role.USER, "budget")))
            fail("sent without consent")
        } catch (_: AiConsentRequired) {
        }
        ai.resetChat("app-1")
        assertEquals(0, server.requestCount)
    }

    @Test
    fun chatEntryMakesFreshAppIds() {
        val ai = DesktopApi.ai(DesktopApiConfig(aiUrl = baseUrl()), isGranted = { true })
        val make = ChatEntry.factory(ai)!!
        val a = make()
        val b = make()
        assertTrue(a.appId.isNotBlank())
        assertTrue(a.appId != b.appId)
    }

    @Test
    fun paletteAsksConsentBeforeOpeningChat() {
        val s = DesktopSession(Examples.load("shopping"), MockStore())
        val k = DesktopKeyboard(s.dsl, s, HostOs.WINDOWS)
        val store = consent(granted = false)
        val ai = DesktopApi.ai(DesktopApiConfig(aiUrl = baseUrl()), isGranted = { store.isGranted })
        k.newChat = ChatEntry.factory(ai)
        k.aiConsent = store

        // 同意が無い: 先に説明を出し、会話はまだ開かない
        assertTrue(k.openChat())
        assertTrue(k.consentOpen)
        assertNull(k.chat)
        // 「あとで」: 会話は開かず、同意も無いまま
        k.answerConsent(false)
        assertFalse(k.consentOpen)
        assertNull(k.chat)
        assertFalse(store.isGranted)
        // 「同意して使う」: 同意を保存して会話を開く
        k.openChat()
        k.answerConsent(true)
        assertTrue(store.isGranted)
        assertNotNull(k.chat)
        assertEquals(0, server.requestCount)
    }

    @Test
    fun grantedConsentOpensChatDirectly() {
        val s = DesktopSession(Examples.load("shopping"), MockStore())
        val k = DesktopKeyboard(s.dsl, s, HostOs.WINDOWS)
        k.newChat = { ChatSession(DesktopApi.ai(DesktopApiConfig(aiUrl = baseUrl()), isGranted = { true })!!, appId = "x") }
        k.aiConsent = consent(granted = true)
        assertTrue(k.openChat())
        assertFalse(k.consentOpen)
        assertNotNull(k.chat)
    }

    @Test
    fun consentTextUsesAndroidKeys() {
        for (lang in listOf(Lang.JA, Lang.EN)) {
            for (key in listOf("consent_title", "consent_intro", "consent_agree", "consent_later", "consent_reconsent", "consent_without")) {
                assertTrue("$key $lang", Strings.table.hasString(lang, key))
            }
        }
        val first = Strings.withLang(Lang.JA) { AiConsentGate.paragraphs(reconsent = false) }
        val again = Strings.withLang(Lang.JA) { AiConsentGate.paragraphs(reconsent = true) }
        assertEquals(first.size + 1, again.size)
        assertEquals(Strings.withLang(Lang.JA) { Strings["consent_reconsent"] }, again.first())
        assertTrue(AiConsentGate.needsPrompt(consent(granted = false)))
        assertFalse(AiConsentGate.needsPrompt(consent(granted = true)))
        assertFalse(AiConsentGate.needsPrompt(null))
    }
}
