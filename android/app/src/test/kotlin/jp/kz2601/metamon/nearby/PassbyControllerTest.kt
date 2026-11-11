package jp.kz2601.metamon.nearby

import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.decode
import jp.kz2601.metamon.dsl.nearby.NearbyLimits
import jp.kz2601.metamon.dsl.nearby.NearbyPassby
import jp.kz2601.metamon.dsl.records.JSONValue
import jp.kz2601.metamon.renderer.components.nearby.PassbyDistanceSupport
import jp.kz2601.metamon.renderer.components.nearby.PassbyPeer
import jp.kz2601.metamon.runtime.RuntimeFixture.runtime
import jp.kz2601.metamon.services.consent.InMemoryKeyValueStore
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.test.TestScope
import kotlinx.coroutines.test.advanceTimeBy
import kotlinx.coroutines.test.runCurrent
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import java.time.Duration
import java.time.Instant

/**
 * すれ違い通信(#1383・iOS の PassbyControllerTests): 同意・オンの前は通信しない / 匿名なら名前を出さない / 自分の記録だけ送る /
 * 受け取りの重複除け。通り道は差し替え。
 */
@OptIn(ExperimentalCoroutinesApi::class)
class PassbyControllerTest {
    private class FakeTransport : PassbyTransport {
        override var onPayload: ((NearbyPassby.Payload) -> Unit)? = null
        override var onPeers: ((List<PassbyPeer>) -> Unit)? = null
        override var onNotice: ((String?) -> Unit)? = null
        override val distanceSupport: PassbyDistanceSupport = PassbyDistanceSupport.Unsupported("x")
        var started: PassbyStart? = null
        var stops = 0
        override fun start(start: PassbyStart) {
            started = start
        }

        override fun stop() {
            started = null
            stops++
        }
    }

    private val dsl = AppDSL.decode(
        """
        {"schemaVersion":1,"app":{"name":"すれ違い","voiceEntry":"none"},"initialScreen":"home",
         "entities":[{"id":"profile","name":"プロフィール","fields":[
           {"id":"name","name":"名前","type":"string","required":true},{"id":"hobby","name":"趣味","type":"string"},
           {"id":"memo","name":"メモ","type":"string","hidden":true}]}],
         "screens":[{"id":"home","title":"すれ違い","components":[
           {"type":"nearbyShare","id":"pb","label":"すれ違い通信","entity":"profile","mode":"passby","fields":["name","hobby","memo"],"limit":5}]}]}
        """.trimIndent(),
    )
    private val config = NearbyPassby.Config.of(dsl.screens[0].components[0])!!
    private var clock = Instant.parse("2026-11-11T10:00:00Z")

    private fun TestScope.setUp(
        store: InMemoryKeyValueStore = InMemoryKeyValueStore(),
        transport: PassbyTransport = FakeTransport(),
        appId: String = "passby-test",
    ): Triple<PassbyController, jp.kz2601.metamon.runtime.AppRuntime, NearbySettings> {
        val rt = runtime(dsl, appId = appId)
        val settings = NearbySettings(store)
        val c = PassbyController(rt, transport, settings, backgroundScope, displayName = { "太郎の Pixel" }, now = { clock })
        return Triple(c, rt, settings)
    }

    @Test
    fun consentOptInExchangeAndDedupe() = runTest {
        val fake = FakeTransport()
        val (c, rt, _) = setUp(transport = fake)
        assertTrue("ランタイムの部品とつながる", rt.passby === c)

        c.attach(config)
        runCurrent()
        assertNull("オンにするまで探さない", fake.started)
        c.setOptIn(true, config)
        assertNull("同意の前は探さない", fake.started)
        c.agree(anonymous = true)
        c.setOptIn(true, config)
        assertNotNull(fake.started)
        val anon = fake.started!!
        assertEquals("匿名なら表示名の代わりに決まった文言", NearbyLimits.ANONYMOUS_NAME, anon.hello.name)
        assertEquals(NearbyLimits.ANONYMOUS_NAME, anon.payload().name)
        c.setAnonymous(false)
        val start = fake.started!!
        assertTrue(c.state.value.isAdvertising)
        assertEquals("切り替えたら名前を変えて探し直す", "太郎の Pixel", start.hello.name)
        assertEquals(NearbyPassby.appTag("passby-test"), start.hello.app)

        rt.createRecord("profile", mapOf("name" to JSONValue.Str("太郎"), "hobby" to JSONValue.Str("釣り"), "memo" to JSONValue.Str("秘密")))
        val mine = start.payload()
        assertEquals("hidden は送らない", listOf(mapOf("name" to JSONValue.Str("太郎"), "hobby" to JSONValue.Str("釣り"))), mine.records.map { it.data })

        val theirs = NearbyPassby.Payload(
            1, "s2", "花子", "profile",
            listOf(NearbyPassby.Item("r1", mapOf("name" to JSONValue.Str("花子"), "hobby" to JSONValue.Str("読書"), "memo" to JSONValue.Str("x")))),
        )
        c.receive(theirs)
        c.receive(theirs)
        assertEquals("同じ相手・同じ記録は 1 回だけ", 2, rt.records("profile").size)
        assertTrue("許可していない項目は保存しない", rt.records("profile").all { it.data["memo"] != JSONValue.Str("x") })

        val changed = theirs.copy(records = listOf(NearbyPassby.Item("r1", mapOf("name" to JSONValue.Str("花子"), "hobby" to JSONValue.Str("映画")))))
        c.receive(changed)
        assertEquals("内容が変わったら更新", 2, rt.records("profile").size)
        assertTrue(rt.records("profile").any { it.data["hobby"] == JSONValue.Str("映画") })

        assertEquals("受け取った記録は送り返さない", listOf(JSONValue.Str("太郎")), start.payload().records.map { it.data["name"] })
        assertEquals(listOf("花子"), c.state.value.encounters.map { it.name })
        assertFalse("続けて交換しない", start.shouldExchange("s2"))
        clock = clock.plus(Duration.ofMinutes(11))
        assertTrue("10 分あけたらまた交換する", start.shouldExchange("s2"))

        c.detach(config)
        assertNull(fake.started)
        assertFalse(c.state.value.isAdvertising)
    }

