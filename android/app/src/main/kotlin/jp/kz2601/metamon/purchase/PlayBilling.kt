package jp.kz2601.metamon.purchase

import android.app.Activity
import android.content.Context
import com.android.billingclient.api.AcknowledgePurchaseParams
import com.android.billingclient.api.BillingClient
import com.android.billingclient.api.BillingClientStateListener
import com.android.billingclient.api.BillingFlowParams
import com.android.billingclient.api.BillingResult
import com.android.billingclient.api.PendingPurchasesParams
import com.android.billingclient.api.ProductDetails
import com.android.billingclient.api.Purchase
import com.android.billingclient.api.PurchasesUpdatedListener
import com.android.billingclient.api.QueryProductDetailsParams
import com.android.billingclient.api.QueryPurchasesParams
import com.android.billingclient.api.acknowledgePurchase
import com.android.billingclient.api.queryProductDetails
import com.android.billingclient.api.queryPurchasesAsync
import jp.kz2601.metamon.services.purchase.PlayPurchase
import jp.kz2601.metamon.services.purchase.ProProducts
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock

/** 課金画面に出す商品(iOS の `ProOffer`)。[offerToken] は購入の画面に渡す基本プランの印 */
data class ProOffer(val productId: String, val displayName: String, val displayPrice: String, internal val offerToken: String? = null)

/** 購入の画面の結果。購入そのものは [PlayBilling.Listener] に届く */
enum class LaunchResult { STARTED, ALREADY_OWNED, UNAVAILABLE, FAILED }

/**
 * Google Play Billing の薄いラッパー(iOS の `ProEntitlementStore` の StoreKit の部分)。外部サービス(RevenueCat 等)は使わない。
 * 商品の取得・購入の画面・購入の一覧(復元)・承認(acknowledge)だけ。Pro の判定と送信は [ProStore]。
 */
class PlayBilling(context: Context, private val listener: Listener) {
    fun interface Listener {
        /** 購入の画面の結果(買えた・保留・やめた・失敗)。[purchases] は買えた時だけ */
        fun onPurchases(result: Int, purchases: List<PlayPurchase>)
    }

    private val updated = PurchasesUpdatedListener { result: BillingResult, purchases: List<Purchase>? ->
        listener.onPurchases(result.responseCode, purchases.orEmpty().map(::toPlay))
    }

    private val client: BillingClient = BillingClient.newBuilder(context.applicationContext)
        .setListener(updated)
        .enablePendingPurchases(PendingPurchasesParams.newBuilder().enableOneTimeProducts().build())
        .enableAutoServiceReconnection()
        .build()

    private val connecting = Mutex()
    private var details: ProductDetails? = null

    /** つながっていなければつなぐ。Play ストアが無い端末・圏外では false */
    private suspend fun connect(): Boolean = connecting.withLock {
        if (client.isReady) return true
        val done = CompletableDeferred<Boolean>()
        client.startConnection(object : BillingClientStateListener {
            override fun onBillingSetupFinished(result: BillingResult) {
                done.complete(result.responseCode == BillingClient.BillingResponseCode.OK)
            }

            override fun onBillingServiceDisconnected() {
                done.complete(false)
            }
        })
        done.await()
    }

    /** 月額の商品。Play Console に商品が無い・取れなければ null */
    suspend fun offer(): ProOffer? {
        if (!connect()) return null
        val params = QueryProductDetailsParams.newBuilder()
            .setProductList(
                ProProducts.ALL.map {
                    QueryProductDetailsParams.Product.newBuilder().setProductId(it).setProductType(BillingClient.ProductType.SUBS).build()
                },
            )
            .build()
        val result = client.queryProductDetails(params)
        if (result.billingResult.responseCode != BillingClient.BillingResponseCode.OK) return null
        val product = result.productDetailsList?.firstOrNull { it.productId == ProProducts.MONTHLY } ?: return null
        details = product
        // 基本プラン(offerId の無いもの)を使う。お試しなどのオファーは Play Console で作った時に考える
        val plan = product.subscriptionOfferDetails?.let { list -> list.firstOrNull { it.offerId == null } ?: list.firstOrNull() } ?: return null
        val price = plan.pricingPhases.pricingPhaseList.lastOrNull()?.formattedPrice ?: return null
        return ProOffer(product.productId, product.name, price, plan.offerToken)
    }

    /** 購入の画面を出す。[accountToken] は uid から作った値([jp.kz2601.metamon.services.purchase.ProAccountToken]) */
    suspend fun launch(activity: Activity, offer: ProOffer, accountToken: String?): LaunchResult {
        if (!connect()) return LaunchResult.UNAVAILABLE
        val product = details?.takeIf { it.productId == offer.productId } ?: return LaunchResult.UNAVAILABLE
        val token = offer.offerToken ?: return LaunchResult.UNAVAILABLE
        val params = BillingFlowParams.newBuilder()
            .setProductDetailsParamsList(
                listOf(BillingFlowParams.ProductDetailsParams.newBuilder().setProductDetails(product).setOfferToken(token).build()),
            )
            .apply { accountToken?.let(::setObfuscatedAccountId) }
            .build()
        return when (client.launchBillingFlow(activity, params).responseCode) {
            BillingClient.BillingResponseCode.OK -> LaunchResult.STARTED
            BillingClient.BillingResponseCode.ITEM_ALREADY_OWNED -> LaunchResult.ALREADY_OWNED
            BillingClient.BillingResponseCode.BILLING_UNAVAILABLE,
            BillingClient.BillingResponseCode.SERVICE_UNAVAILABLE,
            BillingClient.BillingResponseCode.FEATURE_NOT_SUPPORTED,
            -> LaunchResult.UNAVAILABLE
            else -> LaunchResult.FAILED
        }
    }

    /** 今の購入(復元・起動時)。読めなければ null */
    suspend fun purchases(): List<PlayPurchase>? {
        if (!connect()) return null
        val result = client.queryPurchasesAsync(QueryPurchasesParams.newBuilder().setProductType(BillingClient.ProductType.SUBS).build())
        if (result.billingResult.responseCode != BillingClient.BillingResponseCode.OK) return null
        return result.purchasesList.map(::toPlay)
    }

    /** 承認。サーバーが確かめた後だけ呼ぶ(承認しないと Play が 3 日で返金する) */
    suspend fun acknowledge(purchaseToken: String): Boolean {
        if (!connect()) return false
        val result = client.acknowledgePurchase(AcknowledgePurchaseParams.newBuilder().setPurchaseToken(purchaseToken).build())
        return result.responseCode == BillingClient.BillingResponseCode.OK
    }

    private fun toPlay(p: Purchase) = PlayPurchase(
        productIds = p.products,
        purchaseToken = p.purchaseToken,
        state = when (p.purchaseState) {
            Purchase.PurchaseState.PURCHASED -> PlayPurchase.State.PURCHASED
            Purchase.PurchaseState.PENDING -> PlayPurchase.State.PENDING
            else -> PlayPurchase.State.UNSPECIFIED
        },
        acknowledged = p.isAcknowledged,
    )

    companion object {
        const val RESULT_OK = BillingClient.BillingResponseCode.OK
        const val RESULT_CANCELED = BillingClient.BillingResponseCode.USER_CANCELED
    }
}
