package jp.kz2601.metamon.settings

import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.AppDocument
import jp.kz2601.metamon.dsl.decode
import jp.kz2601.metamon.services.AIClient
import jp.kz2601.metamon.services.ChatMessage
import jp.kz2601.metamon.services.ChatMode
import jp.kz2601.metamon.services.ChatTurn
import jp.kz2601.metamon.services.GenerateMode
import jp.kz2601.metamon.services.GenerateProgress
import jp.kz2601.metamon.services.MorphResult
import jp.kz2601.metamon.services.http.ClientInfo
import jp.kz2601.metamon.services.http.MetamonHttp
import jp.kz2601.metamon.services.live.HttpAppConfigClient
import jp.kz2601.metamon.services.mock.MockSpeechTranscriber
import jp.kz2601.metamon.voice.VoiceViewModel
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.cancel
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.withTimeout
import okhttp3.mockwebserver.Dispatcher
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import okhttp3.mockwebserver.RecordedRequest
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Before
import org.junit.Test
import java.net.InetAddress
import java.util.concurrent.CopyOnWriteArrayList

/** 設定を開いた時に `GET /v1/me/usage` を 1 回だけ読む(音声の画面の読み直しを「使っている量」も使う・iOS の `refreshPlanUsage`)。 */
class UsageSingleReadTest {
    private lateinit var server: MockWebServer
    private val paths = CopyOnWriteArrayList<String>()
    private val scope = CoroutineScope(Dispatchers.Default + Job())

    @Before
    fun setUp() {
        server = MockWebServer()
        server.dispatcher = object : Dispatcher() {
            override fun dispatch(request: RecordedRequest): MockResponse {
                val path = request.path.orEmpty()
                paths += path
                val body = when (path) {
                    "/v1/me/usage" ->
                        """{"plan":"free","resetAt":"2026-11-11T15:00:00Z","ai":{"used":4,"limit":60,"remaining":56},"apps":{"used":3,"limit":5,"remaining":2},"perApp":{}}"""
                    "/v1/apps/app-1/usage" ->
                        """{"appId":"app-1","plan":"free","payer":"self","upgrade":true,"records":{"used":90,"limit":100,"remaining":10},"photos":null}"""
                    else -> return MockResponse().setResponseCode(404)
                }
                return MockResponse().setResponseCode(200).setBody(body).addHeader("Content-Type", "application/json")
            }
        }
        server.start()
    }

    @After
    fun tearDown() {
        scope.cancel()
        server.shutdown()
    }

    @Test
    fun openingSettingsReadsMyUsageOnce() = runBlocking<Unit> {
        val base = server.url("/").newBuilder().host(InetAddress.getByName(server.hostName).hostAddress).build().toString()
        val source = HttpAppConfigClient(MetamonHttp(base, tokens = { "token-1" }, clientInfo = ClientInfo(version = "0.1.0", build = "7")))
        val vm = VoiceViewModel(
            transcriber = MockSpeechTranscriber(),
            ai = NoAI,
            appId = { "app-1" },
            initial = AppDocument(
                appId = "app-1",
                version = 1,
                dsl = AppDSL.decode("""{"schemaVersion":1,"app":{"name":"家計簿"},"initialScreen":"home","screens":[{"id":"home","title":"家計簿","components":[]}]}"""),
            ),
            usage = source,
            scope = scope,
        )

        // 設定を開く: シートの読み直し(onRefreshUsage)と「使っている量」の節(今のアプリの分だけ読む)
        vm.refreshUsage()
        val me = withTimeout(5_000) { vm.state.first { it.myUsage != null } }.myUsage
        val app = loadAppUsage(source, "app-1")
        val model = UsageSectionModel.of(me, app, isPro = false)

        assertEquals(listOf(UsageSectionModel.Resource.APPS, UsageSectionModel.Resource.RECORDS_PER_APP), model.rows.map { it.resource })
        assertEquals(1, paths.count { it == "/v1/me/usage" })
        assertEquals(1, paths.count { it == "/v1/apps/app-1/usage" })
        assertEquals(56, vm.state.value.remainingToday?.remaining)
    }

    private object NoAI : AIClient {
        override suspend fun morph(transcript: String, appId: String, currentVersion: Int, locale: String?, mode: GenerateMode): MorphResult =
            error("not used")

        override suspend fun morphStreaming(
            transcript: String,
            appId: String,
            currentVersion: Int,
            locale: String?,
            mode: GenerateMode,
            progress: (GenerateProgress) -> Unit,
        ): MorphResult = error("not used")

        override suspend fun chat(appId: String, messages: List<ChatMessage>, locale: String?, mode: ChatMode?): ChatTurn = error("not used")
        override suspend fun resetChat(appId: String) = Unit
    }
}
