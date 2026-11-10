package jp.kz2601.metamon

import android.content.Context
import android.os.LocaleList
import android.util.Log
import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.AppDocument
import jp.kz2601.metamon.runtime.AndroidRuntimeHost
import jp.kz2601.metamon.runtime.AppRuntime
import jp.kz2601.metamon.runtime.RuntimeHost
import jp.kz2601.metamon.dsl.decode
import jp.kz2601.metamon.dsl.fallback
import jp.kz2601.metamon.services.AIClient
import jp.kz2601.metamon.services.AppConfigClient
import jp.kz2601.metamon.services.AppEnvironment
import jp.kz2601.metamon.services.AuthService
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
import jp.kz2601.metamon.services.mock.MockAuthService
import jp.kz2601.metamon.services.sync.HotReload
import jp.kz2601.metamon.services.sync.RealtimeDocumentSync
import jp.kz2601.metamon.services.sync.SseChangeEventSource
import jp.kz2601.metamon.services.sync.asDocumentFetcher
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import java.io.File

/** 手書きの組み立て 1 か所(#354 §5。Hilt は使わない・iOS の `BuildConfig.make` と同じ形)。 */
class AppContainer(
    val environment: AppEnvironment,
    val auth: AuthService,
    val initialDsl: AppDSL,
    val appConfig: AppConfigClient,
    val ai: AIClient,
    /** 記録のローカル DB と送信待ちの列(#882)。[MetamonApplication] が start する */
    val recordSync: RecordSyncEngine,
    /** 最後に取得した DSL(オフライン起動・#883)。appconfig が取得のたびに入れる */
    val documentCache: DocumentCache,
    /** 他端末・共有の相手の変更(SSE + ポーリングの予備・#883) */
    val hotReload: HotReload,
    private val host: RuntimeHost,
) {
    companion object {
        fun make(context: Context): AppContainer {
            val environment = AppEnvironment(
                name = BuildConfig.METAMON_ENV,
                aiBaseUrl = BuildConfig.AI_BASE_URL,
                appConfigBaseUrl = BuildConfig.APPCONFIG_BASE_URL,
            )
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
            val log: (String) -> Unit = { Log.w("Metamon", it) }

            val documentCache = DocumentCache(File(context.filesDir, "metamon-dsl"))
            val appConfigHttp = http(environment.appConfigBaseUrl)
            val appConfig = HttpAppConfigClient(appConfigHttp, cache = documentCache)
            val sync = RealtimeDocumentSync(appConfig.asDocumentFetcher(), SseChangeEventSource(appConfigHttp), log = log)

            return AppContainer(
                environment = environment,
                auth = auth,
                initialDsl = loadInitialDsl(context),
                appConfig = appConfig,
                ai = HttpAIClient(http(environment.aiBaseUrl), log = log),
                recordSync = RecordSyncEngine.create(
                    client = appConfig,
                    connectivity = AndroidConnectivity(context),
                    directory = File(context.filesDir, "metamon-records"),
                    scope = CoroutineScope(SupervisorJob() + Dispatchers.Default),
                ),
                documentCache = documentCache,
                hotReload = HotReload(sync, CoroutineScope(SupervisorJob() + Dispatchers.Default)),
                host = AndroidRuntimeHost(context.applicationContext),
            )
        }

        /** 同梱の docs/dsl/examples/initial.json(話しかける前の画面)。 */
        private fun loadInitialDsl(context: Context): AppDSL = runCatching {
            context.assets.open("initial.json").bufferedReader().use { AppDSL.decode(it.readText()) }
        }.getOrElse {
            Log.e("Metamon", "initial.json を読めませんでした", it)
            AppDSL.fallback
        }
    }

    /** アプリ全体のランタイム(#889)。起動時に 1 つだけ作る。 */
    val runtime: AppRuntime by lazy {
        AppRuntime(
            document = AppDocument(dsl = initialDsl),
            recordSync = recordSync,
            scope = CoroutineScope(SupervisorJob() + Dispatchers.Main.immediate),
            host = host,
            currentUserId = { auth.currentUser.value?.uid },
            // appId の採番(#360)前は "local" のままなので、AppRuntime がサーバーに取りに行かない
            restore = { appId -> appConfig.fetchDocument(appId).document },
            hotReloader = hotReload,
            cachedDocument = { appId -> documentCache.load(appId)?.document },
        )
    }
}
