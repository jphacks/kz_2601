package jp.kz2601.metamon

import org.junit.Assert.assertEquals
import org.junit.Assert.assertSame
import org.junit.Test

/**
 * App Check のプロバイダを build type で出し分けていること(#1008・#318)。
 * debug は Firebase Console に登録したトークンだけ通る debug provider、配布版(stg / prod の release)は Play Integrity。
 */
class AppCheckProvidersTest {
    @Test
    fun factoryMatchesBuildType() {
        val expected = if (BuildConfig.DEBUG) "DebugAppCheckProviderFactory" else "PlayIntegrityAppCheckProviderFactory"
        assertEquals(expected, AppCheckProviders.factory().javaClass.simpleName)
    }

    @Test
    fun factoryIsReusable() {
        assertSame(AppCheckProviders.factory().javaClass, AppCheckProviders.factory().javaClass)
    }
}
