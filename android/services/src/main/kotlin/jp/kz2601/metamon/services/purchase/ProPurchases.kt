package jp.kz2601.metamon.services.purchase

import jp.kz2601.metamon.services.http.ApiJson
import jp.kz2601.metamon.services.http.HttpRequest
import jp.kz2601.metamon.services.http.MetamonHttp
import kotlinx.serialization.Serializable
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.put
import java.security.MessageDigest

/**
 * 「Tailor It Pro」の商品(iOS の `ProProducts`・#77)。Play Console にも同じ ID の定期購入を作る。
 * Play の商品 ID は英小文字・数字・`_`・`.` なので iOS の ID をそのまま使える。
 */
object ProProducts {
    /** 月額の定期購入(自動更新) */
    const val MONTHLY = "jp.kz2601.metamon.pro.monthly"
    val ALL = listOf(MONTHLY)

    /** 無料で 1 日に新規作成できるアプリの数(iOS の `ProProducts.dailyFreeCreations`) */
    const val DAILY_FREE_CREATIONS = 3
}

/**
 * 購入にどの uid の分かを付ける値(iOS の `ProAccountToken`・backend の `entitlement.AccountToken` と同じ計算)。
 * Firebase の uid をそのまま Play に渡さないよう、SHA-256 から UUIDv8 を作る。Play では `obfuscatedAccountId`(64 文字まで)に入れる。
 */
object ProAccountToken {
    fun uuid(uid: String): String {
        val b = MessageDigest.getInstance("SHA-256").digest("tailorit:appAccountToken:$uid".toByteArray(Charsets.UTF_8))
        b[6] = ((b[6].toInt() and 0x0f) or 0x80).toByte() // version 8
        b[8] = ((b[8].toInt() and 0x3f) or 0x80).toByte() // RFC 9562 variant
        val hex = b.take(16).joinToString("") { "%02x".format(it) }
        return "${hex.substring(0, 8)}-${hex.substring(8, 12)}-${hex.substring(12, 16)}-${hex.substring(16, 20)}-${hex.substring(20, 32)}"
    }
}

/** サーバーが保存した Pro の権利(`GET /v1/me/entitlements`・openapi-appconfig.yaml `Entitlements`)。 */
@Serializable
data class ServerEntitlements(val isPro: Boolean = false, val pro: Pro? = null) {
    @Serializable
    data class Pro(val productId: String = "", val environment: String = "", val expiresAt: String = "")
}

/**
 * Pro の権利の API。`/v1/text` などの Pro の判定はサーバーがするので、画面もサーバーの答えを使う。
 * Play の購入は [submitGooglePurchase](`POST /v1/me/entitlements/google`)で送る。
 */
interface EntitlementClient {
    /** `GET /v1/me/entitlements` */
    suspend fun current(): ServerEntitlements

    /**
     * Play の購入を送り、サーバーが Google Play Developer API で確かめて Pro を保存する。
     * 確かめられた時だけ購入を承認(acknowledge)する。承認しないと 3 日で返金される(お金を取りっぱなしにしない)。
     */
    suspend fun submitGooglePurchase(productId: String, purchaseToken: String): ServerEntitlements
}

class HttpEntitlementClient(private val http: MetamonHttp) : EntitlementClient {
    override suspend fun current(): ServerEntitlements =
        ApiJson.decodeFromString(ServerEntitlements.serializer(), http.send(HttpRequest("GET", listOf("v1", "me", "entitlements"), timeoutMillis = TIMEOUT)))

    override suspend fun submitGooglePurchase(productId: String, purchaseToken: String): ServerEntitlements {
        val body = buildJsonObject {
            put("productId", productId)
            put("purchaseToken", purchaseToken)
        }.toString()
        val res = http.send(HttpRequest("POST", listOf("v1", "me", "entitlements", "google"), body = body, timeoutMillis = TIMEOUT))
        return ApiJson.decodeFromString(ServerEntitlements.serializer(), res)
    }

    private companion object {
        const val TIMEOUT = 20_000L
    }
}

/** Play の購入 1 件(BillingClient の `Purchase` から必要な所だけ)。 */
data class PlayPurchase(
    val productIds: List<String>,
    val purchaseToken: String,
    val state: State,
    val acknowledged: Boolean,
) {
    enum class State { PURCHASED, PENDING, UNSPECIFIED }
}

/** Play の購入の扱い(Android の部品に触らない・JVM テスト)。 */
object PlayPurchaseRules {
    /** サーバーに送る購入: 支払い済みで Pro の商品を含み、この uid でまだ送れていないもの。保留中(コンビニ払いなど)は確定してから。 */
    fun toSubmit(purchases: List<PlayPurchase>, submitted: Set<String>, uid: String): List<PlayPurchase> =
        purchases.filter { p ->
            p.state == PlayPurchase.State.PURCHASED &&
                p.productIds.any { it in ProProducts.ALL } &&
                submittedKey(uid, p.purchaseToken) !in submitted
        }

    /** 承認してよいか: サーバーが確かめて Pro にした後だけ(確かめられない購入は承認せず、Play が 3 日で返金する)。 */
    fun shouldAcknowledge(p: PlayPurchase, serverPro: Boolean): Boolean =
        serverPro && !p.acknowledged && p.state == PlayPurchase.State.PURCHASED && p.productIds.any { it in ProProducts.ALL }

    /** 保留中の購入がある(「承認待ちです」を出す) */
    fun hasPending(purchases: List<PlayPurchase>): Boolean =
        purchases.any { p -> p.state == PlayPurchase.State.PENDING && p.productIds.any { it in ProProducts.ALL } }

    fun submittedKey(uid: String, purchaseToken: String): String = "$uid|$purchaseToken"
}
