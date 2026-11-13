package jp.kz2601.metamon.account

import jp.kz2601.metamon.auth.AuthSession
import jp.kz2601.metamon.services.AuthUser
import jp.kz2601.metamon.services.account.AccountClient
import jp.kz2601.metamon.services.mock.MockAuthService
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.IOException

/**
 * アカウント削除の前に Apple のトークンを取り消す(#1165・iOS の `PrivacyConsentTests` の失効の確かめと同じ内容)。
 * 入り直しで取れたトークンが取り消しに渡ること・失敗しても削除が続くこと・Apple 以外では何もしないこと。
 */
class AppleTokenRevocationTest {
    private val logs = mutableListOf<String>()

    private suspend fun run(
        provider: String?,
        reauthenticate: suspend () -> String?,
        revoke: suspend (String) -> Unit,
    ) = AppleTokenRevocation.run(provider, reauthenticate, revoke, log = { logs += it })

    @Test
    fun revokesTheTokenFromTheReauthentication() = runTest {
        val revoked = mutableListOf<String>()

        val outcome = run(AuthUser.APPLE, reauthenticate = { "apple-access-token" }, revoke = { revoked += it })

        assertEquals(AppleTokenRevocation.Outcome(reauthenticated = true, revoked = true), outcome)
        assertEquals(listOf("apple-access-token"), revoked)
        assertTrue(logs.isEmpty())
    }

    @Test
    fun doesNothingForGoogleOrAnonymous() = runTest {
        var asked = false
        val ask: suspend () -> String? = { asked = true; "t" }

        assertEquals(AppleTokenRevocation.Outcome.NotApple, run(AuthUser.GOOGLE, ask, revoke = { fail() }))
        assertEquals(AppleTokenRevocation.Outcome.NotApple, run(null, ask, revoke = { fail() }))

        assertFalse(asked) // 匿名・Google に Apple の画面を出さない
        assertTrue(logs.isEmpty())
    }

    @Test
    fun failedReauthenticationIsLoggedAndSwallowed() = runTest {
        val outcome = run(AuthUser.APPLE, reauthenticate = { throw IOException("offline") }, revoke = { fail() })

        assertEquals(AppleTokenRevocation.Outcome(), outcome)
        assertEquals(1, logs.size)
        assertTrue(logs.single().startsWith(AppleTokenRevocation.REAUTH_FAILED))
    }

    @Test
    fun missingTokenIsLoggedAndSkipsTheRevocation() = runTest {
        val outcome = run(AuthUser.APPLE, reauthenticate = { null }, revoke = { fail() })

        assertEquals(AppleTokenRevocation.Outcome(), outcome)
        assertEquals(listOf(AppleTokenRevocation.NO_TOKEN), logs)
    }

    @Test
    fun failedRevocationIsLoggedAndSwallowed() = runTest {
        // Firebase の Apple に OAuth コードフローの構成が無い時(審査前の人の作業が済むまでここを通る)
        val outcome = run(AuthUser.APPLE, reauthenticate = { "t" }, revoke = { throw IllegalStateException("no code flow") })

        assertEquals(AppleTokenRevocation.Outcome(reauthenticated = true), outcome)
        assertEquals(1, logs.size)
        assertTrue(logs.single().startsWith(AppleTokenRevocation.REVOKE_FAILED))
    }

    @Test
    fun cancellationIsNotSwallowed() = runTest {
        val fromReauth = runCatching { run(AuthUser.APPLE, { throw CancellationException("stop") }, revoke = { fail() }) }
        val fromRevoke = runCatching { run(AuthUser.APPLE, { "t" }, revoke = { throw CancellationException("stop") }) }

        assertTrue(fromReauth.exceptionOrNull() is CancellationException)
        assertTrue(fromRevoke.exceptionOrNull() is CancellationException)
    }

    @Test
    fun revocationRunsBeforeTheServerAndFirebaseDeletion() = runTest {
        val order = mutableListOf<String>()
        val auth = MockAuthService(signedIn = true)
        val account = object : AccountClient {
            override suspend fun deleteAccount() {
                order += "server"
            }
        }

        AuthSession(auth).deleteAccount(
            account,
            reauthenticate = { false },
            wipe = { order += "wipe" },
            revokeAppleToken = { order += "revoke" },
        )

        assertEquals(listOf("revoke", "server", "wipe"), order)
    }

    @Test
    fun deletionContinuesWhenTheRevocationThrows() = runTest {
        var serverDeleted = false
        val auth = MockAuthService(signedIn = true)

        AuthSession(auth).deleteAccount(
            object : AccountClient {
                override suspend fun deleteAccount() {
                    serverDeleted = true
                }
            },
            reauthenticate = { false },
            wipe = {},
            revokeAppleToken = { throw IllegalStateException("revoke blew up") },
        )

        assertTrue(serverDeleted)
    }

    private fun fail(): Nothing = throw AssertionError("取り消しを呼んではいけない")
}
