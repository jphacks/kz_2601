package jp.kz2601.metamon.desktop.net

import jp.kz2601.metamon.services.http.ClientInfo
import okhttp3.Interceptor
import okhttp3.Response

/** `X-Client-Platform` の値(#1748)。backend は ios / android に加えて windows / linux / macos を受ける。 */
enum class ClientPlatform(val header: String) {
    WINDOWS("windows"),
    LINUX("linux"),
    MACOS("macos");

    companion object {
        /** `os.name` から。Windows 10 / 11・Linux・Mac OS X。知らない OS(BSD など)は linux として申告する。 */
        fun from(osName: String): ClientPlatform {
            val n = osName.lowercase()
            return when {
                n.startsWith("windows") -> WINDOWS
                n.startsWith("mac") || n.startsWith("darwin") -> MACOS
                else -> LINUX
            }
        }

        fun current(): ClientPlatform = from(System.getProperty("os.name").orEmpty())
    }
}

/** User-Agent(診断用に OS の名前と版を入れる)。例 `TailorIt-Desktop/0.1.0 (Windows 11 10.0; amd64)` */
fun desktopUserAgent(
    version: String,
    osName: String = System.getProperty("os.name").orEmpty(),
    osVersion: String = System.getProperty("os.version").orEmpty(),
    arch: String = System.getProperty("os.arch").orEmpty(),
): String = "TailorIt-Desktop/$version ($osName $osVersion; $arch)"

/**
 * 共有の `MetamonHttp` は `X-Client-Platform: android` を付ける(`ClientInfo.PLATFORM`)。
 * android/ を変えずにデスクトップの値へ差し替えるため、OkHttp の段で上書きする。
 */
class DesktopClientInterceptor(
    private val platform: ClientPlatform,
    private val userAgent: String,
) : Interceptor {
    override fun intercept(chain: Interceptor.Chain): Response = chain.proceed(
        chain.request().newBuilder()
            .header(ClientInfo.PLATFORM_HEADER, platform.header)
            .header("User-Agent", userAgent)
            .build(),
    )
}
