package jp.kz2601.metamon.purchase

import android.app.Activity
import jp.kz2601.metamon.services.purchase.EntitlementClient
import jp.kz2601.metamon.services.purchase.PlayPurchase
import jp.kz2601.metamon.services.purchase.ProProducts
import jp.kz2601.metamon.services.purchase.ServerEntitlements
import jp.kz2601.metamon.services.consent.InMemoryKeyValueStore
import jp.kz2601.metamon.services.http.PaidFeature
import jp.kz2601.metamon.services.purchase.CreationQuota
import jp.kz2601.metamon.services.purchase.CreationUsageStore
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.async
import kotlinx.coroutines.runBlocking
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.IOException

/** Tailor It Pro の状態(#77 の Android 版): Pro はサーバーの答え・Play の購入は確かめられた後だけ承認する。 */
class ProStoreTest {
    private class FakeServer(var pro: Boolean = false, var verifies: Boolean = true) : EntitlementClient {
        val submitted = mutableListOf<String>()
        override suspend fun current() = ServerEntitlements(isPro = pro)
        override suspend fun submitGooglePurchase(productId: String, purchaseToken: String): ServerEntitlements {
            submitted += purchaseToken
            if (!verifies) throw IOException("404")
            pro = true
            return ServerEntitlements(isPro = true)
        }
    }

    private class FakeBilling(var owned: List<PlayPurchase>? = emptyList()) : ProBilling {
        val acknowledged = mutableListOf<String>()
        override suspend fun offer() = ProOffer(ProProducts.MONTHLY, "Tailor It Pro", "¥500", "offer-token")
        override suspend fun launch(activity: Activity, offer: ProOffer, accountToken: String?) = LaunchResult.STARTED
        override suspend fun purchases() = owned
        override suspend fun acknowledge(purchaseToken: String): Boolean {
            acknowledged += purchaseToken
            return true
        }
    }

    private fun paid(token: String, ack: Boolean = false) = PlayPurchase(listOf(ProProducts.MONTHLY), token, PlayPurchase.State.PURCHASED, ack)

    private fun store(server: FakeServer?, billing: FakeBilling?, uid: String? = "uid1", mock: Boolean = false) =
        ProStore(server, billing, uid = { uid }, scope = CoroutineScope(Dispatchers.Unconfined), mock = mock)

    @Test
    fun proComesFromTheServerEvenWithoutAPlayPurchase() = runBlocking {
        // iOS で買った Pro も同じアカウントなら効く
        val s = store(FakeServer(pro = true), FakeBilling())
        assertFalse(s.isPro)
        s.refresh()
        assertTrue(s.isPro)
    }

    @Test
    fun verifiedPurchaseIsAcknowledgedOnce() = runBlocking {
        val server = FakeServer()
        val billing = FakeBilling()
        val s = store(server, billing)
        s.onPurchases(PlayBilling.RESULT_OK, listOf(paid("tok")))
        assertTrue(s.isPro)
        assertEquals(listOf("tok"), server.submitted)
        assertEquals(listOf("tok"), billing.acknowledged)
        // 起動のたびに送り直さない(承認済みなら承認もしない)
        billing.owned = listOf(paid("tok", ack = true))
        s.refresh()
        assertEquals(listOf("tok"), server.submitted)
        assertEquals(listOf("tok"), billing.acknowledged)
    }

    @Test
    fun unverifiedPurchaseIsNotAcknowledgedAndSaysSo() = runBlocking {
        // backend に Play の確かめが無い間: 承認しない(Play が 3 日で返金する)。Pro にもしない
        val server = FakeServer(verifies = false)
        val billing = FakeBilling()
        val s = store(server, billing)
        s.onPurchases(PlayBilling.RESULT_OK, listOf(paid("tok")))
        assertFalse(s.isPro)
        assertTrue(billing.acknowledged.isEmpty())
        assertEquals(ProMessage.NOT_VERIFIED, s.state.value.message)
        // 次の機会に送り直す
        server.verifies = true
        billing.owned = listOf(paid("tok"))
        s.restore()
        assertTrue(s.isPro)
        assertEquals(listOf("tok"), billing.acknowledged)
        assertEquals(ProMessage.RESTORED, s.state.value.message)
    }

    @Test
    fun purchaseTheServerRejectedIsNotAcknowledgedEvenWhenAlreadyPro() = runBlocking {
        // iOS で買った Pro がある人でも、サーバーが確かめられなかった Play の購入は承認しない
        val server = FakeServer(pro = true, verifies = false)
        val billing = FakeBilling(owned = listOf(paid("other")))
        val s = store(server, billing)
        s.refresh()
        assertTrue(s.isPro)
        assertEquals(listOf("other"), server.submitted)
        assertTrue(billing.acknowledged.isEmpty())
        s.onPurchases(PlayBilling.RESULT_OK, listOf(paid("tok")))
        assertTrue(billing.acknowledged.isEmpty())
    }

