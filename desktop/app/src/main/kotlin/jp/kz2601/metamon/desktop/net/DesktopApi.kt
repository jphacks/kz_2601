package jp.kz2601.metamon.desktop.net

import jp.kz2601.metamon.desktop.create.GenerateSource
import jp.kz2601.metamon.desktop.create.HttpGenerateSource
import jp.kz2601.metamon.services.AIClient
import jp.kz2601.metamon.services.AppConfigClient
import jp.kz2601.metamon.services.consent.ConsentGatedAiClient
import jp.kz2601.metamon.services.http.ClientInfo
import jp.kz2601.metamon.services.http.MetamonHttp
import jp.kz2601.metamon.services.http.TokenProvider
import jp.kz2601.metamon.services.live.HttpAIClient
import jp.kz2601.metamon.services.live.HttpAppConfigClient
import jp.kz2601.metamon.services.local.DocumentCache
import okhttp3.OkHttpClient

/**
 * 接続先。既定はモック(通信しない)。サインイン(#1749)が入るまで、本物の API は手元の確かめにだけ使う。
 *
 * 環境変数:
 * - `TAILORIT_API=stg` で stg の appconfig へ(prod はまだ選べない)
 * - `TAILORIT_APPCONFIG_URL` で URL を差し替え(手元の backend など)
 * - `TAILORIT_AI_URL` で ai の URL を差し替え(#2160)。stg なら既定は stg の ai。モックでもこれを渡すと会話・作るだけはその URL へ送る
 * - `TAILORIT_DEV_TOKEN` に Firebase の ID トークン(手で取ったもの)。リポジトリ・ファイルには書かない
 */
data class DesktopApiConfig(
    val mode: Mode = Mode.MOCK,
    val appConfigUrl: String = STG_APPCONFIG_URL,
    val devToken: String? = null,
    /** ai(会話・作る)の接続先。null なら AI を使う入口を出さない(モックの既定) */
    val aiUrl: String? = null,
) {
    enum class Mode { MOCK, STG }

    val isLive: Boolean get() = mode != Mode.MOCK

    /** ログ・画面に出してよい形(トークンは伏せる)。 */
    override fun toString(): String =
        "DesktopApiConfig(mode=$mode, appConfigUrl=$appConfigUrl, aiUrl=${aiUrl ?: "none"}, devToken=${if (devToken.isNullOrEmpty()) "none" else "***"})"

    companion object {
        /** android/app の stg flavor と同じ */
        const val STG_APPCONFIG_URL = "https://appconfig-stg-378953239426.asia-northeast1.run.app"

        /** android/app の stg flavor の AI_BASE_URL と同じ */
        const val STG_AI_URL = "https://ai-stg-378953239426.asia-northeast1.run.app"

        fun fromEnvironment(env: Map<String, String> = System.getenv()): DesktopApiConfig {
            val mode = when (env["TAILORIT_API"]?.trim()?.lowercase()) {
                "stg" -> Mode.STG
                else -> Mode.MOCK
            }
            return DesktopApiConfig(
                mode = mode,
                appConfigUrl = env["TAILORIT_APPCONFIG_URL"]?.takeIf { it.isNotBlank() } ?: STG_APPCONFIG_URL,
                devToken = env["TAILORIT_DEV_TOKEN"]?.trim()?.takeIf { it.isNotEmpty() },
                aiUrl = env["TAILORIT_AI_URL"]?.trim()?.takeIf { it.isNotEmpty() } ?: STG_AI_URL.takeIf { mode == Mode.STG },
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

    /**
     * 会話(`/v1/chat`)・作る(`/v1/generate`)のクライアント(#2160)。接続先が無ければ null(モック・通信しない)。
     * 同意([isGranted])が無い間は送らない(android/services の ConsentGatedAiClient で包む)。
     */
    fun ai(config: DesktopApiConfig, isGranted: () -> Boolean, cache: DocumentCache? = null): AIClient? {
        val url = config.aiUrl ?: return null
        return ConsentGatedAiClient(HttpAIClient(http(url, devTokens(config.devToken)), cache = cache), isGranted)
    }

    /** 作る(`/v1/generate?stream=1`)の行の出どころ(#2163)。接続先が無ければ null。同意が無い間は送らない。 */
    fun generate(config: DesktopApiConfig, isGranted: () -> Boolean): GenerateSource? {
        val url = config.aiUrl ?: return null
        return HttpGenerateSource(http(url, devTokens(config.devToken)), isGranted)
    }
}
