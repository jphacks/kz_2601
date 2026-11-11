package jp.kz2601.metamon.services.purchase

import jp.kz2601.metamon.services.http.json
import jp.kz2601.metamon.services.http.metamonHttp
import jp.kz2601.metamon.services.http.take
import kotlinx.coroutines.runBlocking
import okhttp3.mockwebserver.MockWebServer
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotEquals
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test

/** Tailor It Pro の Android の購入(#77): 商品 ID・uid の印・権利の API・Play の購入の扱い。 */
class ProPurchasesTest {
    private lateinit var server: MockWebServer

    @Before
    fun setUp() {
        server = MockWebServer().apply { start() }
    }

    @After
    fun tearDown() {
        server.shutdown()
    }

    @Test
    fun productIdIsTheSameAsIos() {
        assertEquals(listOf("jp.kz2601.metamon.pro.monthly"), ProProducts.ALL)
    }

    @Test
    fun accountTokenMatchesIosAndBackend() {
        // iOS の ProServerEntitlementsTests・backend の entitlement.AccountToken と同じ値
        assertEquals("578b5ddb-6720-8e0a-a511-88fa2b02abdb", ProAccountToken.uuid("firebase-uid-123"))
        assertNotEquals(ProAccountToken.uuid("a"), ProAccountToken.uuid("b"))
        assertTrue(ProAccountToken.uuid("x").length <= 64) // Play の obfuscatedAccountId の上限
    }

    @Test
    fun readsAndSubmitsEntitlements() = runBlocking {
        val client = HttpEntitlementClient(server.metamonHttp())
        server.enqueue(json(200, """{"isPro":true,"pro":{"productId":"jp.kz2601.metamon.pro.monthly","environment":"Production","expiresAt":"2026-12-11T00:00:00Z"}}"""))
        val current = client.current()
        assertTrue(current.isPro)
        assertEquals("Production", current.pro?.environment)
        val get = server.take()
        assertEquals("GET", get.method)
        assertEquals("/v1/me/entitlements", get.path)

        server.enqueue(json(200, """{"isPro":false}"""))
        assertFalse(client.submitGooglePurchase(ProProducts.MONTHLY, "tok-1").isPro)
        val post = server.take()
        assertEquals("POST", post.method)
        assertEquals("/v1/me/entitlements/google", post.path)
        assertEquals("""{"productId":"jp.kz2601.metamon.pro.monthly","purchaseToken":"tok-1"}""", post.body.readUtf8())
    }

    private fun purchase(token: String, state: PlayPurchase.State = PlayPurchase.State.PURCHASED, product: String = ProProducts.MONTHLY, ack: Boolean = false) =
        PlayPurchase(listOf(product), token, state, ack)

    @Test
    fun submitsPaidProPurchasesOncePerUid() {
        val list = listOf(
            purchase("paid"),
            purchase("pending", PlayPurchase.State.PENDING),
            purchase("other", product = "something.else"),
            purchase("sent"),
        )
        val submitted = setOf(PlayPurchaseRules.submittedKey("uid1", "sent"))
        assertEquals(listOf("paid"), PlayPurchaseRules.toSubmit(list, submitted, "uid1").map { it.purchaseToken })
        // 別の人なら、送ったものも送り直す
        assertEquals(listOf("paid", "sent"), PlayPurchaseRules.toSubmit(list, submitted, "uid2").map { it.purchaseToken })
        assertTrue(PlayPurchaseRules.hasPending(list))
        assertFalse(PlayPurchaseRules.hasPending(listOf(purchase("paid"))))
    }

    @Test
    fun acknowledgesOnlyAfterTheServerSaysPro() {
        assertFalse(PlayPurchaseRules.shouldAcknowledge(purchase("p"), serverPro = false))
        assertTrue(PlayPurchaseRules.shouldAcknowledge(purchase("p"), serverPro = true))
        assertFalse(PlayPurchaseRules.shouldAcknowledge(purchase("p", ack = true), serverPro = true))
        assertFalse(PlayPurchaseRules.shouldAcknowledge(purchase("p", PlayPurchase.State.PENDING), serverPro = true))
        assertFalse(PlayPurchaseRules.shouldAcknowledge(purchase("p", product = "something.else"), serverPro = true))
    }
}
