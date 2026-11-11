package jp.kz2601.metamon.services

import android.app.Activity
import kotlinx.coroutines.flow.StateFlow

/** サインイン中の利用者。 */
data class AuthUser(val uid: String, val isAnonymous: Boolean)

/** サインインの失敗(画面に出す文言の元)。 */
sealed class AuthError(message: String, cause: Throwable? = null) : Exception(message, cause) {
    /** 利用者が自分で閉じた。画面にエラーは出さない */
    class Canceled(cause: Throwable? = null) : AuthError("canceled", cause)

    /** この端末・ビルドでは使えない方法(Google の Web クライアント ID が無い等) */
    class Unavailable(message: String) : AuthError(message)
}

/** 認証(iOS の `AuthService`)。実装は Firebase([FirebaseAuthService])かモック([MockAuthService])。 */
interface AuthService {
    /** 認証の実装の名前(画面・ログ用)。 */
    val kind: String

    val currentUser: StateFlow<AuthUser?>

    suspend fun signInAnonymously(): AuthUser

    /** Credential Manager で取った Google の ID トークンで入る。 */
    suspend fun signInWithGoogle(googleIdToken: String): AuthUser

    /** Firebase の `OAuthProvider("apple.com")`(Custom Tab で Apple のページを開く・#361)。 */
    suspend fun signInWithApple(activity: Activity): AuthUser

    /** backend に付ける ID トークン。モックや未サインインなら null(Authorization ヘッダを付けない)。 */
    suspend fun idToken(forceRefresh: Boolean = false): String?

    suspend fun signOut()
}
