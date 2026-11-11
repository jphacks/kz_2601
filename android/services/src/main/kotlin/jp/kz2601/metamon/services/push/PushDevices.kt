package jp.kz2601.metamon.services.push

import android.content.SharedPreferences
import kotlinx.serialization.Serializable

/**
 * `PUT /v1/me/devices/{installationId}` の本文(iOS の `PushDeviceRegistration`)。
 * Android は FCM のトークンなので `platform: "android"` を付ける。
 * 今の backend は APNs の 16 進のトークンと platform 無しだけを受ける(#918 で android を足す)。
 */
@Serializable
data class PushDeviceRegistration(
    val token: String,
    /** FCM に sandbox / production の区別は無い。backend の必須項目なので production を送る */
    val environment: String = "production",
    val platform: String = PLATFORM,
    val appVersion: String,
    val os: String,
) {
    companion object {
        const val PLATFORM = "android"
    }
}

/** 端末の登録と、他の人の行動で届く通知の同意(#269・#339)。 */
interface PushDeviceClient {
    suspend fun putDevice(installationId: String, registration: PushDeviceRegistration)
    suspend fun deleteDevice(installationId: String)
    suspend fun setNotifyConsent(appId: String, allowed: Boolean)
}

/** 登録の控えの置き場。テストではメモリ、端末では SharedPreferences。 */
interface PushPrefs {
    fun getString(key: String): String?
    fun putString(key: String, value: String)
    fun remove(key: String)
}

class SharedPreferencesPushPrefs(private val prefs: SharedPreferences) : PushPrefs {
    override fun getString(key: String): String? = prefs.getString(key, null)
    override fun putString(key: String, value: String) = prefs.edit().putString(key, value).apply()
    override fun remove(key: String) = prefs.edit().remove(key).apply()
}

class InMemoryPushPrefs : PushPrefs {
    val values = mutableMapOf<String, String>()
    override fun getString(key: String): String? = synchronized(values) { values[key] }
    override fun putString(key: String, value: String) = synchronized(values) { values[key] = value }
    override fun remove(key: String) { synchronized(values) { values.remove(key) } }
}

/**
 * 同意の控え(uid + appId ごと・iOS の `NotifyConsentStore`)。読む API が無いので、この端末で変えた値を覚える。
 * 保存が無い = 受け取らない(#339)。
 */
class NotifyConsentStore(private val prefs: PushPrefs) {
    fun allowed(uid: String, appId: String): Boolean = prefs.getString(key(uid, appId)) == "1"

    fun set(allowed: Boolean, uid: String, appId: String) = prefs.putString(key(uid, appId), if (allowed) "1" else "0")

    private fun key(uid: String, appId: String) = "metamon.push.notifyConsent.$uid.$appId"
}

/**
 * FCM の data の読み方。runner の APNs の本文(`appId`・title・body・アプリ名)と同じキーを使う前提(#918 で決める)。
 * notification つきのメッセージは、その title / body を先に使う。
 */
data class PushMessage(val appId: String?, val title: String, val body: String) {
    /** 出す中身が無い(data だけの合図など)。 */
    val isEmpty: Boolean get() = title.isEmpty() && body.isEmpty()

    companion object {
        private val APP_ID = Regex("^[A-Za-z0-9_.:-]{1,128}$")

        /** タップで開くアプリ。形が backend の appId でなければ null。 */
        fun appId(raw: String?): String? = raw?.trim()?.takeIf { APP_ID.matches(it) }

        fun from(data: Map<String, String>, notificationTitle: String? = null, notificationBody: String? = null): PushMessage {
            val appName = data["appName"].orEmpty().trim()
            val title = notificationTitle?.trim().orEmpty().ifEmpty { data["title"].orEmpty().trim() }
            val body = notificationBody?.trim().orEmpty().ifEmpty { data["body"].orEmpty().trim() }
            return PushMessage(
                appId = appId(data["appId"]),
                // 題が無ければアプリ名を題にする(iOS はアプリ名を subtitle に出す)
                title = title.ifEmpty { appName },
                body = body,
            )
        }
    }
}
