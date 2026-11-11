package jp.kz2601.metamon.dsl.logic

import java.net.URI
import java.net.URISyntaxException
import java.util.Locale

/**
 * `shareButton.shareTo` の送り先 preset。iOS の Model/ExternalLinks.swift の `ShareTarget`。
 * 描画は共有シート / 直接開く URL(LINE・X・メール)を使い分ける。ここは URL と文面だけ。
 * x / mail は iOS の #989 で足した preset(生成した型 `ShareTo` にはまだ無いので、文字列から [of] で読む)。
 */
enum class ShareTarget(val wire: String) {
    ANY("any"), LINE("line"), SLACK("slack"), DISCORD("discord"), X("x"), MAIL("mail");

    /** ボタンの既定ラベル(`label` 省略時・日本語。画面の言語の文言は描画側で引き直す)。 */
    val defaultLabel: String
        get() = when (this) {
            ANY -> "共有"
            LINE -> "LINE で送る"
            SLACK -> "Slack に送る"
            DISCORD -> "Discord に送る"
            X -> "X に投稿"
            MAIL -> "メールで送る"
        }

    /** 既定のアイコン(SF Symbols の名前・`icon` 省略時)。 */
    val defaultIcon: String
        get() = when (this) {
            ANY -> "square.and.arrow.up"
            LINE -> "bubble.left.fill"
            SLACK -> "number"
            DISCORD -> "gamecontroller"
            X -> "square.and.pencil"
            MAIL -> "envelope"
        }

    /** 文面を直接渡せる送り先の URL(line / x / mail)。それ以外(共有シート)は null。写真は渡せない。 */
    fun directUrl(text: String): String? = when (this) {
        LINE -> lineShareUrl(text)
        X -> xPostUrl(text)
        MAIL -> mailUrl(to = null, body = text)
        ANY, SLACK, DISCORD -> null
    }

    /** 送り先に合わせた文面。slack / discord は Markdown 記号を落とし、先頭に 💬 を付ける。それ以外はそのまま。 */
    fun format(text: String): String = when (this) {
        ANY, LINE, X, MAIL -> text
        SLACK, DISCORD -> "💬 " + plainText(text)
    }

    companion object {
        /** 未知・省略は ANY(backend は enum で弾くが端末は寛容)。 */
        fun of(raw: String?): ShareTarget = entries.firstOrNull { it.wire == raw } ?: ANY

        /**
         * LINE の共有画面を開く URL。英数字と `-._~` 以外は UTF-8 でパーセントエンコードする。
         * (iOS は Unicode の英数字を残し、URL(string:) が後でエンコードする。開く先と text の中身は同じ)
         */
        fun lineShareUrl(text: String): String = "https://line.me/R/share?text=" + percentEncoded(text)

        /** X の投稿画面(Web Intent。アプリがあればアプリ、無ければ Web)。 */
        fun xPostUrl(text: String): String = "https://x.com/intent/tweet?text=" + percentEncoded(text)

        /** メールの新規作成(RFC 6068 `mailto:<to>?body=`)。宛先は形を確かめたアドレスだけ入れる。 */
        fun mailUrl(to: String?, body: String?): String {
            val sb = StringBuilder("mailto:")
            if (to != null && OpenApp.isEmail(to)) sb.append(percentEncoded(to, keep = "@"))
            if (!body.isNullOrEmpty()) sb.append("?body=").append(percentEncoded(body))
            return sb.toString()
        }

        /** RFC 3986 の非予約文字(英数字と `-._~`)と [keep] 以外を UTF-8 で % エンコードする(`&` `=` `?` `#` `+` 空白も)。 */
        fun percentEncoded(text: String, keep: String = ""): String {
            val sb = StringBuilder()
            for (b in text.toByteArray(Charsets.UTF_8)) {
                val c = b.toInt() and 0xFF
                if (isUnreserved(c) || (c < 0x80 && keep.indexOf(c.toChar()) >= 0)) {
                    sb.append(c.toChar())
                } else {
                    sb.append('%').append(String.format(Locale.ROOT, "%02X", c))
                }
            }
            return sb.toString()
        }

        private fun isUnreserved(c: Int): Boolean =
            c in 'a'.code..'z'.code || c in 'A'.code..'Z'.code || c in '0'.code..'9'.code ||
                c == '-'.code || c == '.'.code || c == '_'.code || c == '~'.code

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
