package jp.kz2601.metamon.services.live

import android.util.Log
import com.google.firebase.appcheck.FirebaseAppCheck
import jp.kz2601.metamon.services.http.AppCheckTokenProvider
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.withTimeoutOrNull

/**
 * `FirebaseAppCheck` からトークンを取る(iOS の `FirebaseAppCheckTokenProvider`・#318)。
 * SDK がキャッシュと期限前の更新を持つので毎リクエスト呼んでよい。初回の Play Integrity は往復があるので、
 * [timeoutMillis] を超えたら null(ヘッダ無し)で先に進む。失敗しても throw しない。
 */
class FirebaseAppCheckTokenProvider internal constructor(
    private val fetch: suspend (Boolean) -> String,
    private val timeoutMillis: Long = 5_000,
    private val log: (String) -> Unit = { Log.w("Metamon", it) },
) : AppCheckTokenProvider {
    override suspend fun token(forceRefresh: Boolean): String? = try {
        withTimeoutOrNull(timeoutMillis) { fetch(forceRefresh) }
            .also { if (it == null) log("App Check: token timed out") }
    } catch (e: CancellationException) {
        throw e
    } catch (e: Exception) {
        log("App Check: token unavailable: ${e.message}")
        null
    }

    companion object {
        /** Firebase が未初期化なら null(App Check のヘッダを付けない)。 */
        fun createIfConfigured(timeoutMillis: Long = 5_000): FirebaseAppCheckTokenProvider? {
            if (!FirebaseBootstrap.isConfigured) return null
            val appCheck = FirebaseAppCheck.getInstance()
            return FirebaseAppCheckTokenProvider(
                fetch = { force -> appCheck.getAppCheckToken(force).await().token },
                timeoutMillis = timeoutMillis,
            )
        }
    }
}
