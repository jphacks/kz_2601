package jp.kz2601.metamon.renderer

import java.net.URI

/**
 * `openUrlButton` / `link` / `openUrl` アクションが開いてよい URL(iOS の `ExternalURLPolicy`・backend `dsl.OpenURLSchemes` と同じ allowlist)。
 */
object ExternalUrlPolicy {
    val allowedSchemes: Set<String> = setOf("https", "line", "slack", "discord", "tel", "mailto", "maps", "comgooglemaps")

    /** 許可されたスキームなら URL の文字列、そうでなければ null。 */
    fun openable(url: String?): String? {
        val scheme = url?.let(::scheme) ?: return null
        return url.takeIf { scheme in allowedSchemes }
    }

    /** `webView` は https で host があるものだけ。 */
    fun webViewUrl(url: String?): String? {
        val uri = url?.let { runCatching { URI(it) }.getOrNull() } ?: return null
        return url.takeIf { uri.scheme?.lowercase() == "https" && !uri.host.isNullOrEmpty() }
    }

    private fun scheme(url: String): String? = runCatching { URI(url) }.getOrNull()?.scheme?.lowercase()
}
