package jp.kz2601.metamon.services.account

import android.app.Activity
import jp.kz2601.metamon.services.AuthError
import jp.kz2601.metamon.services.AuthService
import jp.kz2601.metamon.services.AuthUser
import jp.kz2601.metamon.services.local.AccountOwnerStore
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.fail
import org.junit.Rule
import org.junit.Test
import org.junit.rules.TemporaryFolder
import java.io.IOException

/** アカウント削除とログアウトの順番(#841・iOS の AccountDeletionFlowTests / SignOutFlowTests)。 */
class AccountFlowsTest {
    @get:Rule
    val tmp = TemporaryFolder()

    private class Auth(private val recentLoginFailures: Int = 0) : AuthService {
        override val kind = "fake"
        override val currentUser: StateFlow<AuthUser?> = MutableStateFlow(AuthUser("u1", isAnonymous = false))
        val log = mutableListOf<String>()
        private var failures = recentLoginFailures

        override suspend fun signInAnonymously(): AuthUser = error("unused")
        override suspend fun signInWithGoogle(googleIdToken: String): AuthUser = error("unused")
        override suspend fun signInWithApple(activity: Activity): AuthUser = error("unused")
        override suspend fun idToken(forceRefresh: Boolean): String? = "t"
        override suspend fun signOut() {
            log += "signOut"
        }

        override suspend fun deleteCurrentUser() {
            log += "deleteUser"
            if (failures-- > 0) throw AuthError.RequiresRecentLogin()
        }
    }

    private fun account(log: MutableList<String>, fail: Boolean = false) = object : AccountClient {
        override suspend fun deleteAccount() {
            log += "server"
            if (fail) throw IOException("offline")
        }
    }

    @Test
    fun deletionRunsServerThenFirebaseThenWipeThenSignOut() = runTest {
        val auth = Auth()
        AccountDeletionFlow.run(account(auth.log), auth, reauthenticate = { error("not needed") }, wipeLocal = { auth.log += "wipe" })
        assertEquals(listOf("server", "deleteUser", "wipe", "signOut"), auth.log)
    }

    @Test
    fun deletionStopsBeforeTouchingAnythingWhenTheServerFails() = runTest {
        val auth = Auth()
        try {
            AccountDeletionFlow.run(account(auth.log, fail = true), auth, reauthenticate = { true }, wipeLocal = { auth.log += "wipe" })
            fail("throw する")
        } catch (_: IOException) {
        }
        assertEquals(listOf("server"), auth.log)
    }

    @Test
    fun deletionReauthenticatesWhenFirebaseAsksForARecentLogin() = runTest {
        val auth = Auth(recentLoginFailures = 1)
        AccountDeletionFlow.run(
            account(auth.log), auth,
            reauthenticate = { auth.log += "reauth"; true },
            wipeLocal = { auth.log += "wipe" },
        )
        assertEquals(listOf("server", "deleteUser", "reauth", "deleteUser", "wipe", "signOut"), auth.log)
    }

    @Test
    fun deletionStillWipesWhenReauthenticationIsCanceled() = runTest {
        // サーバーは削除済みなので、Firebase のユーザーが残っても端末は消してサインアウトする
        val auth = Auth(recentLoginFailures = 1)
        AccountDeletionFlow.run(account(auth.log), auth, reauthenticate = { false }, wipeLocal = { auth.log += "wipe" }, log = {})
        assertEquals(listOf("server", "deleteUser", "wipe", "signOut"), auth.log)
    }

    @Test
    fun signOutPromptCountsUnsentRecords() {
        assertEquals(SignOutFlow.Prompt.Plain, SignOutFlow.prompt(pending = 0, isOnline = false))
        assertEquals(SignOutFlow.Prompt.Unsent(3, canSend = true), SignOutFlow.prompt(pending = 3, isOnline = true))
        assertEquals(SignOutFlow.Prompt.Unsent(3, canSend = false), SignOutFlow.prompt(pending = 3, isOnline = false))
    }

    @Test
    fun sendThenWipeKeepsEverythingWhenRecordsAreLeft() = runTest {
        val auth = Auth()
        val outcome = SignOutFlow.run(SignOutFlow.Choice.SEND_THEN_WIPE, auth, flush = { 2 }, wipeLocal = { auth.log += "wipe" })
        assertEquals(SignOutFlow.Outcome.Unsent(2), outcome)
        assertEquals(emptyList<String>(), auth.log)
    }

    @Test
    fun sendThenWipeSignsOutWhenEverythingWasSent() = runTest {
        val auth = Auth()
        val outcome = SignOutFlow.run(SignOutFlow.Choice.SEND_THEN_WIPE, auth, flush = { 0 }, wipeLocal = { auth.log += "wipe" })
        assertEquals(SignOutFlow.Outcome.SignedOut, outcome)
        assertEquals(listOf("wipe", "signOut"), auth.log)
    }

    @Test
    fun discardWipesWithoutFlushing() = runTest {
        val auth = Auth()
        val outcome = SignOutFlow.run(SignOutFlow.Choice.DISCARD_AND_WIPE, auth, flush = { error("送らない") }, wipeLocal = { auth.log += "wipe" })
        assertEquals(SignOutFlow.Outcome.SignedOut, outcome)
        assertEquals(listOf("wipe", "signOut"), auth.log)
    }

    @Test
    fun ownerStoreSurvivesARelaunchAndForgets() {
        val dir = tmp.newFolder("metamon-account")
        AccountOwnerStore(dir).uid = "uid-a"
        assertEquals("uid-a", AccountOwnerStore(dir).uid)
        AccountOwnerStore(dir).uid = null
        assertNull(AccountOwnerStore(dir).uid)
    }
}
