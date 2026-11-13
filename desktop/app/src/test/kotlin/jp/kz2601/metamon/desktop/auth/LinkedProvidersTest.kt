package jp.kz2601.metamon.desktop.auth

import jp.kz2601.metamon.services.http.FailureKind
import kotlinx.coroutines.runBlocking
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test

/** つながっているサインインの方法を `accounts:lookup` から読む(読むだけ)。通信は MockWebServer だけ。 */
class LinkedProvidersTest {
    private lateinit var server: MockWebServer

    @Before
    fun setUp() {
        server = MockWebServer().apply { start() }
    }

    @After
    fun tearDown() {
        server.shutdown()
    }

    private fun client(apiKey: String? = "test-key") = FirebaseAuthRestClient(
        FirebaseAuthConfig(apiKey = apiKey, identityToolkitUrl = server.url("/identitytoolkit/v1").toString()),
    )

    private fun json(body: String, code: Int = 200) =
        MockResponse().setResponseCode(code).setHeader("Content-Type", "application/json").setBody(body)

    private fun lookup(body: String): LinkedProviders {
        server.enqueue(json(body))
        val result = runBlocking { client().lookupLinkedProviders("id-1") }
        return (result as AuthResult.Ok).value
    }

    @Test
    fun googleOnly() {
        val linked = lookup(
            """{"users":[{"localId":"u","email":"a@example.com","providerUserInfo":[{"providerId":"google.com","federatedId":"1"}]}]}""",
        )
        assertEquals(listOf(SignInProvider.GOOGLE), linked.rows)
        assertEquals(false, linked.isAnonymous)

        val req = server.takeRequest()
        assertEquals("POST", req.method)
        assertEquals("/identitytoolkit/v1/accounts:lookup", req.requestUrl!!.encodedPath)
        assertEquals("test-key", req.requestUrl!!.queryParameter("key"))
        assertEquals("""{"idToken":"id-1"}""", req.body.readUtf8())
    }

    @Test
    fun appleAndGoogleAreListedInIosOrderAndOthersAreIgnored() {
        val linked = lookup(
            """{"users":[{"providerUserInfo":[{"providerId":"google.com"},{"providerId":"password"},{"providerId":"apple.com"}]}]}""",
        )
        assertEquals(listOf(SignInProvider.APPLE, SignInProvider.GOOGLE), linked.rows)
        assertEquals(false, linked.isAnonymous)
    }

    @Test
    fun anonymousShowsNoRows() {
        val noKey = lookup("""{"users":[{"localId":"u"}]}""")
        assertTrue(noKey.isAnonymous)
        assertTrue(noKey.rows.isEmpty())

        val emptyList = lookup("""{"users":[{"localId":"u","providerUserInfo":[]}]}""")
        assertTrue(emptyList.isAnonymous)
        assertTrue(emptyList.rows.isEmpty())
    }

    @Test
    fun passwordOnlyIsNotAnonymousButHasNoRows() {
        val linked = lookup("""{"users":[{"providerUserInfo":[{"providerId":"password"}]}]}""")
        assertEquals(false, linked.isAnonymous)
        assertTrue(linked.rows.isEmpty())
    }

    @Test
    fun failuresAreClassified() = runBlocking {
        server.enqueue(json("""{"error":{"code":400,"message":"INVALID_ID_TOKEN"}}""", 400))
        assertEquals(AuthResult.Failed(FailureKind.SessionExpired), client().lookupLinkedProviders("bad"))

        server.enqueue(json("""{"kind":"identitytoolkit#GetAccountInfoResponse"}"""))
        assertTrue((client().lookupLinkedProviders("id-1") as AuthResult.Failed).kind is FailureKind.Other)

        assertTrue((client(apiKey = null).lookupLinkedProviders("id-1") as AuthResult.Failed).kind is FailureKind.Rejected)
        assertEquals(2, server.requestCount)
    }
}
