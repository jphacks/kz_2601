package jp.kz2601.metamon

import android.content.Context
import android.os.LocaleList
import android.util.Log
import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.renderer.DSLRuntime
import jp.kz2601.metamon.services.AIClient
import jp.kz2601.metamon.services.AppConfigClient
import jp.kz2601.metamon.services.AppEnvironment
import jp.kz2601.metamon.services.AuthService
import jp.kz2601.metamon.services.SpeechTranscriber
import jp.kz2601.metamon.services.http.AuthTokenProvider
import jp.kz2601.metamon.services.http.ClientInfo
import jp.kz2601.metamon.services.http.DeviceContext
import jp.kz2601.metamon.services.http.MetamonHttp
import jp.kz2601.metamon.services.live.FirebaseAuthService
import jp.kz2601.metamon.services.live.FirebaseBootstrap
import jp.kz2601.metamon.services.live.HttpAIClient
import jp.kz2601.metamon.services.live.HttpAppConfigClient
import jp.kz2601.metamon.services.mock.DslSamples
import jp.kz2601.metamon.services.mock.InMemoryAppConfigClient
import jp.kz2601.metamon.services.mock.MockAIClient
import jp.kz2601.metamon.services.mock.MockAuthService
import jp.kz2601.metamon.services.mock.MockSeed
import jp.kz2601.metamon.services.mock.MockTranscriber
import jp.kz2601.metamon.services.mock.OfflineSimulatingAppConfigClient
import java.io.File
import java.time.Instant

/**
 * 手書きの組み立て 1 か所(#354 §5。Hilt は使わない・iOS の `BuildConfig.make` と同じ形)。
 * モックと本物の差し替えはこのファイルだけで完結する(画面はインターフェースしか知らない)。
 */