    @Test
    fun ledgerConsentAndOptInSurviveRestart() = runTest {
        val store = InMemoryKeyValueStore()
        val (first, _, _) = setUp(store = store)
        first.attach(config)
        runCurrent()
        first.agree(anonymous = false)
        first.setOptIn(true, config)
        first.receive(NearbyPassby.Payload(1, "s2", "花子", "profile", listOf(NearbyPassby.Item("r1", mapOf("name" to JSONValue.Str("花子"))))))
        first.detach(config)

        val fake = FakeTransport()
        val (second, _, _) = setUp(store = store, transport = fake)
        assertEquals("同意の版を覚えている", NearbyPassby.CONSENT_VERSION, second.state.value.consentedVersion)
        second.attach(config)
        runCurrent()
        assertTrue("オンのまま", second.isOptedIn(config))
        assertNotNull("前面に戻ったら探し直す", fake.started)
        assertEquals("会った人を覚えている", listOf("s2"), second.state.value.encounters.map { it.id })
        assertFalse("台帳も残る(続けて交換しない)", fake.started!!.shouldExchange("s2"))
        assertEquals("senderId は端末で 1 つ", store.getString(PassbyController.SENDER_ID_KEY), fake.started!!.hello.senderId)
    }

    @Test
    fun oldConsentVersionAsksAgain() = runTest {
        val store = InMemoryKeyValueStore().apply { putInt(PassbyController.CONSENT_KEY, 1) }
        val fake = FakeTransport()
        val (c, _, _) = setUp(store = store, transport = fake)
        c.attach(config)
        runCurrent()
        c.setOptIn(true, config)
        assertNull("同意の版が上がったら聞き直す(探さない)", fake.started)
        assertEquals(NearbyPassby.ToggleOutcome.ASK_CONSENT, NearbyPassby.toggle(true, c.state.value.consentedVersion))
    }

    @Test
    fun anonymousIsSharedWithNearbyShare() = runTest {
        val fake = FakeTransport()
        val (c, _, settings) = setUp(transport = fake)
        c.attach(config)
        runCurrent()
        c.agree(anonymous = false)
        c.setOptIn(true, config)
        assertEquals("太郎の Pixel", fake.started!!.hello.name)
        settings.anonymous = true // 近くの人に送るシートで切り替えた
        assertTrue(c.state.value.isAnonymous)
        assertEquals("すれ違い通信も名前を変えて探し直す", NearbyLimits.ANONYMOUS_NAME, fake.started!!.hello.name)
    }

    @Test
    fun ledgerIsPerApp() = runTest {
        val store = InMemoryKeyValueStore()
        val (a, _, _) = setUp(store = store, appId = "app-a")
        a.attach(config)
        runCurrent()
        a.agree(anonymous = false)
        a.setOptIn(true, config)
        a.receive(NearbyPassby.Payload(1, "s2", "花子", "profile", listOf(NearbyPassby.Item("r1", mapOf("name" to JSONValue.Str("花子"))))))
        val (b, _, _) = setUp(store = store, appId = "app-b")
        b.attach(config)
        runCurrent()
        assertTrue("別のアプリの会った人は出さない", b.state.value.encounters.isEmpty())
        assertFalse("別のアプリのオンは引き継がない", b.isOptedIn(config))
    }

    @Test
    fun mockTransportPassesTestHanakoThroughTheRealPath() = runTest {
        val mock = MockPassbyTransport(backgroundScope, delayMillis = 1_500, now = { clock })
        val (c, rt, _) = setUp(transport = mock)
        c.attach(config)
        runCurrent()
        c.agree(anonymous = false)
        c.setOptIn(true, config)
        advanceTimeBy(1_600)
        runCurrent()
        assertEquals(listOf(MockPassbyTransport.PEER_NAME), c.state.value.encounters.map { it.name })
        val saved = rt.records("profile").single()
        assertEquals("許可 field に見本の値", JSONValue.Str(MockPassbyTransport.PEER_NAME), saved.data["name"])
        assertNull("hidden には入れない", saved.data["memo"])
        assertTrue(c.state.value.distanceSupport is PassbyDistanceSupport.Unsupported)
    }
}
