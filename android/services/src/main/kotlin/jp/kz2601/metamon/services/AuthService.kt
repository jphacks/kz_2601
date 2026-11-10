package jp.kz2601.metamon.services

import android.app.Activity
import kotlinx.coroutines.flow.StateFlow

/**
 * サインイン中の利用者。
 * @param provider 入った方法(`google.com` / `apple.com`)。匿名・モックは null。アカウント削除の再認証に使う
 */
data class AuthUser(val uid: String, val isAnonymous: Boolean, val provider: String? = null) {
    companion object {
        const val GOOGLE = "google.com"
        const val APPLE = "apple.com"
    }
}

/** サインインの失敗(画面に出す文言の元)。 */
sealed class AuthError(message: String, cause: Throwable? = null) : Exception(message, cause) {
    /** 利用者が自分で閉じた。画面にエラーは出さない */
    class Canceled(cause: Throwable? = null) : AuthError("canceled", cause)

    /** この端末・ビルドでは使えない方法(Google の Web クライアント ID が無い等) */
    class Unavailable(message: String) : AuthError(message)

    /** 削除の前に入り直しが要る(Firebase の requires-recent-login)。 */
    class RequiresRecentLogin(cause: Throwable? = null) : AuthError("requires recent login", cause)
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

    /**
     * 今の利用者を Firebase から消す(アカウント削除 #841)。入ってから時間が経っていれば [AuthError.RequiresRecentLogin]。
     * 消せたら Firebase 側はサインアウト済みになる。
     */
    suspend fun deleteCurrentUser()

    /** 削除の前の入り直し(Google)。Credential Manager で取り直した ID トークンを渡す。 */
    suspend fun reauthenticateWithGoogle(googleIdToken: String) {
        throw AuthError.Unavailable("reauthenticate is not supported")
    }

    /** 削除の前の入り直し(Apple・Custom Tab)。 */
    suspend fun reauthenticateWithApple(activity: Activity) {
        throw AuthError.Unavailable("reauthenticate is not supported")
    }
}
