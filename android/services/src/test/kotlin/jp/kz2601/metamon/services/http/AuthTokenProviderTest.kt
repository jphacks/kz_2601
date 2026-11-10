package jp.kz2601.metamon.services.http

import android.app.Activity
import jp.kz2601.metamon.services.AuthService
import jp.kz2601.metamon.services.AuthUser
import jp.kz2601.metamon.services.mock.MockAuthService
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.runBlocking
import okhttp3.mockwebserver.MockWebServer
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Before
import org.junit.Test

/** 認証の ID トークンが MetamonHttp の Bearer に渡ること(#840)。 */
class AuthTokenProviderTest {
    private lateinit var server: MockWebServer

    @Before
    fun setUp() {
        server = MockWebServer().apply { start() }
    }

    @After
    fun tearDown() {
        server.shutdown()
    }

    private fun get() = HttpRequest("GET", listOf("v1", "apps"))

    @Test
    fun firebaseIdTokenBecomesBearer() = runBlocking<Unit> {
        server.enqueue(json(200, "{}"))
        server.metamonHttp(tokens = AuthTokenProvider(TokenAuth("id-1"))).send(get())

        assertEquals("Bearer id-1", server.take().getHeader("Authorization"))
    }

    @Test
    fun expiredTokenIsRefreshedFromAuthService() = runBlocking<Unit> {
        val auth = TokenAuth("id-old", "id-new")
        server.enqueue(json(401, errorBody("unauthenticated")))
        server.enqueue(json(200, "{}"))
        server.metamonHttp(tokens = AuthTokenProvider(auth)).send(get())

        assertEquals("Bearer id-old", server.take().getHeader("Authorization"))
        assertEquals("Bearer id-new", server.take().getHeader("Authorization"))
        assertEquals(listOf(false, true), auth.calls)
    }

    @Test
    fun mockAuthSendsNoAuthorization() = runBlocking<Unit> {
        server.enqueue(json(200, "{}"))
        server.metamonHttp(tokens = AuthTokenProvider(MockAuthService(signedIn = true))).send(get())

        assertNull(server.take().getHeader("Authorization"))
    }
}

/** forceRefresh のたびに次のトークンを返す AuthService。 */
private class TokenAuth(private vararg val values: String) : AuthService {
    override val kind = "fake"
    override val currentUser: StateFlow<AuthUser?> = MutableStateFlow(AuthUser("u1", isAnonymous = false))
    val calls = mutableListOf<Boolean>()
    private var index = 0

    override suspend fun idToken(forceRefresh: Boolean): String? {
        calls += forceRefresh
        if (forceRefresh && index < values.lastIndex) index++
        return values[index]
    }

    override suspend fun signInAnonymously(): AuthUser = error("unused")
    override suspend fun signInWithGoogle(googleIdToken: String): AuthUser = error("unused")
    override suspend fun signInWithApple(activity: Activity): AuthUser = error("unused")
    override suspend fun signOut() = Unit
}
