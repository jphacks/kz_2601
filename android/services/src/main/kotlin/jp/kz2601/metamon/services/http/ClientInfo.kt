package jp.kz2601.metamon.services.http

import java.util.Locale
import java.util.TimeZone

/**
 * この版の申告(iOS の `ClientInfo`・#302)。backend への全リクエストにヘッダで付ける。
 * backend はログに出すだけ(backend/internal/httpx/client.go)。部品の絞り込みは body の `allowedComponents`。
 */
data class ClientInfo(
    /** versionName(例 0.1.0) */
    val version: String,
    /** versionCode(例 12)。最低対応版の比較はこちら */
    val build: String,
    val dslVersion: Int = SUPPORTED_DSL_VERSION,
) {
    val headers: Map<String, String>
        get() = mapOf(
            PLATFORM_HEADER to PLATFORM,
            "X-Client-Version" to version,
            "X-Client-Build" to build,
            "X-Client-DSL-Version" to dslVersion.toString(),
        )

    companion object {
        const val PLATFORM = "android"
        const val PLATFORM_HEADER = "X-Client-Platform"
        const val SUPPORTED_DSL_VERSION = 1

        /** UI の言語("ja" | "en")。好みの言語を順に見て最初に当たった方。どちらも無ければ ja(iOS と同じ)。 */
        fun languageCode(preferred: List<Locale> = listOf(Locale.getDefault())): String =
            preferred.firstNotNullOfOrNull { locale ->
                when (locale.language) {
                    "ja" -> "ja"
                    "en" -> "en"
                    else -> null
                }
            } ?: "ja"
    }
}

/** リクエストごとに読む端末の設定(テストで差し替える)。 */
data class DeviceContext(
    /** `X-Time-Zone`(定期のルールの時刻・「今日」の基準・#342) */
    val timeZone: () -> String = { TimeZone.getDefault().id },
    /** `Accept-Language`(自動の動きの説明などの言語) */
    val language: () -> String = { ClientInfo.languageCode() },
)
