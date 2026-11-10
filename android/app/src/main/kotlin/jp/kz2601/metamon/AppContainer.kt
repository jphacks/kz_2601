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
import jp.kz2601.metamon.services.http.AuthTokenProvider
import jp.kz2601.metamon.services.http.ClientInfo
import jp.kz2601.metamon.services.http.DeviceContext
import jp.kz2601.metamon.services.http.MetamonHttp
import jp.kz2601.metamon.services.live.FirebaseAuthService
import jp.kz2601.metamon.services.live.FirebaseBootstrap
import jp.kz2601.metamon.services.live.HttpAIClient
import jp.kz2601.metamon.services.live.HttpAppConfigClient
import jp.kz2601.metamon.services.local.AndroidConnectivity
import jp.kz2601.metamon.services.local.RecordSyncEngine
import jp.kz2601.metamon.services.mock.MockAuthService
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

            val appConfig = HttpAppConfigClient(http(environment.appConfigBaseUrl))

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
}

/** 描画器に渡す能力の実装(iOS の `AppRuntime`)。 */
class AppRuntime(private val onVoice: (String?) -> Unit) : DSLRuntime {
    override fun onVoiceInput(componentId: String?) = onVoice(componentId)
}
