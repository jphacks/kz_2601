package jp.kz2601.metamon.desktop.auth

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotEquals
import org.junit.Assert.assertThrows
import org.junit.Assert.assertTrue
import org.junit.Test
import java.net.URI
import java.net.URLDecoder
import java.nio.charset.StandardCharsets
import java.security.SecureRandom

/** PKCE と同意の URL(#2372)。通信はしない。 */
class GooglePkce2372Test {
    private val unreserved = Regex("^[A-Za-z0-9._~-]+$")

    @Test
    fun rfc7636AppendixBChallenge() {
        assertEquals(
            "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM",
            GooglePkce.codeChallengeS256("dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"),
        )
    }

    @Test
    fun verifierLengthAndCharset() {
        val random = SecureRandom()
        for (length in listOf(43, 64, 128)) {
            val v = GooglePkce.newCodeVerifier(random, length)
            assertEquals(length, v.length)
            assertTrue(unreserved.matches(v))
        }
        assertEquals(GooglePkce.DEFAULT_VERIFIER_LENGTH, GooglePkce.newCodeVerifier().length)
        assertThrows(IllegalArgumentException::class.java) { GooglePkce.newCodeVerifier(random, 42) }
        assertThrows(IllegalArgumentException::class.java) { GooglePkce.newCodeVerifier(random, 129) }
    }

    @Test
    fun challengeRejectsInvalidVerifier() {
        assertThrows(IllegalArgumentException::class.java) { GooglePkce.codeChallengeS256("short") }
        assertThrows(IllegalArgumentException::class.java) { GooglePkce.codeChallengeS256("a".repeat(42) + " ") }
    }

    @Test
    fun requestsAreFreshAndConsistent() {
        val a = GooglePkce.newRequest()
        val b = GooglePkce.newRequest()
        assertNotEquals(a.codeVerifier, b.codeVerifier)
        assertNotEquals(a.state, b.state)
        assertEquals(GooglePkce.codeChallengeS256(a.codeVerifier), a.codeChallenge)
        assertEquals(43, a.state.length) // 32 バイトの base64url(詰め物なし)
        assertTrue(unreserved.matches(a.state))
    }

    @Test
    fun toStringHidesValues() {
        val r = GooglePkce.newRequest()
        val s = r.toString()
        assertFalse(s.contains(r.codeVerifier))
        assertFalse(s.contains(r.codeChallenge))
        assertFalse(s.contains(r.state))
    }

    @Test
    fun authorizationUrlHasAllParams() {
        val r = PkceRequest("v".repeat(43), "challenge-value", "state-value")
        val url = GooglePkce.authorizationUrl("fake-client.apps.example", 54321, r)
        assertTrue(url.startsWith(GooglePkce.AUTHORIZATION_ENDPOINT + "?"))
        val q = query(url)
        assertEquals("fake-client.apps.example", q["client_id"])
        assertEquals("http://127.0.0.1:54321", q["redirect_uri"])
        assertEquals("code", q["response_type"])
        assertEquals("openid email profile", q["scope"])
        assertEquals("challenge-value", q["code_challenge"])
        assertEquals("S256", q["code_challenge_method"])
        assertEquals("state-value", q["state"])
        assertFalse("verifier must not be in the URL", url.contains(r.codeVerifier))
    }

    @Test
    fun authorizationUrlEscapesParams() {
        val r = PkceRequest("v".repeat(43), "c", "a&b=c d")
        val url = GooglePkce.authorizationUrl("id with space&x=1", 8080, r)
        assertTrue(url.contains("scope=openid%20email%20profile"))
        assertTrue(url.contains("redirect_uri=http%3A%2F%2F127.0.0.1%3A8080"))
        assertTrue(url.contains("client_id=id%20with%20space%26x%3D1"))
        assertTrue(url.contains("state=a%26b%3Dc%20d"))
        assertFalse(url.contains("+"))
        val q = query(url)
        assertEquals("id with space&x=1", q["client_id"])
        assertEquals("a&b=c d", q["state"])
        URI(url) // 正しい URI として読める
    }

    @Test
    fun redirectUriRejectsBadPort() {
        assertThrows(IllegalArgumentException::class.java) { GooglePkce.redirectUri(0) }
        assertThrows(IllegalArgumentException::class.java) { GooglePkce.redirectUri(65536) }
    }

    @Test
    fun statesMatch() {
        assertTrue(GooglePkce.statesMatch("abc", "abc"))
        assertFalse(GooglePkce.statesMatch("abc", "abd"))
        assertFalse(GooglePkce.statesMatch("abc", "abcd"))
        assertFalse(GooglePkce.statesMatch("abc", null))
    }

    private fun query(url: String): Map<String, String> =
        url.substringAfter('?').split('&').associate {
            val (k, v) = it.split('=', limit = 2)
            URLDecoder.decode(k, StandardCharsets.UTF_8) to URLDecoder.decode(v, StandardCharsets.UTF_8)
        }
}
