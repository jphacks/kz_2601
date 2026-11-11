package jp.kz2601.metamon

import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

/** flavor ごとの接続先が混ざっていないこと(stg のビルドが prod を向かない・逆も)。 */
class FlavorConfigTest {
    @Test
    fun urlsMatchEnvironment() {
        val env = BuildConfig.METAMON_ENV
        assertTrue(env == "stg" || env == "prod")
        assertTrue(BuildConfig.AI_BASE_URL.startsWith("https://ai-$env-"))
        assertTrue(BuildConfig.APPCONFIG_BASE_URL.startsWith("https://appconfig-$env-"))
        val suffix = if (env == "stg") ".stg" else ""
        assertEquals("jp.kz2601.metamon$suffix", BuildConfig.APPLICATION_ID)
    }
}
