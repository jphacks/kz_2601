package jp.kz2601.metamon.dsl.logic

import java.util.Locale

/**
 * 地図の URL を Android で開ける形に写す表。DSL・iOS は Apple の形(`maps:` / `https://maps.apple.com/`)と
 * iOS 版 Google マップのスキーム(`comgooglemaps://`)を使うが、Android にはどちらも無いので Google マップの URL にする
 * (Google マップのアプリがあればアプリ、無ければブラウザで開く)。
 *
 * | 元のキー | Google マップの URL |
 * |---|---|
 * | `daddr`(+ `saddr`) | `https://www.google.com/maps/dir/?api=1&destination=…&origin=…` |
 * | `q` / `address` | `https://www.google.com/maps/search/?api=1&query=…` |
 * | `ll` / `center`(緯度,経度) | `https://www.google.com/maps/search/?api=1&query=…` |
 * | どれも無い | `https://www.google.com/maps/` |
 *
 * 値はエンコードされたまま移す(デコードしてから作り直さない)。地図以外の URL はそのまま返す。
 */
object MapsUrl {
    const val GOOGLE_MAPS_HOME = "https://www.google.com/maps/"

    /** Apple の地図・iOS の Google マップの URL か。 */
    fun isAppleOrIosMaps(url: String): Boolean {
        val scheme = ExternalUrlPolicy.scheme(url) ?: return false
        if (scheme == "maps" || scheme == "comgooglemaps") return true
        if (scheme != "https") return false
        val host = url.substringAfter("://").substringBefore('/').substringBefore('?').substringBefore('#')
        return host.lowercase(Locale.ROOT) == "maps.apple.com"
    }

    /** 地図の URL なら Google マップの URL、それ以外はそのまま。 */
    fun forAndroid(url: String): String {
        if (!isAppleOrIosMaps(url)) return url
        val params = query(url)
        fun p(vararg keys: String): String? = keys.firstNotNullOfOrNull { k -> params[k]?.takeIf { it.isNotEmpty() } }
        p("daddr")?.let { dest ->
            val origin = p("saddr")?.let { "&origin=$it" }.orEmpty()
            return "https://www.google.com/maps/dir/?api=1&destination=$dest$origin"
        }
        p("q", "address", "ll", "center")?.let { return "https://www.google.com/maps/search/?api=1&query=$it" }
        return GOOGLE_MAPS_HOME
    }

    /** `?` の後・`#` の前の `key=value`(値はエンコードされたまま・同じキーは最初の 1 つ)。 */
    private fun query(url: String): Map<String, String> {
        val q = url.substringAfter('?', "").substringBefore('#')
        if (q.isEmpty()) return emptyMap()
        val out = linkedMapOf<String, String>()
        for (pair in q.split('&')) {
            val key = pair.substringBefore('=')
            if (key.isNotEmpty() && key !in out) out[key] = pair.substringAfter('=', "")
        }
        return out
    }
}
