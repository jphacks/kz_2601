package jp.kz2601.metamon.desktop.auth

import jp.kz2601.metamon.desktop.net.DesktopApiConfig
import jp.kz2601.metamon.services.http.FailureKind
import kotlinx.coroutines.runBlocking
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import okhttp3.mockwebserver.SocketPolicy
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test
import java.time.Instant

/** Firebase Auth の REST(#2150)。通信は MockWebServer だけで、本物の Firebase は呼ばない。 */
class FirebaseAuthRestClientTest {
    private lateinit var server: MockWebServer
    private val now = Instant.parse("2026-11-12T00:00:00Z")

    @Before
    fun setUp() {
        server = MockWebServer().apply { start() }
    }

    @After
    fun tearDown() {
        server.shutdown()
    }

    private fun client(apiKey: String? = "test-key") = FirebaseAuthRestClient(
        config = FirebaseAuthConfig(
            apiKey = apiKey,
            identityToolkitUrl = server.url("/identitytoolkit/v1").toString(),
            secureTokenUrl = server.url("/securetoken/v1").toString(),
        ),
        clock = { now },
    )

    private fun json(body: String, code: Int = 200) =
        MockResponse().setResponseCode(code).setHeader("Content-Type", "application/json").setBody(body)

    private fun firebaseError(message: String, code: Int = 400) =
        json("""{"error":{"code":$code,"message":"$message","errors":[{"message":"$message","reason":"invalid"}]}}""", code)

    @Test
    fun signInAnonymously() = runBlocking {
        server.enqueue(json("""{"kind":"identitytoolkit#SignupNewUserResponse","idToken":"id-1","refreshToken":"rt-1","expiresIn":"3600","localId":"uid-1"}"""))

        val result = client().signInAnonymously()

        assertEquals(AuthResult.Ok(AuthSession("uid-1", "id-1", "rt-1", now.plusSeconds(3600))), result)
        val req = server.takeRequest()
        assertEquals("POST", req.method)
        assertEquals("/identitytoolkit/v1/accounts:signUp", req.requestUrl!!.encodedPath)
        assertEquals("test-key", req.requestUrl!!.queryParameter("key"))
        assertEquals("""{"returnSecureToken":true}""", req.body.readUtf8())
    }

    @Test
    fun refresh() = runBlocking {
        server.enqueue(json("""{"expires_in":"3600","token_type":"Bearer","refresh_token":"rt-2","id_token":"id-2","user_id":"uid-1","project_id":"p"}"""))

        val result = client().refresh("rt-1")

        assertEquals(AuthResult.Ok(AuthSession("uid-1", "id-2", "rt-2", now.plusSeconds(3600))), result)
        val req = server.takeRequest()
        assertEquals("/securetoken/v1/token", req.requestUrl!!.encodedPath)
        assertEquals("test-key", req.requestUrl!!.queryParameter("key"))
        assertTrue(req.getHeader("Content-Type")!!.startsWith("application/x-www-form-urlencoded"))
        assertEquals("grant_type=refresh_token&refresh_token=rt-1", req.body.readUtf8())
    }

    @Test
    fun refreshKeepsOldRefreshTokenWhenNotReturned() = runBlocking {
        server.enqueue(json("""{"expires_in":"60","id_token":"id-3","user_id":"uid-1"}"""))

        val session = (client().refresh("rt-1") as AuthResult.Ok).value

        assertEquals("rt-1", session.refreshToken)
    }

    @Test
    fun expiredOrInvalidRefreshTokenIsSessionExpired() = runBlocking {
        server.enqueue(firebaseError("TOKEN_EXPIRED"))
        server.enqueue(firebaseError("INVALID_REFRESH_TOKEN"))
        server.enqueue(firebaseError("USER_DISABLED"))

        repeat(3) {
            assertEquals(AuthResult.Failed(FailureKind.SessionExpired), client().refresh("rt-old"))
        }
    }

    @Test
    fun otherFailuresUseFailureKind() = runBlocking {
        server.enqueue(firebaseError("OPERATION_NOT_ALLOWED"))
        server.enqueue(firebaseError("TOO_MANY_ATTEMPTS_TRY_LATER"))
        server.enqueue(json("""{"error":{"code":503,"message":"UNAVAILABLE"}}""", 503).setHeader("Retry-After", "5"))
        server.enqueue(json("not json"))

        assertEquals(AuthResult.Failed(FailureKind.Rejected("OPERATION_NOT_ALLOWED")), client().signInAnonymously())
        assertEquals(AuthResult.Failed(FailureKind.Busy(null)), client().signInAnonymously())
        assertEquals(AuthResult.Failed(FailureKind.Busy(5.0)), client().signInAnonymously())
        assertTrue((client().signInAnonymously() as AuthResult.Failed).kind is FailureKind.Other)
    }

    @Test
    fun networkFailureIsOffline() = runBlocking {
        server.enqueue(MockResponse().setSocketPolicy(SocketPolicy.DISCONNECT_AT_START))

        assertEquals(AuthResult.Failed(FailureKind.Offline), client().refresh("rt-1"))
    }

    @Test
    fun missingApiKeyDoesNotCallNetwork() = runBlocking {
        val result = client(apiKey = null).signInAnonymously()

        assertTrue((result as AuthResult.Failed).kind is FailureKind.Rejected)
        assertEquals(0, server.requestCount)
    }

    @Test
    fun weakPasswordStyleMessageUsesLeadingCode() {
        assertEquals(FailureKind.SessionExpired, FirebaseAuthRestClient.classify(400, "TOKEN_EXPIRED : expired"))
        assertEquals(FailureKind.Timeout, FirebaseAuthRestClient.classify(504, null))
    }

    @Test
    fun sessionExpiry() {
        val session = AuthSession("u", "secret-id", "secret-refresh", now.plusSeconds(120))
        assertFalse(session.isExpired(now))
        assertTrue(session.isExpired(now.plusSeconds(61)))
        assertFalse(session.toString().contains("secret"))
    }

    @Test
    fun configFromEnvironmentKeepsKeyOutOfLogs() {
        val config = FirebaseAuthConfig.fromEnvironment(mapOf("TAILORIT_FIREBASE_API_KEY" to " secret-key "))
        assertEquals("secret-key", config.apiKey)
        assertEquals(FirebaseAuthConfig.IDENTITY_TOOLKIT_URL, config.identityToolkitUrl)
        assertFalse(config.toString().contains("secret-key"))
        assertFalse(FirebaseAuthConfig.fromEnvironment(emptyMap()).isConfigured)
    }

    @Test
    fun desktopApiStaysMockByDefault() {
        // サインインを足しても既定は通信しない
        assertFalse(DesktopApiConfig.fromEnvironment(emptyMap()).isLive)
    }

    @Test
    fun inMemoryTokenStore() {
        val store: TokenStore = InMemoryTokenStore()
        assertNull(store.load())
        store.save(StoredCredential("uid-1", "rt-1"))
        assertEquals(StoredCredential("uid-1", "rt-1"), store.load())
        assertFalse(store.load().toString().contains("rt-1"))
        store.clear()
        assertNull(store.load())
    }
}
