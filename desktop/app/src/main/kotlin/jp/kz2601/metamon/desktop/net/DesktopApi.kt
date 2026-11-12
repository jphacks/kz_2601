package jp.kz2601.metamon.desktop.net

import jp.kz2601.metamon.services.AppConfigClient
import jp.kz2601.metamon.services.http.ClientInfo
import jp.kz2601.metamon.services.http.MetamonHttp
import jp.kz2601.metamon.services.http.TokenProvider
import jp.kz2601.metamon.services.live.HttpAppConfigClient
import jp.kz2601.metamon.services.local.DocumentCache
import okhttp3.OkHttpClient

/**
 * 接続先。既定はモック(通信しない)。サインイン(#1749)が入るまで、本物の API は手元の確かめにだけ使う。
 *
 * 環境変数:
 * - `TAILORIT_API=stg` で stg の appconfig へ(prod はまだ選べない)
 * - `TAILORIT_APPCONFIG_URL` で URL を差し替え(手元の backend など)
 * - `TAILORIT_DEV_TOKEN` に Firebase の ID トークン(手で取ったもの)。リポジトリ・ファイルには書かない
 */
data class DesktopApiConfig(
    val mode: Mode = Mode.MOCK,
    val appConfigUrl: String = STG_APPCONFIG_URL,
    val devToken: String? = null,
) {
    enum class Mode { MOCK, STG }

    val isLive: Boolean get() = mode != Mode.MOCK

    /** ログ・画面に出してよい形(トークンは伏せる)。 */
    override fun toString(): String =
        "DesktopApiConfig(mode=$mode, appConfigUrl=$appConfigUrl, devToken=${if (devToken.isNullOrEmpty()) "none" else "***"})"

    companion object {
        /** android/app の stg flavor と同じ */
        const val STG_APPCONFIG_URL = "https://appconfig-stg-378953239426.asia-northeast1.run.app"

        fun fromEnvironment(env: Map<String, String> = System.getenv()): DesktopApiConfig {
            val mode = when (env["TAILORIT_API"]?.trim()?.lowercase()) {
                "stg" -> Mode.STG
                else -> Mode.MOCK
            }
            return DesktopApiConfig(
                mode = mode,
                appConfigUrl = env["TAILORIT_APPCONFIG_URL"]?.takeIf { it.isNotBlank() } ?: STG_APPCONFIG_URL,
                devToken = env["TAILORIT_DEV_TOKEN"]?.trim()?.takeIf { it.isNotEmpty() },
            )
        }
    }
}

/** API クライアントの組み立て(Android の AppContainer の http 部分に当たる)。クライアント本体は android/services と同じソース。 */
object DesktopApi {
    /** 版の申告。jpackage の packageVersion と合わせる */
    const val VERSION = "0.1.0"
    const val BUILD = "1"

    fun clientInfo(): ClientInfo = ClientInfo(version = VERSION, build = BUILD)

    fun okHttp(
        platform: ClientPlatform = ClientPlatform.current(),
        userAgent: String = desktopUserAgent(VERSION),
    ): OkHttpClient = MetamonHttp.defaultClient().newBuilder()
        .addInterceptor(DesktopClientInterceptor(platform, userAgent))
        .build()

    /** サインインが入るまでの Bearer。トークンが無ければ Authorization を付けない */
    fun devTokens(token: String?): TokenProvider = TokenProvider { token }

    fun http(
        baseUrl: String,
        tokens: TokenProvider,
        platform: ClientPlatform = ClientPlatform.current(),
        client: OkHttpClient = okHttp(platform),
    ): MetamonHttp = MetamonHttp(baseUrl = baseUrl, tokens = tokens, clientInfo = clientInfo(), client = client)

    /** モックなら null(画面はメモリのモックのまま)。 */
    fun appConfig(config: DesktopApiConfig, cache: DocumentCache? = null): AppConfigClient? {
        if (!config.isLive) return null
        return HttpAppConfigClient(http(config.appConfigUrl, devTokens(config.devToken)), cache)
    }
}
