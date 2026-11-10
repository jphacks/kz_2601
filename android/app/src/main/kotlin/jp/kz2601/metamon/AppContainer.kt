package jp.kz2601.metamon

import android.content.Context
import android.util.Log
import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.AppDocument
import jp.kz2601.metamon.runtime.AndroidRuntimeHost
import jp.kz2601.metamon.runtime.AppRuntime
import jp.kz2601.metamon.runtime.RuntimeHost
import jp.kz2601.metamon.dsl.decode
import jp.kz2601.metamon.dsl.fallback
import jp.kz2601.metamon.dsl.screen
import jp.kz2601.metamon.language.AppLocales
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
import jp.kz2601.metamon.services.local.AndroidConnectivity
import jp.kz2601.metamon.services.local.DocumentCache
import jp.kz2601.metamon.services.local.RecordSyncEngine
import jp.kz2601.metamon.services.mock.DslSamples
import jp.kz2601.metamon.services.mock.InMemoryAppConfigClient
import jp.kz2601.metamon.services.mock.MockAIClient
import jp.kz2601.metamon.services.mock.MockAuthService
import jp.kz2601.metamon.services.mock.MockSeed
import jp.kz2601.metamon.services.mock.MockTranscriber
import jp.kz2601.metamon.services.mock.OfflineSimulatingAppConfigClient
import jp.kz2601.metamon.services.speech.AndroidSpeechTranscriber
import jp.kz2601.metamon.services.sync.HotReload
import jp.kz2601.metamon.services.sync.RealtimeDocumentSync
import jp.kz2601.metamon.services.sync.SseChangeEventSource
import jp.kz2601.metamon.services.sync.asDocumentFetcher
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
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
    /** 文字起こし。本物は端末内の認識、モックは決まった文を返す [MockTranscriber] */
    val transcriber: SpeechTranscriber,
    val appId: DeviceAppId,
    val knobs: DebugKnobs,
    /** 記録のローカル DB と送信待ちの列(#882)。[MetamonApplication] が start する */
    val recordSync: RecordSyncEngine,
    /** 最後に取得した DSL(オフライン起動・#883)。appconfig が取得のたびに入れる */
    val documentCache: DocumentCache,
    /** 他端末・共有の相手の変更(SSE + ポーリングの予備・#883)。モックは null */
    val hotReload: HotReload?,
    private val host: RuntimeHost,
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
                makeMock(context, environment, samples, initialDsl, knobs)
            } else {
                makeLive(context, environment, initialDsl, knobs)
            }
        }

        /** `METAMON_MOCK=1`: 通信しない構成(iOS の `.mock`)。 */
        private fun makeMock(context: Context, environment: AppEnvironment, samples: DslSamples, initialDsl: AppDSL, knobs: DebugKnobs): AppContainer {
            Log.i(TAG, "モックの構成で起動します($knobs)")
            val appConfig = InMemoryAppConfigClient(
                seed = MockSeed.demoRecords(),
                apps = if (knobs.mockSharing) MockSeed.sharingApps else emptyList(),
                sharing = knobs.mockSharing,
                initialDocument = { appId -> samples.document(DslSamples.INITIAL, appId, 1) },
            )
            val delayMillis = knobs.mockAiDelaySeconds?.let { (it * 1000).toLong() } ?: 1_200
            val records = offlineIfRequested(appConfig, knobs)
            return AppContainer(
                environment = environment,
                auth = MockAuthService(signedIn = true),
                initialDsl = initialDsl,
                appConfig = records,
                ai = MockAIClient(samples, delayMillis = delayMillis),
                transcriber = MockTranscriber(),
                appId = DeviceAppId(context),
                knobs = knobs,
                // 本物の記録と混ざらないよう、モックは別の置き場
                recordSync = recordSync(context, records, "metamon-records-mock"),
                documentCache = DocumentCache(null),
                hotReload = null,
                host = AndroidRuntimeHost(context.applicationContext),
            )
        }

        private fun makeLive(context: Context, environment: AppEnvironment, initialDsl: AppDSL, knobs: DebugKnobs): AppContainer {
            FirebaseBootstrap.configureIfAvailable(context)
            val auth = FirebaseAuthService.createIfConfigured() ?: MockAuthService(signedIn = true)

            val tokens = AuthTokenProvider(auth)
            val clientInfo = ClientInfo(version = BuildConfig.VERSION_NAME, build = BuildConfig.VERSION_CODE.toString())
            // アプリ内で選んだ言語(無ければ端末の言語)。リクエストごとに読むので切り替えはすぐ効く(#843)
            val language = { AppLocales.languageCode(context) }
            val device = DeviceContext(language = language)
            // App Check(Play Integrity)はまだ無い: ヘッダ無しで送る(backend が enforce の環境では 401 になる)
            val client = MetamonHttp.defaultClient()
            fun http(baseUrl: String) = MetamonHttp(baseUrl, tokens, clientInfo, appCheck = null, client = client, device = device)
            val log: (String) -> Unit = { Log.w(TAG, it) }

            val documentCache = DocumentCache(File(context.filesDir, "metamon-dsl"))
            val appConfigHttp = http(environment.appConfigBaseUrl)
            val appConfig = offlineIfRequested(HttpAppConfigClient(appConfigHttp, cache = documentCache), knobs)
            val sync = RealtimeDocumentSync(appConfig.asDocumentFetcher(), SseChangeEventSource(appConfigHttp), log = log)

            return AppContainer(
                environment = environment,
                auth = auth,
                initialDsl = initialDsl,
                appConfig = appConfig,
                ai = HttpAIClient(http(environment.aiBaseUrl), log = log),
                // 声は端末内だけで文字にする(#354 §3)。端末内が無い端末は文字入力に落とす
                transcriber = AndroidSpeechTranscriber(context, language = language, log = { Log.i(TAG, it) }),
                appId = DeviceAppId(context),
                knobs = knobs,
                recordSync = recordSync(context, appConfig, "metamon-records"),
                documentCache = documentCache,
                hotReload = HotReload(sync, CoroutineScope(SupervisorJob() + Dispatchers.Default)),
                host = AndroidRuntimeHost(context.applicationContext),
            )
        }

        private fun recordSync(context: Context, client: AppConfigClient, dir: String) = RecordSyncEngine.create(
            client = client,
            connectivity = AndroidConnectivity(context),
            directory = File(context.filesDir, dir),
            scope = CoroutineScope(SupervisorJob() + Dispatchers.Default),
        )

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

    /** アプリ全体のランタイム(#889)。起動時に 1 つだけ作る。 */
    val runtime: AppRuntime by lazy {
        AppRuntime(
            document = AppDocument(appId = appId.current, dsl = initialDsl),
            recordSync = recordSync,
            scope = CoroutineScope(SupervisorJob() + Dispatchers.Main.immediate),
            host = host,
            currentUserId = { auth.currentUser.value?.uid },
            // appId の採番(#360)前は "local" のままなので、AppRuntime がサーバーに取りに行かない
            restore = { id -> appConfig.fetchDocument(id).document },
            hotReloader = hotReload,
            cachedDocument = { id -> documentCache.load(id)?.document },
        )
    }
}
