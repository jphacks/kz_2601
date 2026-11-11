package jp.kz2601.metamon.dsl.logic

import java.net.URI
import java.net.URISyntaxException
import java.util.Locale

/**
 * `shareButton.shareTo` の送り先 preset。iOS の Model/ExternalLinks.swift の `ShareTarget`。
 * 描画は共有シート / LINE の共有 URL を使い分ける。ここは URL と文面だけ。
 */
enum class ShareTarget(val wire: String) {
    ANY("any"), LINE("line"), SLACK("slack"), DISCORD("discord");

    /** ボタンの既定ラベル(`label` 省略時・日本語。画面の言語の文言は描画側で引き直す)。 */
    val defaultLabel: String
        get() = when (this) {
            ANY -> "共有"
            LINE -> "LINE で送る"
            SLACK -> "Slack に送る"
            DISCORD -> "Discord に送る"
        }

    /** 送り先に合わせた文面。slack / discord は Markdown 記号を落とし、先頭に 💬 を付ける。line / any はそのまま。 */
    fun format(text: String): String = when (this) {
        ANY, LINE -> text
        SLACK, DISCORD -> "💬 " + plainText(text)
    }

    companion object {
        /** 未知・省略は ANY(backend は enum で弾くが端末は寛容)。 */
        fun of(raw: String?): ShareTarget = entries.firstOrNull { it.wire == raw } ?: ANY

        /**
         * LINE の共有画面を開く URL。英数字と `-._~` 以外は UTF-8 でパーセントエンコードする。
         * (iOS は Unicode の英数字を残し、URL(string:) が後でエンコードする。開く先と text の中身は同じ)
         */
        fun lineShareUrl(text: String): String {
            val sb = StringBuilder("https://line.me/R/share?text=")
            for (b in text.toByteArray(Charsets.UTF_8)) {
                val c = b.toInt() and 0xFF
                if (c in 'a'.code..'z'.code || c in 'A'.code..'Z'.code || c in '0'.code..'9'.code || c == '-'.code || c == '.'.code || c == '_'.code || c == '~'.code) {
                    sb.append(c.toChar())
                } else {
                    sb.append('%').append(String.format(Locale.ROOT, "%02X", c))
                }
            }
            return sb.toString()
        }

        private val lineMarker = Regex("""^[#>*\-]+\s+""")

        /** `**強調**` `__強調__` `` `code` `` と行頭の `# ` `> ` `- ` `* ` を外し、3 連以上の改行を 2 つに詰める。 */
        fun plainText(text: String): String {
            var s = text
            for (marker in listOf("**", "__", "`")) s = s.replace(marker, "")
            s = s.split("\n").joinToString("\n") { lineMarker.replaceFirst(it, "") }
            while (s.contains("\n\n\n")) s = s.replace("\n\n\n", "\n\n")
            return s.trimWhitespace()
        }
    }
}

/** `openUrlButton` / `link` / `openUrl` アクションが開いてよい URL の判定。backend `dsl.OpenURLSchemes` と同じ allowlist。 */
object ExternalUrlPolicy {
    val allowedSchemes: Set<String> = setOf("https", "line", "slack", "discord", "tel", "mailto", "maps", "comgooglemaps")

    private val schemePattern = Regex("^([A-Za-z][A-Za-z0-9+.\\-]*):")

    /** RFC 3986 のスキーム(小文字)。無ければ null。 */
    fun scheme(url: String): String? = schemePattern.find(url)?.groupValues?.get(1)?.lowercase(Locale.ROOT)

    fun isAllowed(url: String): Boolean = scheme(url)?.let { it in allowedSchemes } ?: false

    /** 許可されたスキームなら文字列をそのまま、そうでなければ null。 */
    fun openable(url: String?): String? = url?.takeIf { isAllowed(it) }

    /** `webView` は https でホストのあるものだけ。 */
    fun webViewUrl(url: String?): String? {
        if (url == null || scheme(url) != "https") return null
        val host = try {
            URI(url).host
        } catch (_: URISyntaxException) {
            null
        }
        return if (host.isNullOrEmpty()) null else url
    }
}
