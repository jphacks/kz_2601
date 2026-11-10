package jp.kz2601.metamon.auth

import android.app.Activity
import jp.kz2601.metamon.services.AuthError
import jp.kz2601.metamon.services.AuthService
import jp.kz2601.metamon.services.AuthUser
import jp.kz2601.metamon.services.mock.MockAuthService
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/** 匿名 → Google / Apple の引き継ぎ(#1101・iOS の `AccountLinkTests`)。 */
class AccountLinkSessionTest {
    private class LinkAuth(private val linkFailure: Exception? = null, private val switchFailure: Exception? = null) : AuthService {
        override val kind = "fake"
        private val user = MutableStateFlow<AuthUser?>(AuthUser("anon", isAnonymous = true))
        override val currentUser: StateFlow<AuthUser?> = user
        val googleTokens = mutableListOf<String>()

        override suspend fun signInAnonymously() = error("unused")
        override suspend fun signInWithGoogle(googleIdToken: String) = error("unused")
        override suspend fun signInWithApple(activity: Activity) = error("unused")
        override suspend fun idToken(forceRefresh: Boolean): String? = null
        override suspend fun signOut() { user.value = null }
        override suspend fun deleteCurrentUser() { user.value = null }

        override suspend fun linkWithGoogle(googleIdToken: String): AuthUser {
            googleTokens += googleIdToken
            linkFailure?.let { throw it }
            return AuthUser("anon", isAnonymous = false, AuthUser.GOOGLE).also { user.value = it }
        }

        override suspend fun switchToPendingAccount(): AuthUser {
            switchFailure?.let { throw it }
            return AuthUser("existing", isAnonymous = false, AuthUser.GOOGLE).also { user.value = it }
        }
    }

    @Test
    fun linkKeepsUidAndClearsAnonymous() = runTest {
        val auth = LinkAuth()
        val session = AuthSession(auth)

        assertTrue(session.linkWithGoogle { "google-token" })

        assertEquals(listOf("google-token"), auth.googleTokens)
        assertEquals("anon", session.user.value?.uid)
        assertFalse(session.user.value!!.isAnonymous)
        assertNull(session.linkConflict.value)
        assertNull(session.linkError.value)
        assertFalse(session.isBusy.value)
    }

    @Test
    fun credentialInUseAsksToSwitchWithoutRedText() = runTest {
        val session = AuthSession(LinkAuth(linkFailure = AuthError.CredentialAlreadyInUse()))

        assertFalse(session.linkWithGoogle { "google-token" })

        assertEquals(AuthUser.GOOGLE, session.linkConflict.value)
        assertNull(session.linkError.value) // 確認ダイアログで伝える
        assertEquals("anon", session.user.value?.uid)

        assertTrue(session.switchToExistingAccount())
        assertEquals("existing", session.user.value?.uid)
        assertNull(session.linkConflict.value)
    }

    @Test
    fun dismissConflictStaysAnonymous() = runTest {
        val session = AuthSession(LinkAuth(linkFailure = AuthError.CredentialAlreadyInUse()))
        session.linkWithGoogle { "t" }
        session.dismissLinkConflict()

        assertNull(session.linkConflict.value)
        assertTrue(session.user.value!!.isAnonymous)
    }

    @Test
    fun switchWithoutPendingShowsError() = runTest {
        val session = AuthSession(LinkAuth(switchFailure = AuthError.NoPendingAccount()))

        assertFalse(session.switchToExistingAccount())
        assertEquals(LinkProblem.NoPendingAccount, session.linkError.value)
    }

    @Test
    fun alreadyLinkedAndOtherFailuresShowErrors() = runTest {
        val linked = AuthSession(LinkAuth(linkFailure = AuthError.AlreadyLinked()))
        assertFalse(linked.linkWithGoogle { "t" })
        assertEquals(LinkProblem.AlreadyLinked, linked.linkError.value)

        val down = AuthSession(LinkAuth(linkFailure = IllegalStateException("network down")))
        assertFalse(down.linkWithGoogle { "t" })
        assertEquals(LinkProblem.Failed, down.linkError.value)
        assertNull(down.linkConflict.value)
    }

    @Test
    fun cancelShowsNothing() = runTest {
        val session = AuthSession(LinkAuth())
        assertFalse(session.linkWithGoogle { throw AuthError.Canceled() })

        assertNull(session.linkError.value)
        assertNull(session.linkConflict.value)
        assertTrue(session.user.value!!.isAnonymous)
    }

    @Test
    fun mockLinkKeepsFixedUid() = runTest {
        val session = AuthSession(MockAuthService(signedIn = true))
        assertTrue(session.linkWithGoogle { "t" })
        assertEquals(MockAuthService.FIXED_UID, session.user.value?.uid)
        assertFalse(session.user.value!!.isAnonymous)
    }
}
