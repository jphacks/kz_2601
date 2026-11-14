package jp.kz2601.metamon.renderer.components

import okhttp3.Dns
import okhttp3.HttpUrl
import okhttp3.Interceptor
import okhttp3.Response
import java.io.IOException
import java.net.InetAddress
import java.net.UnknownHostException

/**
 * DSL の画像を読む先は公開のホストだけ(iOS の ImageHostPolicy と同じ規則)。
 * 名前: IP の直書き・localhost・*.local・*.internal・*.home.arpa・ドットの無い名前は読まない。
 * 解決の後: プライベート・ループバック・リンクローカル・CGNAT などの範囲は読まない。
 */
object ImageHosts {
    private val LOCAL_SUFFIXES = listOf("localhost", "local", "internal", "home.arpa")

    /** 名前の段階で読んでよいホストか。 */
    fun isPublicName(host: String?): Boolean {
        val h = host?.lowercase()?.trimEnd('.') ?: return false
        if (h.isEmpty() || h.contains(':') || h.startsWith('[')) return false
        val labels = h.split('.')
        if (labels.size < 2 || labels.any { it.isEmpty() }) return false
        // 最後が数字・0x で始まる名前は IPv4 の書き方(10 進 1 つ・8 / 16 進も含む)
        val last = labels.last()
        if (last.all { it.isDigit() } || last.startsWith("0x")) return false
        return LOCAL_SUFFIXES.none { h == it || h.endsWith(".$it") }
    }

    /** 解決した宛先が公開の範囲か。 */
    fun isPublicAddress(address: InetAddress): Boolean {
        if (address.isAnyLocalAddress || address.isLoopbackAddress || address.isLinkLocalAddress ||
            address.isSiteLocalAddress || address.isMulticastAddress
        ) {
            return false
        }
        val b = address.address.map { it.toInt() and 0xFF }
        return when (b.size) {
            4 -> isPublicV4(b)
            16 -> isPublicV6(b)
            else -> false
        }
    }

    private fun isPublicV4(b: List<Int>): Boolean = when {
        b[0] == 0 || b[0] == 10 || b[0] == 127 || b[0] >= 224 -> false
        b[0] == 100 && b[1] and 0xC0 == 64 -> false // CGNAT 100.64/10
        b[0] == 169 && b[1] == 254 -> false
        b[0] == 172 && b[1] and 0xF0 == 16 -> false
        b[0] == 192 && b[1] == 168 -> false
        b[0] == 192 && b[1] == 0 && (b[2] == 0 || b[2] == 2) -> false
        b[0] == 198 && b[1] and 0xFE == 18 -> false
        b[0] == 198 && b[1] == 51 && b[2] == 100 -> false
        b[0] == 203 && b[1] == 0 && b[2] == 113 -> false
        else -> true
    }

    private fun isPublicV6(b: List<Int>): Boolean {
        val v4 = { from: Int -> isPublicV4(b.subList(from, from + 4)) }
        return when {
            b[0] and 0xFE == 0xFC -> false // ユニークローカル fc00::/7
            b[0] == 0xFE && b[1] and 0x80 == 0x80 -> false // リンクローカル・サイトローカル fe80::/9
            b[0] == 0xFF -> false
            b.subList(0, 10).all { it == 0 } && b[10] == 0xFF && b[11] == 0xFF -> v4(12) // ::ffff:a.b.c.d
            b.subList(0, 12).all { it == 0 } -> false // :: ・::1・::a.b.c.d
            b.subList(0, 4) == listOf(0, 0x64, 0xFF, 0x9B) && b.subList(4, 12).all { it == 0 } -> v4(12) // NAT64
            b[0] == 0 && b[1] == 0x64 && b[2] == 0xFF && b[3] == 0x9B -> false // 64:ff9b:1::/48
            b[0] == 0x20 && b[1] == 0x02 -> v4(2) // 6to4
            b[0] == 0x20 && b[1] == 0x01 && b[2] == 0 && b[3] == 0 -> false // Teredo
            b[0] == 0x20 && b[1] == 0x01 && b[2] == 0x0D && b[3] == 0xB8 -> false // 文書用
            b[0] == 0x01 && b.subList(1, 8).all { it == 0 } -> false // 100::/64
            else -> true
        }
    }
}

/** 名前を解決した後、公開の範囲の宛先だけ返す(全部外れたら解決できない扱い)。 */
class PublicOnlyDns(private val base: Dns = Dns.SYSTEM) : Dns {
    override fun lookup(hostname: String): List<InetAddress> {
        if (!ImageHosts.isPublicName(hostname)) throw UnknownHostException("画像の読み先にしないホスト")
        return base.lookup(hostname).filter(ImageHosts::isPublicAddress)
            .ifEmpty { throw UnknownHostException("画像の読み先にしないホスト") }
    }
}

/**
 * 画像のリダイレクトを自分でたどる(接続の前に毎回 [allowed] で確かめる)。
 * OkHttp の自動のリダイレクトは切っておくこと([RemoteImages.imageClient])。
 */
class ImageRedirects(
    private val allowed: (HttpUrl) -> Boolean,
    private val maxRedirects: Int = MAX_REDIRECTS,
) : Interceptor {
    override fun intercept(chain: Interceptor.Chain): Response {
        var request = chain.request()
        if (!allowed(request.url)) throw IOException("画像の読み先にしない URL")
        for (i in 0..maxRedirects) {
            val response = chain.proceed(request)
            if (!response.isRedirect) return response
            val location = response.header("Location") ?: return response
            val next = response.request.url.resolve(location)
            response.close()
            if (next == null || !allowed(next)) throw IOException("画像の読み先にしない URL")
            request = request.newBuilder().url(next).build()
        }
        throw IOException("画像のリダイレクトが多すぎる")
    }

    companion object {
        const val MAX_REDIRECTS = 5
    }
}
