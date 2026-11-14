package jp.kz2601.metamon.dsl.logic

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Test
import java.net.URI
import java.net.URLDecoder

/** iOS の OpenAppTests(#989)の写し: `openUrlButton.app` と `shareButton.shareTo` x / mail の URL 作り・形に合わない時のトップ。 */
class OpenAppTest {
    private fun urls(app: OpenApp, to: String?) = app.urls(to)

    @Test
    fun lenientDecode() {
        assertEquals(OpenApp.PAYPAY, OpenApp.of("paypay"))
        assertNull(OpenApp.of("telegram"))
        assertNull(OpenApp.of(null))
    }

    @Test
    fun homeWithoutTo() {
        assertEquals(listOf("https://paypay.ne.jp/"), urls(OpenApp.PAYPAY, null))
        assertEquals(listOf("https://line.me/R/nv/chat"), urls(OpenApp.LINE, "  "))
        assertEquals("アプリが無ければ Web", listOf("slack://open", "https://slack.com/"), urls(OpenApp.SLACK, null))
        assertEquals(listOf("https://discord.com/app"), urls(OpenApp.DISCORD, null))
        assertEquals(listOf("https://maps.apple.com/"), urls(OpenApp.MAPS, null))
        assertEquals(listOf("mailto:"), urls(OpenApp.MAIL, null))
        assertEquals("番号が無ければ開かない", emptyList<String>(), urls(OpenApp.PHONE, null))
    }

    @Test
    fun serviceLinks() {
        assertEquals(listOf("https://qr.paypay.ne.jp/p2p01_abc"), urls(OpenApp.PAYPAY, "https://qr.paypay.ne.jp/p2p01_abc"))
        assertEquals(listOf("https://line.me/R/ti/p/%40shop"), urls(OpenApp.LINE, "https://line.me/R/ti/p/%40shop"))
        assertEquals(listOf("https://team.slack.com/archives/C1"), urls(OpenApp.SLACK, "https://team.slack.com/archives/C1"))
        assertEquals(listOf("https://discord.gg/abc"), urls(OpenApp.DISCORD, "https://discord.gg/abc"))
    }

    @Test
    fun foreignLinksFallBackToHome() {
        for (to in listOf("https://paypay.ne.jp.evil.example/", "http://qr.paypay.ne.jp/x", "https://user@paypay.ne.jp/", "javascript:alert(1)", "paypay://pay")) {
            assertEquals(to, listOf("https://paypay.ne.jp/"), urls(OpenApp.PAYPAY, to))
        }
        assertEquals(listOf("slack://open", "https://slack.com/"), urls(OpenApp.SLACK, "https://slack.com.evil.example/"))
    }

    @Test
    fun usernames() {
        assertEquals(listOf("https://x.com/jphacks"), urls(OpenApp.X, "@jphacks"))
        assertEquals(listOf("https://x.com/jphacks"), urls(OpenApp.X, "https://x.com/jphacks"))
        assertEquals("パスを足させない", listOf("https://x.com/"), urls(OpenApp.X, "a/b?c"))
        assertEquals(listOf("https://www.instagram.com/some.user_1/"), urls(OpenApp.INSTAGRAM, "some.user_1"))
        assertEquals(listOf("https://www.instagram.com/"), urls(OpenApp.INSTAGRAM, "日本語"))
    }

    @Test
    fun mapsQueryIsEncoded() {
        val url = URI(urls(OpenApp.MAPS, "金沢駅 東口&b=1").single())
        assertEquals("maps.apple.com", url.host)
        val q = url.rawQuery.split("&")
        assertEquals(1, q.size)
        assertEquals("q", q[0].substringBefore("="))
        assertEquals("金沢駅 東口&b=1", URLDecoder.decode(q[0].substringAfter("="), Charsets.UTF_8))
    }

    @Test
    fun mailAndPhone() {
        assertEquals(listOf("mailto:a.b@example.com"), urls(OpenApp.MAIL, "a.b@example.com"))
        assertEquals("クエリを足させない", listOf("mailto:"), urls(OpenApp.MAIL, "a@b.c?cc=x@y.z"))
        assertEquals(listOf("tel:0761234567"), urls(OpenApp.PHONE, "076-123-4567"))
        assertEquals(listOf("tel:+81761234567"), urls(OpenApp.PHONE, "+81 (76) 123 4567"))
        assertEquals("全角数字も", listOf("tel:076123"), urls(OpenApp.PHONE, "０７６１２３"))
        assertEquals(emptyList<String>(), urls(OpenApp.PHONE, "call me"))
        assertEquals(emptyList<String>(), urls(OpenApp.PHONE, "1;x"))
    }

