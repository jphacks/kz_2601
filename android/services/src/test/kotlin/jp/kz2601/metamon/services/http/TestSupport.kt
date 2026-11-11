package jp.kz2601.metamon.services.http

import jp.kz2601.metamon.services.RemainingToday
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import java.net.InetAddress
import java.util.concurrent.TimeUnit

/** 呼ばれるたびに順にトークンを返す(forceRefresh で次へ進む)。 */
class FakeTokens(private vararg val values: String?) : TokenProvider {
    var index = 0
    val calls = mutableListOf<Boolean>()

    override suspend fun idToken(forceRefresh: Boolean): String? {
        calls += forceRefresh
        if (forceRefresh && index < values.lastIndex) index++
        return values.getOrNull(index)
    }
}

class FakeAppCheck(private vararg val values: String?) : AppCheckTokenProvider {
    var index = 0
    val calls = mutableListOf<Boolean>()

    override suspend fun token(forceRefresh: Boolean): String? {
        calls += forceRefresh
        if (forceRefresh && index < values.lastIndex) index++
        return values.getOrNull(index)
    }
}

val testClientInfo = ClientInfo(version = "0.1.0", build = "7")
val testDevice = DeviceContext(timeZone = { "Asia/Tokyo" }, language = { "ja" })

/**
 * 待ち受けているアドレス(127.0.0.1)そのものの URL。`url()` の localhost は ::1 にも引け、OkHttp 5 は ::1 を先に試すので、
 * 同じ番号で ::1 を開いている別プロセスにつながることがある。
 */
fun MockWebServer.baseUrl(): String =
    url("/").newBuilder().host(InetAddress.getByName(hostName).hostAddress).build().toString()

fun MockWebServer.metamonHttp(
    tokens: TokenProvider = FakeTokens("token-1"),
    appCheck: AppCheckTokenProvider? = null,
    sleeps: MutableList<Long> = mutableListOf(),
    retry: RetryPolicy = RetryPolicy(),
    remainingSink: ((RemainingToday) -> Unit)? = null,
    onSessionExpired: (() -> Unit)? = null,
): MetamonHttp = MetamonHttp(
    baseUrl = baseUrl(),
    tokens = tokens,
    clientInfo = testClientInfo,
    appCheck = appCheck,
    // OkHttp 自身の接続のやり直しを切って、MetamonHttp のやり直しだけを数える
    client = MetamonHttp.defaultClient().newBuilder().retryOnConnectionFailure(false).build(),
    retry = retry,
    device = testDevice,
    sleep = { sleeps += it },
    remainingSink = remainingSink,
    onSessionExpired = onSessionExpired,
)

fun json(status: Int, body: String, vararg headers: Pair<String, String>): MockResponse =
    MockResponse().setResponseCode(status).setBody(body).addHeader("Content-Type", "application/json").apply {
        headers.forEach { (k, v) -> addHeader(k, v) }
    }

fun errorBody(code: String, message: String = "msg", details: String? = null): String =
    """{"error":{"code":"$code","message":"$message"${details?.let { ",\"details\":$it" } ?: ""}}}"""

fun MockWebServer.take() = takeRequest(5, TimeUnit.SECONDS) ?: error("no request")

const val MINI_DSL =
    """{"schemaVersion":1,"app":{"name":"家計簿","voiceEntry":"floating"},"initialScreen":"home","screens":[{"id":"home","title":"ホーム","components":[]}]}"""