class AppContainer(
    val environment: AppEnvironment,
    val auth: AuthService,
    val initialDsl: AppDSL,
    val appConfig: AppConfigClient,
    val ai: AIClient,
    /** 文字起こし。本物(#359)が入るまでは、モックの構成の時だけある */
    val transcriber: SpeechTranscriber?,
    val knobs: DebugKnobs,
) {
    /** 画面の下に出す接続先の名前(モックなら "mock")。 */
    val environmentLabel: String get() = if (knobs.mock) "mock" else environment.name

    companion object {
        fun make(context: Context, knobs: DebugKnobs = DebugKnobs.NONE): AppContainer {
            val environment = AppEnvironment(
                name = BuildConfig.METAMON_ENV,
                aiBaseUrl = BuildConfig.AI_BASE_URL,
                appConfigBaseUrl = BuildConfig.APPCONFIG_BASE_URL,
            )
            val samples = DslSamples { name -> runCatching { context.assets.open("$name.json").bufferedReader().use { it.readText() } }.getOrNull() }
            val initialDsl = loadInitialDsl(samples, knobs)
            return if (knobs.mock) {
                makeMock(environment, samples, initialDsl, knobs)
            } else {
                makeLive(context, environment, initialDsl, knobs)
            }
        }

        /** `METAMON_MOCK=1`: 通信しない構成(iOS の `.mock`)。 */
        private fun makeMock(environment: AppEnvironment, samples: DslSamples, initialDsl: AppDSL, knobs: DebugKnobs): AppContainer {
            Log.i(TAG, "モックの構成で起動します($knobs)")
            val appConfig = InMemoryAppConfigClient(
                seed = MockSeed.demoRecords(),
                apps = if (knobs.mockSharing) MockSeed.sharingApps else emptyList(),
                sharing = knobs.mockSharing,
                initialDocument = { appId -> samples.document(DslSamples.INITIAL, appId, 1) },
            )
            val delayMillis = knobs.mockAiDelaySeconds?.let { (it * 1000).toLong() } ?: 1_200
            return AppContainer(
                environment = environment,
                auth = MockAuthService(signedIn = true),
                initialDsl = initialDsl,
                appConfig = offlineIfRequested(appConfig, knobs),
                ai = MockAIClient(samples, delayMillis = delayMillis),
                transcriber = MockTranscriber(),
                knobs = knobs,
            )
        }

        private fun makeLive(context: Context, environment: AppEnvironment, initialDsl: AppDSL, knobs: DebugKnobs): AppContainer {
            FirebaseBootstrap.configureIfAvailable(context)
            val auth = FirebaseAuthService.createIfConfigured() ?: MockAuthService(signedIn = true)

            val tokens = AuthTokenProvider(auth)
            val clientInfo = ClientInfo(version = BuildConfig.VERSION_NAME, build = BuildConfig.VERSION_CODE.toString())
            val device = DeviceContext(language = {
                val locales = LocaleList.getDefault()
                ClientInfo.languageCode((0 until locales.size()).map(locales::get))
            })
            // App Check(Play Integrity)はまだ無い: ヘッダ無しで送る(backend が enforce の環境では 401 になる)
            val client = MetamonHttp.defaultClient()
            fun http(baseUrl: String) = MetamonHttp(baseUrl, tokens, clientInfo, appCheck = null, client = client, device = device)
            val log: (String) -> Unit = { Log.w(TAG, it) }

            return AppContainer(
                environment = environment,
                auth = auth,
                initialDsl = initialDsl,
                appConfig = offlineIfRequested(HttpAppConfigClient(http(environment.appConfigBaseUrl)), knobs),
                ai = HttpAIClient(http(environment.aiBaseUrl), log = log),
                transcriber = null,
                knobs = knobs,
            )
        }

        /** `METAMON_E2E_OFFLINE_SECONDS=<n>`: 起動から n 秒は記録の API を圏外にする(iOS `RecordSyncFactory`)。 */
        private fun offlineIfRequested(inner: AppConfigClient, knobs: DebugKnobs): AppConfigClient {
            val seconds = knobs.offlineSeconds ?: return inner
            Log.i(TAG, "起動から ${seconds} 秒は圏外の扱いにします")
            return OfflineSimulatingAppConfigClient(inner, offlineUntil = Instant.now().plusMillis((seconds * 1000).toLong()))
        }

        /**
         * 最初に描く DSL。つまみ(`METAMON_INITIAL_DSL_PATH` > `METAMON_INITIAL_EXAMPLE`)が無ければ
         * 同梱の docs/dsl/examples/initial.json(話しかける前の画面)。`METAMON_INITIAL_SCREEN` で最初の画面を変える。
         */
        internal fun loadInitialDsl(samples: DslSamples, knobs: DebugKnobs): AppDSL {
            val fromKnob = knobs.initialDslPath?.let { path ->
                runCatching { AppDSL.decode(File(path).readText()) }
                    .onSuccess { Log.i(TAG, "DSL override: $path screens=${it.screens.map { s -> s.id }}") }
                    .onFailure { Log.e(TAG, "DSL override failed: $path", it) }
                    .getOrNull()
            } ?: knobs.initialExample?.let { name ->
                val json = samples.json(name)
                if (json == null) Log.e(TAG, "METAMON_INITIAL_EXAMPLE=$name が見つかりません")
                json?.let { DslSamples.envelope("example-$name", 1, it).document.dsl }
            }
            val dsl = fromKnob ?: runCatching { samples.document(DslSamples.INITIAL, "local", 0).document.dsl }.getOrElse {
                Log.e(TAG, "initial.json を読めませんでした", it)
                AppDSL.fallback
            }
            val screen = knobs.initialScreen
            return if (screen != null && dsl.screen(screen) != null) dsl.copy(initialScreen = screen) else dsl
        }

        const val TAG = "Metamon"
    }
}

/** 描画器に渡す能力の実装(iOS の `AppRuntime`)。 */
class AppRuntime(private val onVoice: (String?) -> Unit) : DSLRuntime {
    override fun onVoiceInput(componentId: String?) = onVoice(componentId)
}
