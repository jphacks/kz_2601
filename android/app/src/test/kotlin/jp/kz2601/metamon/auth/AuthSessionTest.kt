package jp.kz2601.metamon.auth

import android.app.Activity
import jp.kz2601.metamon.AppContainer
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
import org.junit.Assert.assertSame
import org.junit.Assert.assertTrue
import org.junit.Test

/** サインイン状態での画面の出し分けと、モックへの落とし方(#840)。 */
class AuthSessionTest {
    @Test
    fun signedOutShowsSignInScreen() = runTest {
        val session = AuthSession(MockAuthService(signedIn = false))
        assertEquals(RootScreen.SignIn, RootScreen.of(session.user.value))

        session.signInAnonymously()
        assertEquals(RootScreen.App, RootScreen.of(session.user.value))
        assertEquals(MockAuthService.FIXED_UID, session.user.value?.uid)

        session.signOut()
        assertEquals(RootScreen.SignIn, RootScreen.of(session.user.value))
    }

    @Test
    fun signOutUnregistersPushFirstEvenIfItFails() = runTest {
        val auth = MockAuthService(signedIn = true)
        val calls = mutableListOf<String?>()
        val session = AuthSession(auth, beforeSignOut = {
            calls += auth.currentUser.value?.uid
            error("network down")
        })

        session.signOut()

        // 登録を消す時はまだサインイン中(uid がある)。消せなくてもサインアウトする
        assertEquals(listOf(MockAuthService.FIXED_UID), calls)
        assertEquals(RootScreen.SignIn, RootScreen.of(session.user.value))
    }

    @Test
    fun googleIdTokenIsPassedToAuthService() = runTest {
        val auth = FakeAuth()
        val session = AuthSession(auth)
        session.signInWithGoogle { "google-id-token" }

        assertEquals(listOf("google-id-token"), auth.googleTokens)
        assertEquals(RootScreen.App, RootScreen.of(session.user.value))
        assertNull(session.errorMessage.value)
        assertFalse(session.isBusy.value)
    }

    @Test
    fun cancelShowsNoError() = runTest {
        val session = AuthSession(FakeAuth())
        session.signInWithGoogle { throw AuthError.Canceled() }

        assertNull(session.errorMessage.value)
        assertEquals(RootScreen.SignIn, RootScreen.of(session.user.value))
    }

    @Test
    fun failureShowsMessageAndStaysOnSignIn() = runTest {
        val session = AuthSession(FakeAuth(failWith = IllegalStateException("network down")))
        session.signInAnonymously()

        assertEquals("network down", session.errorMessage.value)
        assertEquals(RootScreen.SignIn, RootScreen.of(session.user.value))
        assertFalse(session.isBusy.value)
    }

    @Test
    fun noFirebaseFallsBackToSignedInMock() {
        var firebaseAsked = false
        val auth = AppContainer.chooseAuth(firebaseConfigured = false) { firebaseAsked = true; FakeAuth() }
        assertTrue(auth is MockAuthService)
        assertFalse(firebaseAsked)
        // json が無い環境でもサインイン画面で止まらず、固定 uid で本体に入る
        assertEquals(RootScreen.App, RootScreen.of(auth.currentUser.value))
    }

    @Test
    fun firebaseUsedWhenConfigured() {
        val firebase = FakeAuth()
        assertSame(firebase, AppContainer.chooseAuth(firebaseConfigured = true) { firebase })
        // 構成されていても作れなければモック
        assertTrue(AppContainer.chooseAuth(firebaseConfigured = true) { null } is MockAuthService)
    }

    @Test
    fun webClientIdPrefersBuildConfigThenResource() {
        assertEquals("from-gradle", GoogleWebClientId.resolve("from-gradle") { "from-json" })
        assertEquals("from-json", GoogleWebClientId.resolve("") { "from-json" })
        assertNull(GoogleWebClientId.resolve("") { null })
        assertNull(GoogleWebClientId.resolve(" ") { "" })
    }
}

private class FakeAuth(private val failWith: Exception? = null) : AuthService {
    override val kind = "fake"
    private val user = MutableStateFlow<AuthUser?>(null)
    override val currentUser: StateFlow<AuthUser?> = user
    val googleTokens = mutableListOf<String>()

    private fun signIn(uid: String, anonymous: Boolean): AuthUser {
        failWith?.let { throw it }
        return AuthUser(uid, anonymous).also { user.value = it }
    }

    override suspend fun signInAnonymously() = signIn("anon", anonymous = true)

    override suspend fun signInWithGoogle(googleIdToken: String): AuthUser {
        googleTokens += googleIdToken
        return signIn("google", anonymous = false)
    }

    override suspend fun signInWithApple(activity: Activity) = signIn("apple", anonymous = false)

    override suspend fun idToken(forceRefresh: Boolean): String? = user.value?.let { "id-${it.uid}" }

    override suspend fun signOut() {
        user.value = null
    }
}