    @Test
    fun answerArrivingAfterResetIsNotKeptForTheNextPerson() = runBlocking {
        // サインアウトの後片付けの後に届いた前の人の答えで、次の人を Pro にしない
        val gate = kotlinx.coroutines.CompletableDeferred<Unit>()
        val server = object : EntitlementClient {
            override suspend fun current(): ServerEntitlements {
                gate.await()
                return ServerEntitlements(isPro = true)
            }

            override suspend fun submitGooglePurchase(productId: String, purchaseToken: String) = ServerEntitlements(isPro = true)
        }
        val s = ProStore(server, FakeBilling(), uid = { "uid1" }, scope = CoroutineScope(Dispatchers.Unconfined))
        val reading = async(Dispatchers.Unconfined) { s.refresh() }
        s.reset()
        gate.complete(Unit)
        reading.await()
        assertFalse(s.isPro)
    }

    @Test
    fun pendingAndCancelled() = runBlocking {
        val s = store(FakeServer(), FakeBilling())
        s.onPurchases(PlayBilling.RESULT_OK, listOf(PlayPurchase(listOf(ProProducts.MONTHLY), "p", PlayPurchase.State.PENDING, false)))
        assertEquals(ProMessage.PENDING, s.state.value.message)
        assertFalse(s.isPro)
        val s2 = store(FakeServer(), FakeBilling())
        s2.onPurchases(PlayBilling.RESULT_CANCELED, emptyList())
        assertNull(s2.state.value.message)
    }

    @Test
    fun restoreWithNothingAndWithoutPlay() = runBlocking {
        val s = store(FakeServer(), FakeBilling(owned = emptyList()))
        s.restore()
        assertEquals(ProMessage.RESTORE_NONE, s.state.value.message)
        val broken = store(FakeServer(), FakeBilling(owned = null))
        broken.restore()
        assertEquals(ProMessage.RESTORE_FAILED, broken.state.value.message)
    }

    @Test
    fun noUidSendsNothing() = runBlocking {
        val server = FakeServer()
        val s = store(server, FakeBilling(), uid = null)
        s.onPurchases(PlayBilling.RESULT_OK, listOf(paid("tok")))
        assertTrue(server.submitted.isEmpty())
    }

    @Test
    fun paywallOpensWithOfferAndResetForgetsEverything() = runBlocking {
        val s = store(FakeServer(), FakeBilling())
        s.requestPaywall(PaywallReason.AI_TEXT)
        assertEquals(PaywallReason.AI_TEXT, s.state.value.paywall)
        assertEquals("¥500", s.state.value.offer?.displayPrice)
        s.dismissPaywall()
        assertNull(s.state.value.paywall)
        s.onPurchases(PlayBilling.RESULT_OK, listOf(paid("tok")))
        assertTrue(s.isPro)
        s.reset()
        assertFalse(s.isPro)
    }

    @Test
    fun withoutPlayOrMockThereIsNoPaywall() {
        val s = store(FakeServer(), billing = null)
        assertFalse(s.isAvailable)
        s.requestPaywall(PaywallReason.AI_TEXT)
        assertNull(s.state.value.paywall)
        val mock = store(null, null, mock = true)
        assertTrue(mock.isAvailable)
    }

    // MARK: - 新規作成の無料枠(#1261)

    private fun metered(usage: CreationUsageStore, enforce: Boolean = true) = ProStore(
        FakeServer(), FakeBilling(), uid = { "uid1" }, scope = CoroutineScope(Dispatchers.Unconfined),
        usage = usage, quota = CreationQuota(dailyFreeLimit = 2), enforceLimit = enforce,
    )

    @Test
    fun admitsUntilTheFreeLimitThenWaitsForThePaywall() = runBlocking {
        val usage = CreationUsageStore(InMemoryKeyValueStore())
        val s = metered(usage)
        assertTrue(s.admitNewApp())
        s.recordNewApp()
        s.recordNewApp()
        assertEquals(CreationQuota.Decision.Blocked, s.decision)
        val admitted = async(Dispatchers.Unconfined) { s.admitNewApp() }
        assertEquals(PaywallReason.LIMIT_REACHED, s.state.value.paywall)
        s.dismissPaywall()
        assertFalse("閉じたら作らない", admitted.await())

        val bought = async(Dispatchers.Unconfined) { s.admitNewApp() }
        s.onPurchases(PlayBilling.RESULT_OK, listOf(paid("tok")))
        s.dismissPaywall()
        assertTrue("買ったら作る", bought.await())
        assertEquals(CreationQuota.Decision.Unlimited, s.decision)
        s.reset()
        assertEquals("後片付けで回数も消す", 0, usage.used())
    }

    @Test
    fun limitIsOffUnlessEnforced() = runBlocking {
        val usage = CreationUsageStore(InMemoryKeyValueStore())
        val s = metered(usage, enforce = false)
        repeat(5) { s.recordNewApp() }
        assertTrue(s.admitNewApp())
        assertNull(s.state.value.paywall)
    }

    @Test
    fun requestProCarriesTheFeaturesAndReturnsFalseWithoutBilling() = runBlocking {
        val s = metered(CreationUsageStore(InMemoryKeyValueStore()))
        val features = listOf(PaidFeature("sharedData", "共有"))
        val result = async(Dispatchers.Unconfined) { s.requestPro(features) }
        assertEquals(PaywallReason.PaidFeatures(features), s.state.value.paywall)
        s.dismissPaywall()
        assertFalse(result.await())
        assertNull(s.state.value.paywall)

        val none = store(FakeServer(), billing = null)
        assertFalse("課金画面を出せない構成は買えない", none.requestPro(features))
    }
}
