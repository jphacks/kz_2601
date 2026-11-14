package jp.kz2601.metamon.auth

import android.app.Activity
import jp.kz2601.metamon.account.AccountGuard
import jp.kz2601.metamon.account.ActiveOwner
import jp.kz2601.metamon.services.AuthService
import jp.kz2601.metamon.services.AuthUser
import jp.kz2601.metamon.services.http.SessionExpirySignal
import jp.kz2601.metamon.services.local.AccountOwnerStore
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.launch
import kotlinx.coroutines.test.TestScope
import kotlinx.coroutines.test.runCurrent
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

/** トークンを取り直しても 401(#1102): 1 回だけサインアウトし、端末のデータを消して、サインイン画面に理由を出す。 */
class SessionExpiryTest {
    private class FakeAuth(initial: AuthUser?) : AuthService {
        override val kind = "fake"
        val user = MutableStateFlow(initial)
        override val currentUser: StateFlow<AuthUser?> = user
        var signOuts = 0

        override suspend fun signInAnonymously(): AuthUser = AuthUser("uid-a", isAnonymous = true).also { user.value = it }
        override suspend fun signInWithGoogle(googleIdToken: String): AuthUser = error("unused")
        override suspend fun signInWithApple(activity: Activity): AuthUser = error("unused")
        override suspend fun idToken(forceRefresh: Boolean): String? = user.value?.let { "id-${it.uid}" }
        override suspend fun signOut() {
            signOuts++
            user.value = null
        }

        override suspend fun deleteCurrentUser() {
            user.value = null
        }
    }

    private class Fixture(val auth: FakeAuth, val session: AuthSession, val guard: AccountGuard) {
        var wipes = 0
        var unregisters = 0
    }

    private fun TestScope.fixture(beforeSignOut: suspend () -> Unit = {}): Fixture {
        val auth = FakeAuth(AuthUser("uid-a", isAnonymous = true))
        lateinit var f: Fixture
        val session = AuthSession(auth, beforeSignOut = {
            f.unregisters++
            beforeSignOut()
        })
        val guard = AccountGuard(
            auth, AccountOwnerStore(null), ActiveOwner(),
            startSync = {}, stopSync = {}, wipe = { f.wipes++ },
        )
        f = Fixture(auth, session, guard)
        guard.start(backgroundScope)
        runCurrent()
        return f
    }

    @Test
    fun expirySignsOutOnceWipesAndShowsTheReason() = runTest {
        val f = fixture()

        f.session.expire("uid-a")
        runCurrent()

        assertEquals(1, f.auth.signOuts)
        assertEquals(1, f.unregisters)
        assertEquals("通常のサインアウトと同じく端末のデータを消す", 1, f.wipes)
        assertEquals(RootScreen.SignIn, RootScreen.of(f.session.user.value))
        assertNull(f.guard.active.value)
        assertEquals(AuthSession.SESSION_EXPIRED, f.session.errorMessage.value)
    }

    @Test
    fun concurrentExpiriesSignOutOnlyOnce() = runTest {
        val gate = CompletableDeferred<Unit>()
        val f = fixture(beforeSignOut = { gate.await() })

        // 同時に走っていた複数のリクエストが続けて 401 になった
        repeat(3) { launch { f.session.expire("uid-a") } }
        runCurrent()
        gate.complete(Unit)
        runCurrent()
        // サインイン画面に戻った後に遅れて届いた 401
        f.session.expire("uid-a")
        runCurrent()

        assertEquals(1, f.auth.signOuts)
        assertEquals(1, f.unregisters)
        assertEquals(1, f.wipes)
        assertEquals(AuthSession.SESSION_EXPIRED, f.session.errorMessage.value)
    }

    @Test
    fun signingInAgainClearsTheReasonAndANewExpiryWorksAgain() = runTest {
        val f = fixture()
        f.session.expire("uid-a")
        runCurrent()

        f.session.signInAnonymously()
        runCurrent()
        assertNull("入り直したら理由は消える", f.session.errorMessage.value)

        f.session.expire("uid-a")
        runCurrent()
        assertEquals(2, f.auth.signOuts)
        assertEquals(AuthSession.SESSION_EXPIRED, f.session.errorMessage.value)
    }

    @Test
    fun signalFromHttpReachesTheSession() = runTest {
        val f = fixture()
        val signal = SessionExpirySignal()
        signal.listener = { uid -> backgroundScope.launch { f.session.expire(uid) } }

        signal.fire("uid-a")
        signal.fire("uid-a")
        runCurrent()

        assertEquals(1, f.auth.signOuts)
        assertEquals(RootScreen.SignIn, RootScreen.of(f.session.user.value))
    }

    @Test
    fun lateExpiryFromThePreviousUserDoesNotSignOutTheNextOne() = runTest {
        val f = fixture()
        // uid-a がサインアウトして uid-b が入った後に、uid-a のトークンで送ったリクエストの 401 が届いた
        f.session.signOut()
        f.auth.user.value = AuthUser("uid-b", isAnonymous = false)
        runCurrent()

        f.session.expire("uid-a")
        f.session.expire(null)
        runCurrent()

        assertEquals(1, f.auth.signOuts)
        assertEquals("uid-b", f.session.user.value?.uid)
        assertEquals(RootScreen.App, RootScreen.of(f.session.user.value))
        assertNull(f.session.errorMessage.value)

        // uid-b 自身の 401 は今まで通り 1 回だけサインアウトする
        repeat(2) { launch { f.session.expire("uid-b") } }
        runCurrent()
        assertEquals(2, f.auth.signOuts)
        assertEquals(AuthSession.SESSION_EXPIRED, f.session.errorMessage.value)
    }
}
