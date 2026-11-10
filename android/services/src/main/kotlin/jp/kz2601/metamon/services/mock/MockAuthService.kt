package jp.kz2601.metamon.services.mock

import jp.kz2601.metamon.services.AuthService
import jp.kz2601.metamon.services.AuthUser
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow

/** google-services.json が無い時の認証(固定 uid・トークン無し)。iOS の `MockAuthService` と同じ。 */
class MockAuthService(signedIn: Boolean = true) : AuthService {
    override val kind: String = "mock"

    private val user = MutableStateFlow(if (signedIn) fixedUser else null)
    override val currentUser: StateFlow<AuthUser?> = user

    override suspend fun signInAnonymously(): AuthUser = fixedUser.also { user.value = it }

    override suspend fun idToken(forceRefresh: Boolean): String? = null

    override suspend fun signOut() {
        user.value = null
    }

    companion object {
        const val FIXED_UID = "mock-user"
        private val fixedUser = AuthUser(uid = FIXED_UID, isAnonymous = true)
    }
}
