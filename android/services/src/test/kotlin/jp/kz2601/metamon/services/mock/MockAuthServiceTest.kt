package jp.kz2601.metamon.services.mock

import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

class MockAuthServiceTest {
    @Test
    fun signedInWithFixedUidAndNoToken() = runTest {
        val auth = MockAuthService(signedIn = true)
        assertEquals(MockAuthService.FIXED_UID, auth.currentUser.value?.uid)
        assertNull(auth.idToken())
    }

    @Test
    fun signInAndOut() = runTest {
        val auth = MockAuthService(signedIn = false)
        assertNull(auth.currentUser.value)
        assertEquals(MockAuthService.FIXED_UID, auth.signInAnonymously().uid)
        assertEquals(MockAuthService.FIXED_UID, auth.currentUser.value?.uid)
        auth.signOut()
        assertNull(auth.currentUser.value)
    }
}
