package jp.kz2601.metamon.desktop.net

import jp.kz2601.metamon.services.MyUsage
import jp.kz2601.metamon.services.consent.KeyValueStore
import jp.kz2601.metamon.services.http.HttpRequest
import jp.kz2601.metamon.services.http.MetamonHttp
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.booleanOrNull
import kotlinx.serialization.json.contentOrNull
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import java.time.Instant

// Pro かどうか(#2205・親 #1760)。権利の正はサーバー(#314)なので、デスクトップは読むだけ(買う所は作らない)。

/** `GET /v1/me/entitlements`(openapi-appconfig.yaml `Entitlements`)のうち、表示に使う所。 */
data class Entitlements(val isPro: Boolean, val pro: Pro?) {
    data class Pro(
        val productId: String,
        val expiresAt: Instant?,
        /** 請求の猶予の終わり。[expiresAt] を過ぎてもこの時刻まで Pro。 */
        val graceExpiresAt: Instant?,
        val revokedAt: Instant?,
        /** `googlePlay`。App Store で買った時は無い。 */
        val platform: String?,
    ) {
        /** Pro でいられる終わり(猶予があればその終わり)。 */
        val activeUntil: Instant? get() = listOfNotNull(expiresAt, graceExpiresAt).maxOrNull()
    }

    companion object {
        private val json = Json { ignoreUnknownKeys = true }

        /** 読めなければ例外(呼び出し側で「読めない」にする)。 */
        fun decode(body: String): Entitlements {
            val o = json.parseToJsonElement(body).jsonObject
            val pro = (o["pro"] as? JsonObject)?.let { p ->
                fun s(k: String) = (p[k] as? JsonPrimitive)?.contentOrNull
                fun t(k: String) = s(k)?.let { runCatching { Instant.parse(it) }.getOrNull() }
                Pro(s("productId").orEmpty(), t("expiresAt"), t("graceExpiresAt"), t("revokedAt"), s("platform"))
            }
            val isPro = o["isPro"]?.jsonPrimitive?.booleanOrNull ?: throw IllegalArgumentException("isPro missing")
            return Entitlements(isPro, pro)
        }
    }
}

/** 権利と使っている量を読む口(テストで作り物に替える)。 */
interface AccountPlanApi {
    /** `GET /v1/me/entitlements` の本文。 */
    suspend fun entitlements(): String

    /** `GET /v1/me/usage`。 */
    suspend fun usage(): MyUsage
}

class HttpAccountPlanApi(private val http: MetamonHttp) : AccountPlanApi {
    override suspend fun entitlements(): String = http.send(HttpRequest("GET", listOf("v1", "me", "entitlements"), timeoutMillis = TIMEOUT))

    override suspend fun usage(): MyUsage = MyUsage.decode(http.send(HttpRequest("GET", listOf("v1", "me", "usage"), timeoutMillis = TIMEOUT)))

    private companion object {
        const val TIMEOUT = 20_000L
    }
}

/** 設定のアカウントの画面に出すプラン。 */
data class AccountPlan(
    val kind: Kind,
    /** Pro の終わり(猶予込み)。期限切れの時は切れた時刻。 */
    val until: Instant? = null,
    /** どこで買ったか。分からなければ null。 */
    val store: PurchaseStore? = null,
    /** Pro だったが期限が切れている・取り消された。 */
    val expired: Boolean = false,
    /** 今は読めず、前回読んだ値。 */
    val cached: Boolean = false,
    /** 作ったアプリの数(読めなければ null)。 */
    val usage: MyUsage? = null,
) {
    enum class Kind { PRO, FREE, UNKNOWN }

    enum class PurchaseStore { APP_STORE, GOOGLE_PLAY }

    companion object {
        fun of(e: Entitlements, cached: Boolean = false, usage: MyUsage? = null): AccountPlan {
            val p = e.pro
            val store = p?.let { if (it.platform == "googlePlay") PurchaseStore.GOOGLE_PLAY else PurchaseStore.APP_STORE }
            return when {
                e.isPro -> AccountPlan(Kind.PRO, p?.activeUntil, store, cached = cached, usage = usage)
                p != null -> AccountPlan(Kind.FREE, p.revokedAt ?: p.activeUntil, store, expired = true, cached = cached, usage = usage)
                else -> AccountPlan(Kind.FREE, cached = cached, usage = usage)
            }
        }
    }
}

/**
 * プランを読む(#2205)。読めたら端末に残し、読めない時は前回の値(Pro なら期限が過ぎていない時だけ)。
 * 残すのは uid ごと(別の人でサインインし直したら前の人の値は使わない)。
 */
class AccountPlanLoader(
    private val api: AccountPlanApi,
    private val store: KeyValueStore,
    private val now: () -> Instant = Instant::now,
) {
    suspend fun load(uid: String): AccountPlan {
        val usage = runCatching { api.usage() }.getOrNull()
        val fresh = runCatching {
            val body = api.entitlements()
            Entitlements.decode(body).also {
                store.putString(KEY_UID, uid)
                store.putString(KEY_BODY, body)
            }
        }.getOrNull()
        if (fresh != null) return AccountPlan.of(fresh, usage = usage)
        return cached(uid)?.let { AccountPlan.of(it, cached = true, usage = usage) } ?: AccountPlan(AccountPlan.Kind.UNKNOWN, usage = usage)
    }

    /** 前回の値。別の人の分・壊れた値・期限が過ぎた Pro は使わない。 */
    private fun cached(uid: String): Entitlements? {
        if (store.getString(KEY_UID) != uid) return null
        val e = store.getString(KEY_BODY)?.let { runCatching { Entitlements.decode(it) }.getOrNull() } ?: return null
        if (!e.isPro) return e
        val until = e.pro?.activeUntil ?: return null
        return e.takeIf { now().isBefore(until) }
    }

    /** サインアウト・端末のデータを消す時。 */
    fun forget() {
        store.remove(KEY_UID)
        store.remove(KEY_BODY)
    }

    companion object {
        const val KEY_UID = "entitlements.uid"
        const val KEY_BODY = "entitlements.body"
    }
}
