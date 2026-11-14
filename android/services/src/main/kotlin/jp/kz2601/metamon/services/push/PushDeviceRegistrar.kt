package jp.kz2601.metamon.services.push

import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import kotlinx.coroutines.withTimeoutOrNull
import java.security.MessageDigest
import java.util.UUID
import kotlin.coroutines.cancellation.CancellationException

/**
 * 端末トークンの登録(#269・iOS の `PushDeviceRegistrar`)。
 * - 変わった時だけ送る(uid・installationId・トークン・版の指紋を覚える。トークンそのものは残さない)
 * - 送れなければ覚えないので、次に前面へ来た時の [register] で送り直す
 * - [unregister] はサインアウト・退会の前。送れなくても [timeoutMillis] で戻る
 */
class PushDeviceRegistrar(
    private val client: PushDeviceClient,
    private val prefs: PushPrefs,
    private val appVersion: String,
    private val os: String,
    private val log: (String) -> Unit = {},
    private val newId: () -> String = { UUID.randomUUID().toString() },
) {
    private val mutex = Mutex()

    @Volatile
    private var unregistering = false

    /** @return 送った(または送り済みだった)なら true */
    suspend fun register(token: String, uid: String): Boolean {
        if (unregistering || token.isBlank()) return false
        return mutex.withLock {
            if (unregistering) return@withLock false
            val id = installationId()
            val registration = PushDeviceRegistration(token = token, appVersion = appVersion, os = os)
            val fingerprint = fingerprint(uid, id, registration)
            if (prefs.getString(LAST_SENT_KEY) == fingerprint) return@withLock true
            val sent = try {
                client.putDevice(id, registration)
                true
            } catch (e: CancellationException) {
                throw e
            } catch (e: Exception) {
                log("push: device not registered (${e.javaClass.simpleName})")
                false
            }
            if (sent && !unregistering) prefs.putString(LAST_SENT_KEY, fingerprint)
            sent
        }
    }

    suspend fun unregister(timeoutMillis: Long = 3_000) {
        unregistering = true
        try {
            // 退会を取りやめた時は次の前面で登録し直す
            prefs.remove(LAST_SENT_KEY)
            val id = prefs.getString(INSTALLATION_ID_KEY) ?: return // 登録したことが無い
            withTimeoutOrNull(timeoutMillis) {
                // 送信中の登録が終わってから消す(消した後に登録が届かないように)
                mutex.withLock {
                    try {
                        client.deleteDevice(id)
                    } catch (e: CancellationException) {
                        throw e
                    } catch (e: Exception) {
                        log("push: device not deleted (${e.javaClass.simpleName})")
                    }
                }
            }
        } finally {
            unregistering = false
        }
    }

    /** 通知の許可が切られた: 登録済みの時だけ消す。 */
    suspend fun authorizationRevoked() {
        if (prefs.getString(LAST_SENT_KEY) != null) unregister()
    }

    val isRegistered: Boolean get() = prefs.getString(LAST_SENT_KEY) != null

    /** 端末で 1 つ。サインアウトでも変えない。 */
    private fun installationId(): String =
        prefs.getString(INSTALLATION_ID_KEY) ?: newId().also { prefs.putString(INSTALLATION_ID_KEY, it) }

    companion object {
        const val INSTALLATION_ID_KEY = "metamon.push.installationId"
        const val LAST_SENT_KEY = "metamon.push.lastSent"

        internal fun fingerprint(uid: String, id: String, r: PushDeviceRegistration): String {
            val text = listOf(uid, id, r.token, r.environment, r.platform, r.appVersion, r.os).joinToString("\n")
            return MessageDigest.getInstance("SHA-256").digest(text.toByteArray()).joinToString("") { "%02x".format(it) }
        }
    }
}
