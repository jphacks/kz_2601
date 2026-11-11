package jp.kz2601.metamon.services.http

import kotlinx.coroutines.runBlocking
import okhttp3.mockwebserver.MockWebServer
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Before
import org.junit.Test

/** トークンを取り直しても 401 の時だけセッション切れを知らせる(#1102)。 */
class SessionExpirySignalTest {
    private lateinit var server: MockWebServer
    private var fired = 0

    @Before
    fun setUp() {
        server = MockWebServer().apply { start() }
    }

    @After
    fun tearDown() {
        server.shutdown()
    }

    private fun post() = HttpRequest("POST", listOf("v1", "generate"), body = "{}")

    @Test
    fun stillUnauthorizedAfterRefreshFiresOnce() = runBlocking<Unit> {
        server.enqueue(json(401, errorBody("unauthenticated")))
        server.enqueue(json(401, errorBody("unauthenticated")))

        expectApiError { server.metamonHttp(tokens = FakeTokens("old", "new"), onSessionExpired = { fired++ }).send(post()) }

        assertEquals(2, server.requestCount)
        assertEquals(1, fired)
    }

    @Test
    fun sameTokenAfterRefreshAlsoFires() = runBlocking<Unit> {
        server.enqueue(json(401, errorBody("unauthenticated")))

        expectApiError { server.metamonHttp(tokens = FakeTokens("only"), onSessionExpired = { fired++ }).send(post()) }

        assertEquals(1, fired)
    }

    @Test
    fun refreshThatSucceedsDoesNotFire() = runBlocking<Unit> {
        // 匿名でもトークンを取り直して通れば、サインイン画面には戻さない
        server.enqueue(json(401, errorBody("unauthenticated")))
        server.enqueue(json(200, "{}"))

        server.metamonHttp(tokens = FakeTokens("old", "new"), onSessionExpired = { fired++ }).send(post())

        assertEquals(0, fired)
    }

    @Test
    fun withoutTokenDoesNotFire() = runBlocking<Unit> {
        // 未サインイン(持ち主の受け入れ前を含む)はトークンを付けないので、401 でもサインアウトしない
        server.enqueue(json(401, errorBody("unauthenticated")))

        expectApiError { server.metamonHttp(tokens = FakeTokens(null), onSessionExpired = { fired++ }).send(post()) }

        assertEquals(0, fired)
    }

    @Test
    fun appCheckRejectionDoesNotFire() = runBlocking<Unit> {
        server.enqueue(json(401, errorBody("app_check_failed")))
        server.enqueue(json(401, errorBody("app_check_failed")))

        expectApiError {
            server.metamonHttp(appCheck = FakeAppCheck("a", "b"), onSessionExpired = { fired++ }).send(post())
        }

        assertEquals(0, fired)
    }

    @Test
    fun signalForwardsToTheListenerInstalledLater() {
        val signal = SessionExpirySignal()
        signal.fire() // 受け手が無ければ何もしない
        signal.listener = { fired++ }
        signal.fire()
        assertEquals(1, fired)
    }
}
