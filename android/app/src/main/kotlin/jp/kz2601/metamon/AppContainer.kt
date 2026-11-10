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
import jp.kz2601.metamon.services.mock.MockAuthService
import jp.kz2601.metamon.services.speech.AndroidSpeechTranscriber
import jp.kz2601.metamon.services.speech.SpeechTranscriber

/** 手書きの組み立て 1 か所(#354 §5。Hilt は使わない・iOS の `BuildConfig.make` と同じ形)。 */
class AppContainer(
    val environment: AppEnvironment,
    val auth: AuthService,
    val initialDsl: AppDSL,
    val appConfig: AppConfigClient,
    val ai: AIClient,
    val transcriber: SpeechTranscriber,
    val appId: DeviceAppId,
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
            val language = {
                val locales = LocaleList.getDefault()
                ClientInfo.languageCode((0 until locales.size()).map(locales::get))
            }
            val device = DeviceContext(language = language)
            // App Check(Play Integrity)はまだ無い: ヘッダ無しで送る(backend が enforce の環境では 401 になる)
            val client = MetamonHttp.defaultClient()
            fun http(baseUrl: String) = MetamonHttp(baseUrl, tokens, clientInfo, appCheck = null, client = client, device = device)
            val log: (String) -> Unit = { Log.w("Metamon", it) }

            return AppContainer(
                environment = environment,
                auth = auth,
                initialDsl = loadInitialDsl(context),
                appConfig = HttpAppConfigClient(http(environment.appConfigBaseUrl)),
                ai = HttpAIClient(http(environment.aiBaseUrl), log = log),
                // 声は端末内だけで文字にする(#354 §3)。端末内が無い端末は文字入力に落とす
                transcriber = AndroidSpeechTranscriber(context, language = language, log = { Log.i("Metamon", it) }),
                appId = DeviceAppId(context),
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
