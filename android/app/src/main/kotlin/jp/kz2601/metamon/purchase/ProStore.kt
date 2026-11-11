package jp.kz2601.metamon.purchase

import android.app.Activity
import jp.kz2601.metamon.services.http.PaidFeature
import jp.kz2601.metamon.services.purchase.CreationQuota
import jp.kz2601.metamon.services.purchase.CreationUsageStore
import jp.kz2601.metamon.services.purchase.EntitlementClient
import jp.kz2601.metamon.services.purchase.PlayPurchase
import jp.kz2601.metamon.services.purchase.PlayPurchaseRules
import jp.kz2601.metamon.services.purchase.ProAccountToken
import jp.kz2601.metamon.services.purchase.ProProducts
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock

/** Play の購入の口([PlayBilling])。テストでは差し替える */
interface ProBilling {
    suspend fun offer(): ProOffer?
    suspend fun launch(activity: Activity, offer: ProOffer, accountToken: String?): LaunchResult
    suspend fun purchases(): List<PlayPurchase>?
    suspend fun acknowledge(purchaseToken: String): Boolean
}

/** 課金画面を開いた理由(iOS の `PaywallReason`)。見出しと「閉じるとどうなるか」が変わる */
enum class PaywallReason {
    SETTINGS,
    AI_TEXT,
    AI_CHAT,

    /** 今日の無料作成を使い切った(#1261) */
    LIMIT_REACHED,

    /** 新しく作るアプリに有料の機能が入る(402 `pro_required`・#1261) */
    PAID_FEATURE,
}

/** 課金画面の下に出す知らせ */
enum class ProMessage { PENDING, PURCHASE_FAILED, NOT_VERIFIED, UNAVAILABLE, RESTORED, RESTORE_NONE, RESTORE_FAILED }

data class ProUiState(
    /** サーバーが Pro と認めているか(`/v1/text` などの判定と同じ) */
    val isPro: Boolean = false,
    val offer: ProOffer? = null,
    val loadingOffer: Boolean = false,
    val busy: Boolean = false,
    val message: ProMessage? = null,
    /** 出している課金画面。null = 出していない */
    val paywall: PaywallReason? = null,
    /** [PaywallReason.PAID_FEATURE] の時に要る機能(空なら「みんなで共有するデータ」) */
    val paywallFeatures: List<PaidFeature> = emptyList(),
)

/**
 * Tailor It Pro の状態と購入の流れ(iOS の `ProGate` + `ProEntitlementStore` のサーバーの部分)。
 * - Pro かどうかはサーバー(`GET /v1/me/entitlements`)の答え。iOS で買った Pro も同じアカウントなら効く
 * - Play で買ったら購入をサーバーに送り(`POST /v1/me/entitlements/google`・backend は未実装)、確かめられた後だけ承認する
 * - モックは Play に触れず、購入はその場で Pro になる(iOS のモックと同じ)
 * - 新規作成の無料枠(#1261・iOS の `ProGate.admitNewApp`): 回数は端末で数え、Pro はサーバーの答え
 *
 * @param usage 今日の新規作成の回数。null なら数えない(枠を見ない)
 * @param enforceLimit 無料枠を効かせるか(iOS の `isEnforced`。debug は既定で切り、つまみ `METAMON_IAP_ENFORCE=1` で効かせる)
 */
