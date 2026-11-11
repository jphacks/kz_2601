package jp.kz2601.metamon.services.live

import kotlinx.coroutines.delay
import kotlinx.coroutines.test.runTest
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test

/** google-services.json が無い・Firebase が壊れている時に落ちないこと(#840)。 */
class FirebaseBootstrapTest {
    private val logs = mutableListOf<String>()
    private var appCheckInstalled = 0

    @Before
    fun setUp() = FirebaseBootstrap.resetForTest()

    @After
    fun tearDown() = FirebaseBootstrap.resetForTest()

    private fun configure(initialize: () -> Boolean, installAppCheck: () -> Unit = { appCheckInstalled++ }) =
        FirebaseBootstrap.configure(initialize, installAppCheck) { message, _ -> logs += message }

    @Test
    fun withoutGoogleServicesJsonStaysUnconfigured() {
        assertFalse(configure(initialize = { false }))
        assertFalse(FirebaseBootstrap.isConfigured)
        assertEquals(0, appCheckInstalled)
        assertTrue(logs.single().contains("google-services.json"))
        // Firebase の実装は作られず、呼ぶ側はモック / Noop に落とす
        assertNull(FirebaseAuthService.createIfConfigured())
        assertNull(FirebaseAppCheckTokenProvider.createIfConfigured())
        assertNull(FirebaseCrashReporter.createIfConfigured())
    }

    @Test
    fun initializeThrowingDoesNotCrash() {
        assertFalse(configure(initialize = { throw IllegalStateException("broken") }))
        assertFalse(FirebaseBootstrap.isConfigured)
        assertEquals(0, appCheckInstalled)
    }

    @Test
    fun configuredInstallsAppCheckOnce() {
        assertTrue(configure(initialize = { true }))
        assertTrue(configure(initialize = { error("should not run twice") }))
        assertEquals(1, appCheckInstalled)
    }

    @Test
    fun appCheckFailureKeepsFirebase() {
        assertTrue(configure(initialize = { true }, installAppCheck = { throw IllegalStateException("no play services") }))
        assertTrue(FirebaseBootstrap.isConfigured)
        assertTrue(logs.single().contains("App Check"))
    }
}

class FirebaseAppCheckTokenProviderTest {
    private val logs = mutableListOf<String>()

    @Test
    fun passesTokenAndForceRefresh() = runTest {
        val calls = mutableListOf<Boolean>()
        val provider = FirebaseAppCheckTokenProvider(fetch = { calls += it; "ac-$it" }, log = { logs += it })
        assertEquals("ac-false", provider.token(false))
        assertEquals("ac-true", provider.token(true))
        assertEquals(listOf(false, true), calls)
    }

    @Test
    fun failureGivesNullInsteadOfThrowing() = runTest {
        val provider = FirebaseAppCheckTokenProvider(fetch = { throw IllegalStateException("integrity") }, log = { logs += it })
        assertNull(provider.token(false))
        assertTrue(logs.single().contains("unavailable"))
    }

    @Test
    fun slowTokenTimesOutToNull() = runTest {
        val provider = FirebaseAppCheckTokenProvider(
            fetch = { delay(10_000); "late" },
            timeoutMillis = 5_000,
            log = { logs += it },
        )
        assertNull(provider.token(false))
        assertTrue(logs.single().contains("timed out"))
    }
}