    @Test
    fun defaults() {
        assertEquals("PayPay を開く", OpenApp.PAYPAY.defaultLabel)
        for (app in OpenApp.entries) {
            assertFalse(app.defaultLabel.isEmpty())
            assertFalse(app.defaultIcon.isEmpty())
        }
    }

    // shareTo x / mail

    @Test
    fun shareTargetNewPresets() {
        assertEquals(ShareTarget.X, ShareTarget.of("x"))
        assertEquals(ShareTarget.MAIL, ShareTarget.of("mail"))
        assertEquals("X に投稿", ShareTarget.X.defaultLabel)
        assertNull(ShareTarget.ANY.directUrl("a"))
        assertNull("Slack は共有シート", ShareTarget.SLACK.directUrl("a"))
        assertEquals("line.me", ShareTarget.LINE.directUrl("a")?.let { URI(it).host })
    }

    @Test
    fun xPostUrlEncodesText() {
        val s = ShareTarget.X.directUrl("ランチ 1200円 #割り勘 a&b=c+d")!!
        val url = URI(s)
        assertEquals("x.com", url.host)
        assertEquals("/intent/tweet", url.path)
        val q = url.rawQuery.split("&")
        assertEquals(1, q.size)
        assertEquals("text", q[0].substringBefore("="))
        assertEquals("ランチ 1200円 #割り勘 a&b=c+d", URLDecoder.decode(q[0].substringAfter("="), Charsets.UTF_8))
        assertFalse("# はフラグメントにしない", s.contains("#"))
    }

    @Test
    fun mailBody() {
        assertEquals(
            "mailto:?body=%E5%90%88%E8%A8%88%203000%E5%86%86%0A1%20%E4%BA%BA%201000%E5%86%86",
            ShareTarget.MAIL.directUrl("合計 3000円\n1 人 1000円"),
        )
        assertEquals("mailto:a@b.jp?body=x%20y", ShareTarget.mailUrl(to = "a@b.jp", body = "x y"))
    }

    @Test
    fun formatKeepsText() {
        assertEquals("**a**", ShareTarget.X.format("**a**"))
        assertEquals("**a**", ShareTarget.MAIL.format("**a**"))
    }

    // Android の地図の写し

    @Test
    fun mapsUrlsBecomeGoogleMaps() {
        assertEquals("https://www.google.com/maps/search/?api=1&query=%E9%87%91%E6%B2%A2", MapsUrl.forAndroid("maps:?q=%E9%87%91%E6%B2%A2"))
        assertEquals("https://www.google.com/maps/search/?api=1&query=Kanazawa+Station", MapsUrl.forAndroid("https://maps.apple.com/?q=Kanazawa+Station"))
        assertEquals("https://www.google.com/maps/search/?api=1&query=36.57,136.65", MapsUrl.forAndroid("comgooglemaps://?center=36.57,136.65&zoom=14"))
        assertEquals(
            "https://www.google.com/maps/dir/?api=1&destination=Kenrokuen&origin=Kanazawa",
            MapsUrl.forAndroid("maps://?saddr=Kanazawa&daddr=Kenrokuen"),
        )
        assertEquals(MapsUrl.GOOGLE_MAPS_HOME, MapsUrl.forAndroid("https://maps.apple.com/"))
        assertEquals("アプリの地図のトップも", MapsUrl.GOOGLE_MAPS_HOME, MapsUrl.forAndroid(OpenApp.MAPS.home.single()))
        assertEquals("地図以外はそのまま", "https://example.com/?q=a", MapsUrl.forAndroid("https://example.com/?q=a"))
        assertEquals("tel:0761234567", MapsUrl.forAndroid("tel:0761234567"))
    }

    @Test
    fun mapsAppQueryRoundTripsToGoogle() {
        val apple = OpenApp.MAPS.urls("金沢駅 東口&b=1").single()
        val google = URI(MapsUrl.forAndroid(apple))
        assertEquals("www.google.com", google.host)
        val query = google.rawQuery.split("&").first { it.startsWith("query=") }.substringAfter("=")
        assertEquals("金沢駅 東口&b=1", URLDecoder.decode(query, Charsets.UTF_8))
    }
}