class ProStore(
    private val entitlements: EntitlementClient?,
    private val billing: ProBilling?,
    private val uid: () -> String?,
    private val scope: CoroutineScope,
    private val mock: Boolean = false,
    private val log: (String) -> Unit = {},
    private val usage: CreationUsageStore? = null,
    private val quota: CreationQuota = CreationQuota(),
    private val enforceLimit: Boolean = false,
) : CreationGate {
    private val _state = MutableStateFlow(ProUiState())
    val state: StateFlow<ProUiState> = _state.asStateFlow()

    val isPro: Boolean get() = _state.value.isPro

    /** 課金画面と Pro の欄を出すか(Play も、モックの購入も無い構成では出さない) */
    val isAvailable: Boolean get() = mock || billing != null

    /** 送れた購入(uid + token)。同じものを何度も送らない。失敗したものは入れない(次の起動・復元で送り直す) */
    private val submitted = mutableSetOf<String>()
    private val syncing = Mutex()

    /** [requestPro] で課金画面が閉じるのを待っている側 */
    private var waiter: CompletableDeferred<Boolean>? = null

    override val dailyFreeLimit: Int get() = quota.dailyFreeLimit

    /** 今日の無料枠の判定(Pro・制限を切っている間は数えない) */
    val decision: CreationQuota.Decision
        get() = quota.decide(used = usage?.used() ?: 0, isPro = isPro, limitDisabled = usage == null || !enforceLimit)

    /** 新規作成の前。枠切れなら課金画面を出し、閉じるまで待つ。戻り値 = 作ってよいか(枠がある・買った) */
    override suspend fun admitNewApp(): Boolean {
        if (decision != CreationQuota.Decision.Blocked) return true
        log("IAP: daily free limit reached → paywall")
        val bought = requestPro(PaywallReason.LIMIT_REACHED)
        return bought || decision != CreationQuota.Decision.Blocked
    }

    /** 新規作成ができた時(失敗・取り消しは数えない)。Pro・制限を切っている間も記録だけはする */
    override fun recordNewApp() {
        usage?.record()
    }

    override suspend fun requestPro(features: List<PaidFeature>): Boolean = requestPro(PaywallReason.PAID_FEATURE, features)

    /**
     * 課金画面を出し、閉じるまで待つ。戻り値 = 閉じた時に Pro か(買った・復元した)。Pro ならすぐ true。
     * 前の課金画面が開いていれば、それは閉じた扱いにする。課金画面を出せない構成は false。
     */
    suspend fun requestPro(reason: PaywallReason, features: List<PaidFeature> = emptyList()): Boolean {
        if (isPro) return true
        if (!isAvailable) return false
        finishWaiter()
        val done = CompletableDeferred<Boolean>()
        waiter = done
        _state.update { it.copy(paywall = reason, paywallFeatures = features, message = null) }
        scope.launch { loadOffer() }
        return done.await()
    }

    private fun finishWaiter() {
        val w = waiter
        waiter = null
        w?.complete(isPro)
    }

    /** AI ボタンなどで Pro が要る時(iOS の `requestPro`)。開いたら商品を読む */
    fun requestPaywall(reason: PaywallReason) {
        if (!isAvailable) return
        _state.update { it.copy(paywall = reason, paywallFeatures = emptyList(), message = null) }
        scope.launch { loadOffer() }
    }

    fun dismissPaywall() {
        _state.update { it.copy(paywall = null, paywallFeatures = emptyList(), message = null) }
        finishWaiter()
    }

    /** サインインの後・起動時: サーバーの Pro を読み、送れていない Play の購入を送る */
    suspend fun refresh() {
        readServer()
        billing?.let { b -> attempt { b.purchases() }?.let { sync(it, fromPurchase = false) } }
    }

    suspend fun loadOffer() {
        if (mock) {
            _state.update { it.copy(offer = MOCK_OFFER) }
            return
        }
        val b = billing ?: return
        if (_state.value.offer != null) return
        _state.update { it.copy(loadingOffer = true) }
        val offer = attempt { b.offer() }
        _state.update { it.copy(offer = offer, loadingOffer = false) }
    }

    /** 「Pro にする」。結果は Play から [onPurchases] に届く */
    suspend fun purchase(activity: Activity) {
        if (mock) {
            _state.update { it.copy(isPro = true, message = null) }
            return
        }
        val b = billing ?: return
        val offer = _state.value.offer ?: return
        _state.update { it.copy(busy = true, message = null) }
        val token = uid()?.takeIf { it.isNotEmpty() }?.let(ProAccountToken::uuid)
        val result = attempt { b.launch(activity, offer, token) } ?: LaunchResult.FAILED
        _state.update { it.copy(busy = result == LaunchResult.STARTED) }
        when (result) {
            LaunchResult.STARTED -> Unit
            LaunchResult.ALREADY_OWNED -> restore()
            LaunchResult.UNAVAILABLE -> _state.update { it.copy(message = ProMessage.UNAVAILABLE) }
            LaunchResult.FAILED -> _state.update { it.copy(message = ProMessage.PURCHASE_FAILED) }
        }
    }

    /** Play の購入の画面の結果([PlayBilling.Listener]) */
    fun onPurchases(result: Int, purchases: List<PlayPurchase>) {
        scope.launch {
            when (result) {
                PlayBilling.RESULT_OK -> sync(purchases, fromPurchase = true)
                PlayBilling.RESULT_CANCELED -> Unit
                else -> _state.update { it.copy(message = ProMessage.PURCHASE_FAILED) }
            }
            _state.update { it.copy(busy = false) }
        }
    }

    /** 「購入を復元」: Play の購入を読み直して送り、サーバーの Pro を読む */
    suspend fun restore() {
        if (mock) return
        val b = billing ?: return
        _state.update { it.copy(busy = true, message = null) }
        val purchases = attempt { b.purchases() }
        if (purchases == null) {
            _state.update { it.copy(busy = false, message = ProMessage.RESTORE_FAILED) }
            return
        }
        sync(purchases, fromPurchase = false)
        readServer()
        _state.update { it.copy(busy = false, message = if (it.isPro) ProMessage.RESTORED else ProMessage.RESTORE_NONE) }
    }

    /** 後片付け(サインアウト・削除・別の人のサインイン・#841) */
    fun reset() {
        submitted.clear()
        _state.value = ProUiState()
        finishWaiter()
        usage?.reset() // 今日の作成回数も人ごと(iOS は LocalWipe で消える)
    }

    private suspend fun readServer() {
        val client = entitlements ?: return
        attempt { client.current() }?.let { s -> _state.update { it.copy(isPro = s.isPro) } }
    }

    private suspend fun sync(purchases: List<PlayPurchase>, fromPurchase: Boolean) = syncing.withLock {
        val id = uid()?.takeIf { it.isNotEmpty() } ?: return@withLock
        if (PlayPurchaseRules.hasPending(purchases)) _state.update { it.copy(message = ProMessage.PENDING) }
        for (p in PlayPurchaseRules.toSubmit(purchases, submitted, id)) {
            val productId = p.productIds.first { it in ProProducts.ALL }
            val server = entitlements?.let { client -> attempt { client.submitGooglePurchase(productId, p.purchaseToken) } }
            if (server == null) {
                log("IAP: Play purchase not verified by the server")
                if (fromPurchase) _state.update { it.copy(message = ProMessage.NOT_VERIFIED) }
                continue
            }
            submitted += PlayPurchaseRules.submittedKey(id, p.purchaseToken)
            _state.update { it.copy(isPro = server.isPro) }
        }
        // 承認はサーバーが Pro と認めた後だけ(承認に失敗しても、次の起動・復元でまた承認する)
        val server = _state.value.isPro
        for (p in purchases) {
            if (PlayPurchaseRules.shouldAcknowledge(p, server)) {
                billing?.let { b -> attempt { b.acknowledge(p.purchaseToken) } }
            }
        }
    }

    private inline fun <T> attempt(block: () -> T): T? = try {
        block()
    } catch (e: CancellationException) {
        throw e
    } catch (e: Exception) {
        log("IAP: ${e.javaClass.simpleName}")
        null
    }

    companion object {
        /** モックの月額(iOS の `ProOffer.mockMonthly` と同じ値) */
        val MOCK_OFFER = ProOffer(ProProducts.MONTHLY, "Tailor It Pro", "¥500")
    }
}

/** [PlayBilling] を [ProBilling] として使う */
class PlayProBilling(private val billing: PlayBilling) : ProBilling {
    override suspend fun offer() = billing.offer()
    override suspend fun launch(activity: Activity, offer: ProOffer, accountToken: String?) = billing.launch(activity, offer, accountToken)
    override suspend fun purchases() = billing.purchases()
    override suspend fun acknowledge(purchaseToken: String) = billing.acknowledge(purchaseToken)
}
