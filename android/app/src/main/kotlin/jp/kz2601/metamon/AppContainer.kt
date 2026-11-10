package jp.kz2601.metamon

import android.content.Context
import android.util.Log
import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.renderer.DSLRuntime
import jp.kz2601.metamon.services.AppEnvironment
import jp.kz2601.metamon.services.AuthService
import jp.kz2601.metamon.services.live.FirebaseAuthService
import jp.kz2601.metamon.services.live.FirebaseBootstrap
import jp.kz2601.metamon.services.mock.MockAuthService

/** 手書きの組み立て 1 か所(#354 §5。Hilt は使わない・iOS の `BuildConfig.make` と同じ形)。 */
class AppContainer(
    val environment: AppEnvironment,
    val auth: AuthService,
    val initialDsl: AppDSL,
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
            return AppContainer(environment, auth, loadInitialDsl(context))
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
