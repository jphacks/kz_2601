package jp.kz2601.metamon.dsl.logic

import java.net.URI
import java.text.Normalizer

/**
 * `openUrlButton.app`: 名前で開くアプリ・サービス。iOS の Model/OpenApp.swift(#989)・backend `dsl.OpenApps` の写し。
 * 公開されている形のリンクだけを作る。`to` が形に合わなければそのサービスのトップ(phone だけは番号が無いと開かない)。
 */
enum class OpenApp(val wire: String) {
    PAYPAY("paypay"), LINE("line"), SLACK("slack"), DISCORD("discord"), X("x"),
    INSTAGRAM("instagram"), MAPS("maps"), MAIL("mail"), PHONE("phone");

    /** ボタンの既定ラベル(`label` 省略時)。 */
    val defaultLabel: String
        get() = when (this) {
            PAYPAY -> "PayPay を開く"
            LINE -> "LINE を開く"
            SLACK -> "Slack を開く"
            DISCORD -> "Discord を開く"
            X -> "X を開く"
            INSTAGRAM -> "Instagram を開く"
            MAPS -> "地図で開く"
            MAIL -> "メールを書く"
            PHONE -> "電話をかける"
        }

    /** 既定のアイコン(SF Symbols の名前・`icon` 省略時)。 */
    val defaultIcon: String
        get() = when (this) {
            PAYPAY -> "yensign.circle"
            LINE -> "bubble.left.fill"
            SLACK -> "number"
            DISCORD -> "gamecontroller"
            X -> "at"
            INSTAGRAM -> "camera"
            MAPS -> "map"
            MAIL -> "envelope"
            PHONE -> "phone"
        }

    /** `to` に書けるリンクのホスト(サブドメインも可)。 */
    private val linkHosts: List<String>
        get() = when (this) {
            PAYPAY -> listOf("paypay.ne.jp")
            LINE -> listOf("line.me", "lin.ee")
            SLACK -> listOf("slack.com")
            DISCORD -> listOf("discord.com", "discord.gg")
            X -> listOf("x.com", "twitter.com")
            INSTAGRAM -> listOf("instagram.com")
            MAPS, MAIL, PHONE -> emptyList()
        }

    /** そのサービスのトップ(`to` が無い・形に合わない時)。 */
    val home: List<String>
        get() = when (this) {
            PAYPAY -> listOf("https://paypay.ne.jp/")
            LINE -> listOf("https://line.me/R/nv/chat")
            SLACK -> listOf("slack://open", "https://slack.com/")
            DISCORD -> listOf("https://discord.com/app")
            X -> listOf("https://x.com/")
            INSTAGRAM -> listOf("https://www.instagram.com/")
            MAPS -> listOf("https://maps.apple.com/")
            MAIL -> listOf("mailto:")
            PHONE -> emptyList()
        }

    /** 開く候補。先頭から開き、開けなければ次を試す(slack:// が無い端末は Web)。空 = 開ける物が無い。 */
    fun urls(to: String?): List<String> {
        val t = to.orEmpty().trimWhitespace()
        if (t.isEmpty()) return home
        link(t)?.let { return listOf(it) }
        return when (this) {
            X -> username(t, max = 15, extra = "_")?.let { listOf("https://x.com/$it") } ?: home
            INSTAGRAM -> username(t, max = 30, extra = "._")?.let { listOf("https://www.instagram.com/$it/") } ?: home
            MAPS -> listOf("https://maps.apple.com/?q=" + ShareTarget.percentEncoded(t))
            MAIL -> listOf(ShareTarget.mailUrl(to = t, body = null))
            PHONE -> phoneNumber(t)?.let { listOf("tel:$it") } ?: emptyList()
            PAYPAY, LINE, SLACK, DISCORD -> home
        }
    }

    /** `to` がこのサービスの https リンクならその文字列(user@host・別ホスト・http は null)。 */
    fun link(s: String): String? {
        if (linkHosts.isEmpty()) return null
        val uri = runCatching { URI(s) }.getOrNull() ?: return null
        if (uri.scheme != "https" || uri.rawUserInfo != null) return null
        val host = uri.host?.lowercase() ?: return null
        return s.takeIf { linkHosts.any { h -> host == h || host.endsWith(".$h") } }
    }

    companion object {
        /** 未知・省略は null(backend は enum で弾く)。 */
        fun of(raw: String?): OpenApp? = entries.firstOrNull { it.wire == raw }

        /** `@name` / `name`(英数字と [extra] の記号・[max] 文字まで)→ name。 */
        fun username(s: String, max: Int, extra: String): String? {
            val name = s.removePrefix("@")
            if (name.length !in 1..max) return null
            val ok = name.all { it in 'a'..'z' || it in 'A'..'Z' || it in '0'..'9' || it in extra }
            return name.takeIf { ok }
        }

        /** 電話番号 → tel: に入れる形(先頭の + と数字だけ)。全角は半角に。数字が 2〜20 桁でない・他の文字が混ざれば null。 */
        fun phoneNumber(s: String): String? {
            val n = Normalizer.normalize(s, Normalizer.Form.NFKC)
            if (!n.all { it in "+0123456789-() " }) return null
            val digits = n.filter { it in '0'..'9' }
            if (digits.length !in 2..20) return null
            return (if (n.startsWith("+")) "+" else "") + digits
        }

        private val emailPattern = Regex("""(?U)^[^@\s/?#&]+@[^@\s/?#&]+\.[^@\s/?#&]+$""")

        fun isEmail(s: String): Boolean = emailPattern.matches(s)
    }
}
