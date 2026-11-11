package jp.kz2601.metamon.services

import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertSame
import org.junit.Assert.assertTrue
import org.junit.Assert.fail
import org.junit.Test

/** 匿名 → Google / Apple の引き継ぎ(#1101)の分岐。Firebase の代わりに errorCode 付きの例外を投げる。 */
class AccountLinkerTest {
    private class FirebaseLike(val code: String, val updated: String? = null) : Exception(code)

    private val anon = AuthUser("anon-uid", isAnonymous = true)

    private fun linker(current: AuthUser?, signedInWith: MutableList<String> = mutableListOf()) = AccountLinker<String>(
        currentUser = { current },
        classify = { e -> (e as? FirebaseLike).let { AccountLinker.classifyFirebase(it?.code, it?.updated) } },
        signInWith = { signedInWith += it; AuthUser("existing-uid", isAnonymous = false, AuthUser.GOOGLE) },
    )

    private suspend inline fun <reified T : Throwable> assertThrows(crossinline body: suspend () -> Unit): T {
        try {
            body()
        } catch (e: Throwable) {
            if (e is T) return e
            throw e
        }
        fail("expected ${T::class.simpleName}")
        error("unreachable")
    }

    @Test
    fun linkKeepsUid() = runTest {
        val linked = linker(anon).link(
            link = { AuthUser("anon-uid", isAnonymous = false, AuthUser.GOOGLE) },
            signIn = { error("not signed in path") },
        )
        assertEquals("anon-uid", linked.uid)
        assertEquals(false, linked.isAnonymous)
    }

    @Test
    fun signedOutJustSignsIn() = runTest {
        val user = linker(current = null).link(link = { error("no user to link") }, signIn = { AuthUser("new", false) })
        assertEquals("new", user.uid)
    }

    @Test
    fun notAnonymousIsAlreadyLinked() = runTest {
        assertThrows<AuthError.AlreadyLinked> {
            linker(AuthUser("g", isAnonymous = false)).link(link = { error("must not link") }, signIn = { error("must not sign in") })
        }
    }

    @Test
    fun credentialInUseKeepsUpdatedCredentialForSwitch() = runTest {
        val used = mutableListOf<String>()
        val l = linker(anon, used)
        assertThrows<AuthError.CredentialAlreadyInUse> {
            l.link(link = { throw FirebaseLike(AccountLinker.CREDENTIAL_ALREADY_IN_USE, updated = "updated") }, signIn = { error("x") }, fallback = "original")
        }
        assertEquals("existing-uid", l.switchToPending().uid)
        assertEquals(listOf("updated"), used)
        // 1 回だけ使える
        assertThrows<AuthError.NoPendingAccount> { l.switchToPending() }
    }

    @Test
    fun credentialInUseWithoutUpdatedUsesFallback() = runTest {
        val used = mutableListOf<String>()
        val l = linker(anon, used)
        assertThrows<AuthError.CredentialAlreadyInUse> {
            l.link(link = { throw FirebaseLike(AccountLinker.CREDENTIAL_ALREADY_IN_USE) }, signIn = { error("x") }, fallback = "google-token")
        }
        l.switchToPending()
        assertEquals(listOf("google-token"), used)
    }

    @Test
    fun credentialInUseWithNothingToSwitchFailsOnSwitch() = runTest {
        val l = linker(anon)
        assertThrows<AuthError.CredentialAlreadyInUse> {
            l.link(link = { throw FirebaseLike(AccountLinker.CREDENTIAL_ALREADY_IN_USE) }, signIn = { error("x") })
        }
        assertThrows<AuthError.NoPendingAccount> { l.switchToPending() }
    }

    @Test
    fun newLinkForgetsPreviousPending() = runTest {
        val l = linker(anon)
        assertThrows<AuthError.CredentialAlreadyInUse> {
            l.link(link = { throw FirebaseLike(AccountLinker.CREDENTIAL_ALREADY_IN_USE, "old") }, signIn = { error("x") })
        }
        assertThrows<AuthError.Canceled> {
            l.link(link = { throw FirebaseLike(AccountLinker.WEB_CONTEXT_CANCELED) }, signIn = { error("x") })
        }
        assertThrows<AuthError.NoPendingAccount> { l.switchToPending() }
    }

    @Test
    fun providerAlreadyLinkedAndOtherErrors() = runTest {
        val l = linker(anon)
        assertThrows<AuthError.AlreadyLinked> {
            l.link(link = { throw FirebaseLike(AccountLinker.PROVIDER_ALREADY_LINKED) }, signIn = { error("x") })
        }
        val network = IllegalStateException("network down")
        val thrown = assertThrows<IllegalStateException> { l.link(link = { throw network }, signIn = { error("x") }) }
        assertSame(network, thrown)
        // 資格情報を取る段階のキャンセル(AuthError)はそのまま
        assertThrows<AuthError.Canceled> { l.link(link = { throw AuthError.Canceled() }, signIn = { error("x") }) }
    }

    @Test
    fun classifyFirebaseCodes() {
        assertEquals(LinkFailure.InUse("c"), AccountLinker.classifyFirebase(AccountLinker.CREDENTIAL_ALREADY_IN_USE, "c"))
        assertEquals(LinkFailure.AlreadyLinked, AccountLinker.classifyFirebase<String>(AccountLinker.PROVIDER_ALREADY_LINKED, null))
        assertEquals(LinkFailure.Canceled, AccountLinker.classifyFirebase<String>(AccountLinker.WEB_CONTEXT_CANCELED, null))
        assertTrue(AccountLinker.classifyFirebase<String>("ERROR_EMAIL_ALREADY_IN_USE", null) is LinkFailure.Other)
        assertTrue(AccountLinker.classifyFirebase<String>(null, null) is LinkFailure.Other)
    }
}
