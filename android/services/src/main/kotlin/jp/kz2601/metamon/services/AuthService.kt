package jp.kz2601.metamon.services

import kotlinx.coroutines.flow.StateFlow

/** サインイン中の利用者。 */
data class AuthUser(val uid: String, val isAnonymous: Boolean)

/** 認証(iOS の `AuthService`)。実装は Firebase([FirebaseAuthService])かモック([MockAuthService])。 */
interface AuthService {
    /** 認証の実装の名前(画面・ログ用)。 */
    val kind: String

    val currentUser: StateFlow<AuthUser?>

    suspend fun signInAnonymously(): AuthUser

    /** backend に付ける ID トークン。モックや未サインインなら null(Authorization ヘッダを付けない)。 */
    suspend fun idToken(forceRefresh: Boolean = false): String?

    suspend fun signOut()
}
