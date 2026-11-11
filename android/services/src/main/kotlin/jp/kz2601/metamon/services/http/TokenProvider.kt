package jp.kz2601.metamon.services.http

import jp.kz2601.metamon.services.AuthService

/** Bearer の供給(iOS の `TokenProvider`)。null なら Authorization を付けない。 */
fun interface TokenProvider {
    suspend fun idToken(forceRefresh: Boolean): String?
}

/** [AuthService] の ID トークンを Bearer にする(iOS の `AuthTokenProvider`)。 */
class AuthTokenProvider(private val auth: AuthService) : TokenProvider {
    override suspend fun idToken(forceRefresh: Boolean): String? = auth.idToken(forceRefresh)
}

/**
 * App Check のトークン(#318)。取れなければ null(ヘッダ無しで送る)。throw しない = App Check の不調で通信を止めない。
 * 実装は [jp.kz2601.metamon.services.live.FirebaseAppCheckTokenProvider](release は Play Integrity・debug は debug provider)。
 */
fun interface AppCheckTokenProvider {
    /** `forceRefresh = true` はサーバーに 401 `app_check_failed` で弾かれた後の取り直し */
    suspend fun token(forceRefresh: Boolean): String?
}

/** App Check のヘッダとエラーコード(backend `internal/appcheck` と同じ値)。 */
object AppCheckHeader {
    const val NAME = "X-Firebase-AppCheck"
    const val FAILED_CODE = ErrorCodes.APP_CHECK_FAILED
}
