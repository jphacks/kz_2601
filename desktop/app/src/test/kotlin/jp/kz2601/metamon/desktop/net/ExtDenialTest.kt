package jp.kz2601.metamon.desktop.net

import jp.kz2601.metamon.desktop.net.ExtDenialRoute.Reason
import jp.kz2601.metamon.services.http.ApiError
import jp.kz2601.metamon.services.http.PaidFeature
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.jsonObject
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

/** 外の API の断り(#2001)から「次にすること」(iOS の ExtDenialRoute と同じ判定)。 */
class ExtDenialTest {
    private fun err(status: Int, code: String, details: String = "{}") =
        ApiError.Server(status, code, "", Json.parseToJsonElement(details).jsonObject)

    @Test
    fun notConnectedGoesToConnect() {
        assertEquals(ExtDenialRoute.Connect("google"), ExtDenialRoute.of(err(409, "ext_not_connected", """{"provider":"google"}""")))
        // provider の無い古いサーバー
        assertEquals(ExtDenialRoute.Message(Reason.NOT_CONNECTED_UNKNOWN), ExtDenialRoute.of(err(409, "ext_not_connected")))
    }

    @Test
    fun proRequiredGoesToProInfo() {
        val features = """{"features":[{"id":"externalServices","title":"つないだサービス","reason":"費用"}]}"""
        assertEquals(
            ExtDenialRoute.ProInfo(listOf(PaidFeature("externalServices", "つないだサービス", "費用"))),
            ExtDenialRoute.of(err(402, "pro_required", features)),
        )
        // features が空なら外の API の機能を出す
        assertEquals(
            ExtDenialRoute.ProInfo(listOf(PaidFeature(ExtDenialRoute.EXT_FEATURE_ID, ExtDenialRoute.EXT_FEATURE_ID))),
            ExtDenialRoute.of(err(402, "pro_required")),
        )
        assertEquals(ExtDenialRoute.Message(Reason.PRO_NOT_SYNCED), ExtDenialRoute.of(err(402, "pro_required"), isPro = true))
        assertEquals(
            ExtDenialRoute.Message(Reason.CREATOR_NEEDS_PRO),
            ExtDenialRoute.of(err(402, "pro_required", """{"reason":"creator_needs_pro"}""")),
        )
    }

    @Test
    fun coinCodesGoToBalance() {
        val insufficient = ExtDenialRoute.of(err(402, "coins_insufficient", """{"required":10,"balance":3,"payer":"self"}"""))
        assertEquals(ExtDenialRoute.CoinBalance(ExtDenial.Coins(ExtDenial.CoinKind.COINS_INSUFFICIENT, 10, 3, "self")), insufficient)
        assertEquals(
            ExtDenialRoute.CoinBalance(ExtDenial.Coins(ExtDenial.CoinKind.ALLOWANCE_EXHAUSTED)),
            ExtDenialRoute.of(err(402, "allowance_exhausted")),
        )
        val cap = ExtDenial.from(err(402, "overflow_cap_reached", """{"payer":"creator"}""")) as ExtDenial.Coins
        assertEquals(ExtDenial.CoinKind.OVERFLOW_CAP_REACHED, cap.kind)
        assertEquals(false, cap.payerIsSelf)
    }

    @Test
    fun unavailableAndBusyAreMessages() {
        assertEquals(ExtDenialRoute.Message(Reason.UNAVAILABLE), ExtDenialRoute.of(err(503, "ext_unavailable")))
        assertEquals(ExtDenialRoute.Message(Reason.BUSY), ExtDenialRoute.of(err(503, "ext_busy")))
    }

    @Test
    fun unknownCodesAreLeftToNormalHandling() {
        assertNull(ExtDenialRoute.of(err(402, "something_new")))
        assertNull(ExtDenialRoute.of(err(402, "app_limit_reached")))
        // 402 以外のコインの code は外の API の断りではない
        assertNull(ExtDenialRoute.of(err(400, "coins_insufficient")))
        assertNull(ExtDenialRoute.of(ApiError.Http(502, "bad gateway")))
        assertNull(ExtDenialRoute.of(IllegalStateException("x")))
        assertNull(ExtDenial.from(ApiError.Server(500, "internal", "", JsonObject(emptyMap()))))
    }
}
