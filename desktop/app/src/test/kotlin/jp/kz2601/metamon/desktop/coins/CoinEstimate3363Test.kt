package jp.kz2601.metamon.desktop.coins

import jp.kz2601.metamon.desktop.i18n.Lang
import jp.kz2601.metamon.desktop.i18n.StringTable
import jp.kz2601.metamon.desktop.i18n.Strings
import jp.kz2601.metamon.desktop.net.ClientPlatform
import jp.kz2601.metamon.desktop.net.DesktopApi
import kotlinx.coroutines.runBlocking
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/** 入力欄の下の「約 N コイン」(#3363): 目安があれば出す・無ければ出さない。数はサーバーの値だけ。 */
class CoinEstimate3363Test {
    private val enforce = """{"mode":"enforce","packs":[],"estimates":{"generate":30,"edit":12},"initialGrant":0,"proMonthly":0,"allowance":{},"overflow":{"blocks":{}}}"""

    @Test
    fun showsTheEstimateTheServerReturned() {
        val e = CoinEstimates.fromConfig(enforce)
        assertEquals(30, e.coins(CoinOperation.GENERATE))
        assertEquals(12, e.coins(CoinOperation.EDIT))
        val label = Strings.withLang(Lang.JA) { CoinEstimateLabel.of(e, CoinOperation.EDIT) }!!
        assertEquals("約 12 コイン", label.text)
        assertEquals("約 12 コイン使います", label.a11y)
        assertEquals("About 30 coins", Strings.withLang(Lang.EN) { CoinEstimateLabel.of(e, CoinOperation.GENERATE)!!.text })
    }

    @Test
    fun hiddenWithoutAnEstimate() {
        val e = CoinEstimates.fromConfig(enforce)
        assertNull("目安の無い処理", CoinEstimateLabel.of(e, CoinOperation.CHAT))
        assertNull("読めていない", CoinEstimateLabel.of(null, CoinOperation.EDIT))
        assertNull("0 は出さない", CoinEstimateLabel.of(CoinEstimates(mapOf("edit" to 0)), CoinOperation.EDIT))
    }

    @Test
    fun hiddenUnlessBillingIsEnforced() {
        for (mode in listOf("log", "off")) {
            assertNull(mode, CoinEstimateLabel.of(CoinEstimates.fromConfig(enforce.replace("enforce", mode)), CoinOperation.GENERATE))
        }
        assertNull("壊れた本文", CoinEstimateLabel.of(CoinEstimates.fromConfig("not json"), CoinOperation.GENERATE))
    }

    @Test
    fun acceptsOldKeysAndNestedValues() {
        val e = CoinEstimates.fromConfig("""{"mode":"ENFORCE","estimates":{"Create":25,"ai_chat":{"coins":4}}}""")
        assertEquals(25, e.coins(CoinOperation.GENERATE))
        assertEquals(4, e.coins(CoinOperation.TEXT))
    }

    @Test
    fun readsFromTheConfigEndpointAndHidesOnFailure() = runBlocking {
        val server = MockWebServer().apply { start() }
        try {
            val http = DesktopApi.http(server.url("/").toString(), DesktopApi.devTokens(null), ClientPlatform.WINDOWS, DesktopApi.okHttp(ClientPlatform.WINDOWS, "ua"))
            val source = HttpCoinEstimateSource(http)
            server.enqueue(MockResponse().setHeader("Content-Type", "application/json").setBody(enforce))
            assertEquals(12, source.estimates()?.coins(CoinOperation.EDIT))
            assertEquals("/v1/coins/config", server.takeRequest().path)
            server.enqueue(MockResponse().setResponseCode(404).setBody("{}"))
            assertNull(source.estimates())
        } finally {
            server.shutdown()
        }
    }

    @Test
    fun stringsResolveInBothLanguages() {
        val t = StringTable.fromClasspath()
        for (lang in Lang.entries) for (k in listOf("desktop_coin_estimate", "desktop_coin_estimate_a11y")) assertTrue("$k ($lang)", t.hasString(lang, k))
    }
}
